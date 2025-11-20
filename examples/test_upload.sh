#!/bin/bash

# Test script for CV Review Agent - Upload Method
# This script demonstrates how to use the CV Review Agent API

set -e

echo "=== CV Review Agent - Upload Method Test ==="
echo ""

# Check if server is running
echo "Checking if server is running..."
if ! curl -s http://localhost:8080/health > /dev/null; then
    echo "Error: Server is not running. Please start it with: go run main.go"
    exit 1
fi
echo "✓ Server is running"
echo ""

# Test health endpoint
echo "Testing health endpoint..."
curl -s http://localhost:8080/health | jq .
echo ""

# Upload documents and job description
echo "Uploading documents and job description..."
curl -X POST http://localhost:8080/ingest \
  -F "method=upload" \
  -F "job_description=$(cat examples/job_description.json)" \
  -F "files=@examples/JohnDoe_CV.txt" \
  -F "files=@examples/JohnDoe_CoverLetter.txt" \
  -F "files=@examples/JaneSmith_CV.txt" \
  -F "files=@examples/JaneSmith_CoverLetter.txt"
echo ""
echo ""

# Wait a moment for processing
echo "Waiting for evaluation to complete..."
sleep 2
echo ""

# Get the report
echo "Fetching evaluation report..."
curl -s http://localhost:8080/report | jq .
echo ""

echo "=== Test completed successfully ==="
