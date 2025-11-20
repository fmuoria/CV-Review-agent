package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/fmuoria/CV-Review-agent/internal/api"
	"github.com/fmuoria/CV-Review-agent/internal/agent"
)

func main() {
	// Initialize the CV review agent
	cvAgent := agent.NewCVReviewAgent()

	// Create API server
	server := api.NewServer(cvAgent)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Starting CV Review Agent on port %s...\n", port)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST /ingest - Upload documents or fetch from Gmail\n")
	fmt.Printf("  GET /report - Get ranked applicant results\n")

	if err := http.ListenAndServe(":"+port, server.Router()); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
