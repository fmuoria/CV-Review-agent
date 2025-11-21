package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/ingestion"
	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/models"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
)

// ProgressCallback is called to report progress during processing
type ProgressCallback func(current, total int, message string)

// CVReviewAgent orchestrates the CV review process
type CVReviewAgent struct {
	FileHandler  *ingestion.FileHandler
	gmailHandler *ingestion.GmailHandler
	llmClient    *llm.VertexAIClient
	scorer       *scoring.Scorer
	jobDesc      models.JobDescription
	results      []models.ApplicantResult
	mu           sync.RWMutex
	progressCb   ProgressCallback
}

// NewCVReviewAgent creates a new CV review agent
func NewCVReviewAgent() *CVReviewAgent {
	fileHandler := ingestion.NewFileHandler("uploads")

	return &CVReviewAgent{
		FileHandler: fileHandler,
	}
}

// SetProgressCallback sets the progress callback function
func (a *CVReviewAgent) SetProgressCallback(cb ProgressCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.progressCb = cb
}

// reportProgress calls the progress callback if set
func (a *CVReviewAgent) reportProgress(current, total int, message string) {
	a.mu.RLock()
	cb := a.progressCb
	a.mu.RUnlock()

	if cb != nil {
		cb(current, total, message)
	}
}

// IngestFromUpload processes documents from the uploads directory
func (a *CVReviewAgent) IngestFromUpload(jobDescJSON string) error {
	return a.IngestFromUploadWithContext(context.Background(), jobDescJSON)
}

// IngestFromUploadWithContext processes documents from the uploads directory with context
func (a *CVReviewAgent) IngestFromUploadWithContext(ctx context.Context, jobDescJSON string) error {
	// Parse job description
	if err := json.Unmarshal([]byte(jobDescJSON), &a.jobDesc); err != nil {
		return fmt.Errorf("failed to parse job description: %w", err)
	}

	a.reportProgress(0, 100, "Initializing LLM client...")

	// Initialize LLM client
	llmClient, err := llm.NewVertexAIClient()
	if err != nil {
		return fmt.Errorf("failed to initialize LLM client: %w", err)
	}
	a.llmClient = llmClient
	a.scorer = scoring.NewScorer(llmClient)

	a.reportProgress(10, 100, "Loading documents...")

	// Load documents
	documents, err := a.FileHandler.LoadDocuments()
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	if len(documents) == 0 {
		return fmt.Errorf("no documents found in uploads directory")
	}

	log.Printf("Found %d applicants to evaluate", len(documents))
	a.reportProgress(20, 100, fmt.Sprintf("Processing %d applicants...", len(documents)))

	// Process each applicant
	return a.processApplicants(ctx, documents)
}

// IngestFromGmail processes documents from Gmail
func (a *CVReviewAgent) IngestFromGmail(subject string, jobDescJSON string) error {
	return a.IngestFromGmailWithContext(context.Background(), subject, jobDescJSON)
}

// IngestFromGmailWithContext processes documents from Gmail with context
func (a *CVReviewAgent) IngestFromGmailWithContext(ctx context.Context, subject string, jobDescJSON string) error {
	// Parse job description
	if err := json.Unmarshal([]byte(jobDescJSON), &a.jobDesc); err != nil {
		return fmt.Errorf("failed to parse job description: %w", err)
	}

	a.reportProgress(0, 100, "Initializing Gmail handler...")

	// Initialize Gmail handler with progress callback
	gmailHandler, err := ingestion.NewGmailHandlerWithCallback("uploads", func(current, total int, message string) {
		// Map Gmail progress (0-40% of total progress)
		progress := 40 * current / total
		a.reportProgress(progress, 100, message)
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Gmail handler: %w", err)
	}
	a.gmailHandler = gmailHandler

	a.reportProgress(5, 100, "Clearing existing uploads...")

	// Clear existing uploads
	if err := a.FileHandler.ClearUploads(); err != nil {
		return fmt.Errorf("failed to clear uploads: %w", err)
	}

	a.reportProgress(10, 100, "Fetching emails from Gmail...")

	// Fetch attachments from Gmail
	if err := a.gmailHandler.FetchAttachmentsWithContext(ctx, subject); err != nil {
		return fmt.Errorf("failed to fetch Gmail attachments: %w", err)
	}

	a.reportProgress(40, 100, "Initializing LLM client...")

	// Initialize LLM client
	llmClient, err := llm.NewVertexAIClient()
	if err != nil {
		return fmt.Errorf("failed to initialize LLM client: %w", err)
	}
	a.llmClient = llmClient
	a.scorer = scoring.NewScorer(llmClient)

	a.reportProgress(50, 100, "Loading documents...")

	// Load the fetched documents
	documents, err := a.FileHandler.LoadDocuments()
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	if len(documents) == 0 {
		return fmt.Errorf("no documents found after Gmail fetch")
	}

	log.Printf("Found %d applicants to evaluate from Gmail", len(documents))
	a.reportProgress(60, 100, fmt.Sprintf("Processing %d applicants...", len(documents)))

	// Process each applicant
	return a.processApplicants(ctx, documents)
}

// processApplicants evaluates all applicants and generates rankings
func (a *CVReviewAgent) processApplicants(ctx context.Context, documents []models.ApplicantDocument) error {
	results := make([]models.ApplicantResult, 0, len(documents))
	baseProgress := 60 // Start at 60% for Gmail, 20% for upload

	for i, doc := range documents {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("Evaluating applicant %d/%d: %s", i+1, len(documents), doc.Name)

		// Calculate progress (60-95% of total)
		progress := baseProgress + (35 * i / len(documents))
		a.reportProgress(progress, 100, fmt.Sprintf("Evaluating %s (%d/%d)", doc.Name, i+1, len(documents)))

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

	a.reportProgress(95, 100, "Ranking candidates...")

	// Sort by total score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Scores.TotalScore > results[j].Scores.TotalScore
	})

	// Assign ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	a.mu.Lock()
	a.results = results
	a.mu.Unlock()

	a.reportProgress(100, 100, "Processing complete!")

	return nil
}

// GetReport returns the evaluation report
func (a *CVReviewAgent) GetReport() (models.ReportResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.results) == 0 {
		return models.ReportResponse{}, fmt.Errorf("no results available, run ingestion first")
	}

	return models.ReportResponse{
		Applicants: a.results,
		JobTitle:   a.jobDesc.Title,
		Timestamp:  time.Now().Format(time.RFC3339),
	}, nil
}

// GetResults returns the current results (thread-safe)
func (a *CVReviewAgent) GetResults() []models.ApplicantResult {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Return a copy to prevent external modification
	resultsCopy := make([]models.ApplicantResult, len(a.results))
	copy(resultsCopy, a.results)
	return resultsCopy
}

// GetJobDescription returns the current job description (thread-safe)
func (a *CVReviewAgent) GetJobDescription() models.JobDescription {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.jobDesc
}

// Close cleans up resources
func (a *CVReviewAgent) Close() error {
	if a.llmClient != nil {
		return a.llmClient.Close()
	}
	return nil
}
