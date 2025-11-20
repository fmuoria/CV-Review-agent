package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/ingestion"
	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/models"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
)

// CVReviewAgent orchestrates the CV review process
type CVReviewAgent struct {
	FileHandler  *ingestion.FileHandler
	gmailHandler *ingestion.GmailHandler
	llmClient    *llm.VertexAIClient
	scorer       *scoring.Scorer
	jobDesc      models.JobDescription
	results      []models.ApplicantResult
}

// NewCVReviewAgent creates a new CV review agent
func NewCVReviewAgent() *CVReviewAgent {
	fileHandler := ingestion.NewFileHandler("uploads")
	
	return &CVReviewAgent{
		FileHandler: fileHandler,
	}
}

// IngestFromUpload processes documents from the uploads directory
func (a *CVReviewAgent) IngestFromUpload(jobDescJSON string) error {
	// Parse job description
	if err := json.Unmarshal([]byte(jobDescJSON), &a.jobDesc); err != nil {
		return fmt.Errorf("failed to parse job description: %w", err)
	}

	// Initialize LLM client
	llmClient, err := llm.NewVertexAIClient()
	if err != nil {
		return fmt.Errorf("failed to initialize LLM client: %w", err)
	}
	a.llmClient = llmClient
	a.scorer = scoring.NewScorer(llmClient)

	// Load documents
	documents, err := a.FileHandler.LoadDocuments()
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	if len(documents) == 0 {
		return fmt.Errorf("no documents found in uploads directory")
	}

	log.Printf("Found %d applicants to evaluate", len(documents))

	// Process each applicant
	return a.processApplicants(documents)
}

// IngestFromGmail processes documents from Gmail
func (a *CVReviewAgent) IngestFromGmail(subject string, jobDescJSON string) error {
	// Parse job description
	if err := json.Unmarshal([]byte(jobDescJSON), &a.jobDesc); err != nil {
		return fmt.Errorf("failed to parse job description: %w", err)
	}

	// Initialize Gmail handler
	gmailHandler, err := ingestion.NewGmailHandler("uploads")
	if err != nil {
		return fmt.Errorf("failed to initialize Gmail handler: %w", err)
	}
	a.gmailHandler = gmailHandler

	// Clear existing uploads
	if err := a.FileHandler.ClearUploads(); err != nil {
		return fmt.Errorf("failed to clear uploads: %w", err)
	}

	// Fetch attachments from Gmail
	if err := a.gmailHandler.FetchAttachments(subject); err != nil {
		return fmt.Errorf("failed to fetch Gmail attachments: %w", err)
	}

	// Initialize LLM client
	llmClient, err := llm.NewVertexAIClient()
	if err != nil {
		return fmt.Errorf("failed to initialize LLM client: %w", err)
	}
	a.llmClient = llmClient
	a.scorer = scoring.NewScorer(llmClient)

	// Load the fetched documents
	documents, err := a.FileHandler.LoadDocuments()
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	if len(documents) == 0 {
		return fmt.Errorf("no documents found after Gmail fetch")
	}

	log.Printf("Found %d applicants to evaluate from Gmail", len(documents))

	// Process each applicant
	return a.processApplicants(documents)
}

// processApplicants evaluates all applicants and generates rankings
func (a *CVReviewAgent) processApplicants(documents []models.ApplicantDocument) error {
	ctx := context.Background()
	results := make([]models.ApplicantResult, 0, len(documents))

	for i, doc := range documents {
		log.Printf("Evaluating applicant %d/%d: %s", i+1, len(documents), doc.Name)

		scores, err := a.scorer.ScoreApplicant(ctx, doc, a.jobDesc)
		if err != nil {
			log.Printf("Failed to score applicant %s: %v", doc.Name, err)
			// Continue with next applicant
			continue
		}

		result := models.ApplicantResult{
			Name:   doc.Name,
			Scores: scores,
		}
		results = append(results, result)
	}

	// Sort by total score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Scores.TotalScore > results[j].Scores.TotalScore
	})

	// Assign ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	a.results = results
	return nil
}

// GetReport returns the evaluation report
func (a *CVReviewAgent) GetReport() (models.ReportResponse, error) {
	if len(a.results) == 0 {
		return models.ReportResponse{}, fmt.Errorf("no results available, run ingestion first")
	}

	return models.ReportResponse{
		Applicants: a.results,
		JobTitle:   a.jobDesc.Title,
		Timestamp:  time.Now().Format(time.RFC3339),
	}, nil
}

// Close cleans up resources
func (a *CVReviewAgent) Close() error {
	if a.llmClient != nil {
		return a.llmClient.Close()
	}
	return nil
}
