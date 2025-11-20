package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/fmuoria/CV-Review-agent/internal/agent"
)

// Server handles HTTP requests
type Server struct {
	agent *agent.CVReviewAgent
}

// NewServer creates a new API server
func NewServer(agent *agent.CVReviewAgent) *Server {
	return &Server{
		agent: agent,
	}
}

// Router returns the HTTP router
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /ingest", s.handleIngest)
	mux.HandleFunc("GET /report", s.handleReport)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /", s.handleRoot)

	return s.loggingMiddleware(mux)
}

// handleRoot provides API information
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": "CV Review Agent",
		"version": "1.0.0",
		"endpoints": map[string]string{
			"POST /ingest": "Upload documents or fetch from Gmail",
			"GET /report":  "Get ranked applicant results",
			"GET /health":  "Health check",
		},
	})
}

// handleHealth provides a health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleIngest processes document ingestion
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB max
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse form: %v", err))
		return
	}

	method := r.FormValue("method")
	jobDescJSON := r.FormValue("job_description")

	if jobDescJSON == "" {
		s.respondError(w, http.StatusBadRequest, "job_description is required")
		return
	}

	switch method {
	case "upload":
		if err := s.handleUploadMethod(r, jobDescJSON); err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "gmail":
		gmailSubject := r.FormValue("gmail_subject")
		if gmailSubject == "" {
			s.respondError(w, http.StatusBadRequest, "gmail_subject is required for gmail method")
			return
		}
		if err := s.agent.IngestFromGmail(gmailSubject, jobDescJSON); err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		s.respondError(w, http.StatusBadRequest, "method must be 'upload' or 'gmail'")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Documents ingested and evaluated successfully",
	})
}

// handleUploadMethod processes file uploads
func (s *Server) handleUploadMethod(r *http.Request, jobDescJSON string) error {
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		return fmt.Errorf("no files uploaded")
	}

	// Create file handler
	fileHandler := s.agent.FileHandler

	// Save uploaded files
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return fmt.Errorf("failed to open uploaded file: %w", err)
		}
		defer file.Close()

		// Validate file extension
		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		if ext != ".pdf" && ext != ".txt" && ext != ".doc" && ext != ".docx" {
			log.Printf("Skipping unsupported file type: %s", fileHeader.Filename)
			continue
		}

		if _, err := fileHandler.SaveUploadedFile(fileHeader.Filename, file); err != nil {
			return fmt.Errorf("failed to save file %s: %w", fileHeader.Filename, err)
		}
		log.Printf("Saved file: %s", fileHeader.Filename)
	}

	// Process the uploaded documents
	return s.agent.IngestFromUpload(jobDescJSON)
}

// handleReport returns the evaluation report
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	report, err := s.agent.GetReport()
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, report)
}

// respondJSON sends a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// respondError sends an error response
func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{
		"error": message,
	})
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
