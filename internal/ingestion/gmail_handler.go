package ingestion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailHandler manages Gmail operations for fetching attachments
type GmailHandler struct {
	service    *gmail.Service
	uploadsDir string
}

// NewGmailHandler creates a new Gmail handler
func NewGmailHandler(uploadsDir string) (*GmailHandler, error) {
	ctx := context.Background()

	// Read credentials
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	client := getClient(config)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Gmail client: %w", err)
	}

	return &GmailHandler{
		service:    srv,
		uploadsDir: uploadsDir,
	}, nil
}

// getClient retrieves a token, saves it, then returns the generated client
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// getTokenFromWeb requests a token from the web
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// tokenFromFile retrieves a token from a local file
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken saves a token to a file path
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// FetchAttachments fetches email attachments with a specific subject
func (gh *GmailHandler) FetchAttachments(subject string) error {
	// Ensure uploads directory exists
	if err := os.MkdirAll(gh.uploadsDir, 0755); err != nil {
		return fmt.Errorf("failed to create uploads directory: %w", err)
	}

	user := "me"
	query := fmt.Sprintf("subject:%s has:attachment", subject)

	r, err := gh.service.Users.Messages.List(user).Q(query).Do()
	if err != nil {
		return fmt.Errorf("unable to retrieve messages: %w", err)
	}

	if len(r.Messages) == 0 {
		return fmt.Errorf("no messages found with subject: %s", subject)
	}

	for _, msg := range r.Messages {
		message, err := gh.service.Users.Messages.Get(user, msg.Id).Do()
		if err != nil {
			log.Printf("Unable to retrieve message %s: %v", msg.Id, err)
			continue
		}

		// Extract sender name for file naming
		senderName := extractSenderName(message)

		// Process attachments
		for _, part := range message.Payload.Parts {
			if part.Filename != "" && part.Body.AttachmentId != "" {
				attachment, err := gh.service.Users.Messages.Attachments.Get(user, msg.Id, part.Body.AttachmentId).Do()
				if err != nil {
					log.Printf("Unable to retrieve attachment: %v", err)
					continue
				}

				data, err := base64.URLEncoding.DecodeString(attachment.Data)
				if err != nil {
					log.Printf("Unable to decode attachment: %v", err)
					continue
				}

				// Determine if it's a CV or cover letter based on filename
				filename := part.Filename
				ext := filepath.Ext(filename)
				baseName := strings.TrimSuffix(filename, ext)

				// Rename to match convention: SenderName_CV.ext or SenderName_CoverLetter.ext
				var newFilename string
				if strings.Contains(strings.ToLower(baseName), "cv") || strings.Contains(strings.ToLower(baseName), "resume") {
					newFilename = fmt.Sprintf("%s_CV%s", senderName, ext)
				} else if strings.Contains(strings.ToLower(baseName), "cover") || strings.Contains(strings.ToLower(baseName), "letter") {
					newFilename = fmt.Sprintf("%s_CoverLetter%s", senderName, ext)
				} else {
					newFilename = fmt.Sprintf("%s_%s", senderName, filename)
				}

				filePath := filepath.Join(gh.uploadsDir, newFilename)
				if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
					log.Printf("Unable to write file %s: %v", filePath, err)
					continue
				}

				fmt.Printf("Downloaded: %s\n", newFilename)
			}
		}
	}

	return nil
}

// extractSenderName extracts the sender's name from email headers
func extractSenderName(message *gmail.Message) string {
	for _, header := range message.Payload.Headers {
		if header.Name == "From" {
			// Parse "Name <email@example.com>" format
			from := header.Value
			if idx := strings.Index(from, "<"); idx > 0 {
				name := strings.TrimSpace(from[:idx])
				name = strings.ReplaceAll(name, " ", "")
				return name
			}
			// If no name, use email prefix
			if idx := strings.Index(from, "@"); idx > 0 {
				return from[:idx]
			}
			return "Unknown"
		}
	}
	return "Unknown"
}
