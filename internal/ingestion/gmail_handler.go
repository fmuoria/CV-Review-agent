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
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailProgressCallback is called to report progress during Gmail fetching
type GmailProgressCallback func(current, total int, message string)

// GmailHandler manages Gmail operations for fetching attachments
type GmailHandler struct {
	service    *gmail.Service
	uploadsDir string
	progressCb GmailProgressCallback
}

// NewGmailHandler creates a new Gmail handler
func NewGmailHandler(uploadsDir string) (*GmailHandler, error) {
	return NewGmailHandlerWithCallback(uploadsDir, nil)
}

// NewGmailHandlerWithCallback creates a new Gmail handler with progress callback
func NewGmailHandlerWithCallback(uploadsDir string, progressCb GmailProgressCallback) (*GmailHandler, error) {
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
		progressCb: progressCb,
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
	return gh.FetchAttachmentsWithContext(context.Background(), subject)
}

// FetchAttachmentsWithContext fetches email attachments with a specific subject and context
func (gh *GmailHandler) FetchAttachmentsWithContext(ctx context.Context, subject string) error {
	// Ensure uploads directory exists
	if err := os.MkdirAll(gh.uploadsDir, 0755); err != nil {
		return fmt.Errorf("failed to create uploads directory: %w", err)
	}

	user := "me"
	query := fmt.Sprintf("subject:%s has:attachment", subject)

	gh.reportProgress(0, 100, "Listing emails...")

	// List all messages with pagination
	var allMessages []*gmail.Message
	pageToken := ""
	maxResults := int64(100) // Fetch 100 messages per page

	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listCall := gh.service.Users.Messages.List(user).Q(query).MaxResults(maxResults)
		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}

		r, err := listCall.Do()
		if err != nil {
			return fmt.Errorf("unable to retrieve messages: %w", err)
		}

		allMessages = append(allMessages, r.Messages...)

		if r.NextPageToken == "" {
			break
		}
		pageToken = r.NextPageToken

		gh.reportProgress(len(allMessages)*20/100, 100, fmt.Sprintf("Listed %d emails...", len(allMessages)))
	}

	if len(allMessages) == 0 {
		return fmt.Errorf("no messages found with subject: %s", subject)
	}

	log.Printf("Found %d emails to process", len(allMessages))
	gh.reportProgress(20, 100, fmt.Sprintf("Processing %d emails...", len(allMessages)))

	// Process each message with retry logic
	downloadedCount := 0
	for i, msg := range allMessages {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		progress := 20 + (80 * i / len(allMessages))
		gh.reportProgress(progress, 100, fmt.Sprintf("Processing email %d/%d", i+1, len(allMessages)))

		if err := gh.processMessageWithRetry(ctx, user, msg.Id, 3); err != nil {
			log.Printf("Failed to process message %s after retries: %v", msg.Id, err)
			continue
		}
		downloadedCount++
	}

	gh.reportProgress(100, 100, fmt.Sprintf("Downloaded %d attachments", downloadedCount))
	log.Printf("Successfully downloaded attachments from %d emails", downloadedCount)

	return nil
}

// processMessageWithRetry processes a single message with retry logic
func (gh *GmailHandler) processMessageWithRetry(ctx context.Context, user, messageId string, retries int) error {
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			// Wait before retry (exponential backoff)
			time.Sleep(time.Duration(attempt) * time.Second)
			log.Printf("Retry attempt %d for message %s", attempt, messageId)
		}

		message, err := gh.service.Users.Messages.Get(user, messageId).Do()
		if err != nil {
			lastErr = fmt.Errorf("unable to retrieve message: %w", err)
			continue
		}

		// Extract sender name for file naming
		senderName := extractSenderName(message)

		// Process attachments
		hasAttachments := false
		for _, part := range message.Payload.Parts {
			if part.Filename != "" && part.Body.AttachmentId != "" {
				hasAttachments = true

				attachment, err := gh.service.Users.Messages.Attachments.Get(user, messageId, part.Body.AttachmentId).Do()
				if err != nil {
					lastErr = fmt.Errorf("unable to retrieve attachment: %w", err)
					continue
				}

				data, err := base64.URLEncoding.DecodeString(attachment.Data)
				if err != nil {
					lastErr = fmt.Errorf("unable to decode attachment: %w", err)
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
					lastErr = fmt.Errorf("unable to write file: %w", err)
					continue
				}

				log.Printf("Downloaded: %s", newFilename)
			}
		}

		if hasAttachments {
			return nil // Success
		}
	}

	return lastErr
}

// reportProgress calls the progress callback if set
func (gh *GmailHandler) reportProgress(current, total int, message string) {
	if gh.progressCb != nil {
		gh.progressCb(current, total, message)
	}
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
