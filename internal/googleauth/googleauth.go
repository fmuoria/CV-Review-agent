// Package googleauth gets OAuth access tokens for Google Cloud APIs
// (Vertex AI, Cloud Storage) without the heavy Google SDKs.
//
// It understands the two credential files you will meet in practice:
//
//   - "service_account" keys — what Render uses in production
//     (GOOGLE_SERVICE_ACCOUNT_JSON env var or GOOGLE_APPLICATION_CREDENTIALS path).
//   - "authorized_user" files — what `gcloud auth application-default login`
//     writes on your laptop, handy for local development.
package googleauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CloudPlatformScope grants access to Vertex AI and Cloud Storage.
const CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

const defaultTokenURL = "https://oauth2.googleapis.com/token"

// TokenSource returns a valid access token, refreshing it when needed.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type credentialsFile struct {
	Type string `json:"type"`
	// service_account
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI     string `json:"token_uri"`
	ProjectID    string `json:"project_id"`
	// authorized_user
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// FromConfig builds a TokenSource from raw JSON (preferred) or a file path.
// When both are empty it falls back to gcloud's default credentials file.
func FromConfig(jsonContent, path string, scopes ...string) (TokenSource, error) {
	var raw []byte
	switch {
	case strings.TrimSpace(jsonContent) != "":
		raw = []byte(jsonContent)
	case path != "":
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read credentials file: %w", err)
		}
		raw = b
	default:
		b, err := os.ReadFile(gcloudDefaultPath())
		if err != nil {
			return nil, errors.New("no Google credentials: set GOOGLE_SERVICE_ACCOUNT_JSON or GOOGLE_APPLICATION_CREDENTIALS, or run `gcloud auth application-default login`")
		}
		raw = b
	}
	return fromJSON(raw, scopes)
}

func fromJSON(raw []byte, scopes []string) (TokenSource, error) {
	var cf credentialsFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("parse credentials JSON: %w", err)
	}
	tokenURL := cf.TokenURI
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}
	switch cf.Type {
	case "service_account":
		key, err := parsePrivateKey(cf.PrivateKey)
		if err != nil {
			return nil, err
		}
		return &cachingSource{fetch: (&serviceAccount{
			email: cf.ClientEmail, keyID: cf.PrivateKeyID, key: key,
			tokenURL: tokenURL, scopes: scopes,
		}).fetch}, nil
	case "authorized_user":
		return &cachingSource{fetch: (&authorizedUser{
			clientID: cf.ClientID, clientSecret: cf.ClientSecret,
			refreshToken: cf.RefreshToken, tokenURL: defaultTokenURL,
		}).fetch}, nil
	default:
		return nil, fmt.Errorf("unsupported credentials type %q", cf.Type)
	}
}

func gcloudDefaultPath() string {
	if appdata := os.Getenv("APPDATA"); appdata != "" { // Windows
		return filepath.Join(appdata, "gcloud", "application_default_credentials.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

// cachingSource caches a token until shortly before it expires.
type cachingSource struct {
	mu      sync.Mutex
	token   string
	expires time.Time
	fetch   func(ctx context.Context) (string, time.Duration, error)
}

func (c *cachingSource) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.expires) > 2*time.Minute {
		return c.token, nil
	}
	tok, ttl, err := c.fetch(ctx)
	if err != nil {
		return "", err
	}
	c.token, c.expires = tok, time.Now().Add(ttl)
	return tok, nil
}

type serviceAccount struct {
	email, keyID, tokenURL string
	key                    *rsa.PrivateKey
	scopes                 []string
}

func (s *serviceAccount) fetch(ctx context.Context) (string, time.Duration, error) {
	now := time.Now()
	assertion, err := SignJWT(s.key, s.keyID, map[string]any{
		"iss":   s.email,
		"scope": strings.Join(s.scopes, " "),
		"aud":   s.tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", 0, err
	}
	return postToken(ctx, s.tokenURL, url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	})
}

type authorizedUser struct {
	clientID, clientSecret, refreshToken, tokenURL string
}

func (a *authorizedUser) fetch(ctx context.Context) (string, time.Duration, error) {
	return postToken(ctx, a.tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {a.clientID},
		"client_secret": {a.clientSecret},
		"refresh_token": {a.refreshToken},
	})
}

// RefreshAccessToken exchanges an OAuth refresh token (e.g. the stored Gmail
// connection) for a short-lived access token.
func RefreshAccessToken(ctx context.Context, clientID, clientSecret, refreshToken string) (string, time.Duration, error) {
	return postToken(ctx, defaultTokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
	})
}

func postToken(ctx context.Context, tokenURL string, form url.Values) (string, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, truncate(string(body), 300))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return "", 0, fmt.Errorf("token response missing access_token")
	}
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return tr.AccessToken, ttl, nil
}

// SignJWT creates an RS256-signed JWT.
func SignJWT(key *rsa.PrivateKey, keyID string, claims map[string]any) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if keyID != "" {
		header["kid"] = keyID
	}
	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	c, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func parsePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("service account private_key is not valid PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("service account key is not RSA")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// StaticToken is a TokenSource that always returns the same token (tests).
type StaticToken string

// Token implements TokenSource.
func (s StaticToken) Token(context.Context) (string, error) { return string(s), nil }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
