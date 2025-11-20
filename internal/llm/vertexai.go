package llm

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/vertexai/genai"
)

// VertexAIClient wraps the Vertex AI Gemini API
type VertexAIClient struct {
	client    *genai.Client
	model     *genai.GenerativeModel
	projectID string
	location  string
}

// NewVertexAIClient creates a new Vertex AI client
func NewVertexAIClient() (*VertexAIClient, error) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT environment variable not set")
	}

	location := os.Getenv("GOOGLE_CLOUD_LOCATION")
	if location == "" {
		location = "us-central1" // Default location
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vertex AI client: %w", err)
	}

	model := client.GenerativeModel("gemini-1.5-flash")
	
	// Configure model parameters
	model.SetTemperature(0.2) // Lower temperature for more consistent scoring
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(2048)

	return &VertexAIClient{
		client:    client,
		model:     model,
		projectID: projectID,
		location:  location,
	}, nil
}

// GenerateContent sends a prompt to the model and returns the response
func (v *VertexAIClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	resp, err := v.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no response candidates returned")
	}

	// Extract text from response
	var result string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			result += string(text)
		}
	}

	return result, nil
}

// Close closes the Vertex AI client
func (v *VertexAIClient) Close() error {
	return v.client.Close()
}
