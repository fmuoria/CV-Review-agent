# Quick Start Guide

Get up and running with CV Review Agent in 5 minutes!

## Prerequisites Check

```bash
# Verify Go installation
go version  # Should be 1.20+

# Verify GCP project
echo $GOOGLE_CLOUD_PROJECT  # Should show your project ID
```

## 1. Install (30 seconds)

```bash
git clone https://github.com/fmuoria/CV-Review-agent.git
cd CV-Review-agent
go mod download
```

## 2. Configure (2 minutes)

### Set Environment Variables

```bash
export GOOGLE_CLOUD_PROJECT="your-project-id"
export GOOGLE_CLOUD_LOCATION="us-central1"
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/your/service-account-key.json"
```

💡 **Don't have credentials?** See [SETUP.md](SETUP.md) for detailed instructions.

## 3. Run (5 seconds)

```bash
go run main.go
```

You should see:
```
Starting CV Review Agent on port 8080...
Endpoints:
  POST /ingest - Upload documents or fetch from Gmail
  GET /report - Get ranked applicant results
```

## 4. Test (2 minutes)

### Option A: Use the Test Script

```bash
# In a new terminal
./examples/test_upload.sh
```

### Option B: Manual Testing

```bash
# Test health endpoint
curl http://localhost:8080/health

# Upload example documents
curl -X POST http://localhost:8080/ingest \
  -F "method=upload" \
  -F "job_description=$(cat examples/job_description.json)" \
  -F "files=@examples/JohnDoe_CV.txt" \
  -F "files=@examples/JohnDoe_CoverLetter.txt" \
  -F "files=@examples/JaneSmith_CV.txt" \
  -F "files=@examples/JaneSmith_CoverLetter.txt"

# Get the report
curl http://localhost:8080/report | jq .
```

## Expected Output

```json
{
  "applicants": [
    {
      "name": "JohnDoe",
      "scores": {
        "experience_score": 45,
        "education_score": 20,
        "duties_score": 18,
        "cover_letter_score": 9,
        "total_score": 92,
        "experience_reasoning": "Candidate has 7 years of Go experience...",
        "education_reasoning": "Holds Bachelor's and Master's degrees...",
        "duties_reasoning": "Strong evidence of required capabilities...",
        "cover_letter_reasoning": "Excellent alignment with role..."
      },
      "rank": 1
    },
    {
      "name": "JaneSmith",
      "scores": {
        "experience_score": 35,
        "education_score": 15,
        "duties_score": 16,
        "cover_letter_score": 7,
        "total_score": 73,
        "experience_reasoning": "Has 4 years experience, slightly below required...",
        "education_reasoning": "Bachelor's degree meets minimum requirement...",
        "duties_reasoning": "Some relevant experience demonstrated...",
        "cover_letter_reasoning": "Good cover letter but less specific..."
      },
      "rank": 2
    }
  ],
  "job_title": "Senior Software Engineer",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Understanding the Scores

| Category | Points | What It Measures |
|----------|--------|------------------|
| **Experience** | 0-50 | Years of experience, relevant skills, required qualifications |
| **Education** | 0-20 | Degree level, field relevance, required education |
| **Duties** | 0-20 | Ability to perform job responsibilities |
| **Cover Letter** | 0-10 | Quality, relevance, enthusiasm |
| **TOTAL** | 0-100 | Overall candidate fit |

### Scoring Rules

- **Missing Required**: -10 to -15 points per item
- **Missing Nice-to-Have**: -2 to -5 points per item
- **No Cover Letter**: 0 points for that category

## Your Own Documents

### File Naming Convention

For the agent to match CVs with cover letters:

```
ApplicantName_CV.pdf          ← CV
ApplicantName_CoverLetter.pdf ← Cover Letter
```

Examples:
- ✅ `JohnSmith_CV.pdf` and `JohnSmith_CoverLetter.pdf`
- ✅ `JaneDoeLow_Resume.txt` and `JaneDoe_Letter.txt`
- ❌ `cv-john.pdf` and `letter-john.pdf` (won't match)

### Create Your Job Description

```bash
cat > my_job.json << 'EOF'
{
  "title": "Your Job Title",
  "description": "Full job description...",
  "required_experience": [
    "5+ years in X",
    "Expert in Y"
  ],
  "required_education": [
    "Bachelor's degree in Z"
  ],
  "required_duties": [
    "Do important task A",
    "Handle responsibility B"
  ],
  "nice_to_have_experience": [
    "Experience with W",
    "Knowledge of V"
  ],
  "nice_to_have_education": [
    "Master's degree"
  ],
  "nice_to_have_duties": [
    "Optional task C"
  ]
}
EOF
```

### Upload Your Documents

```bash
curl -X POST http://localhost:8080/ingest \
  -F "method=upload" \
  -F "job_description=$(cat my_job.json)" \
  -F "files=@Candidate1_CV.pdf" \
  -F "files=@Candidate1_CoverLetter.pdf" \
  -F "files=@Candidate2_CV.pdf" \
  -F "files=@Candidate2_CoverLetter.pdf"
```

## Troubleshooting

### "GOOGLE_CLOUD_PROJECT environment variable not set"

```bash
export GOOGLE_CLOUD_PROJECT="your-project-id"
```

### "failed to create Vertex AI client"

1. Check VertexAI API is enabled: [Enable VertexAI](https://console.cloud.google.com/apis/library/aiplatform.googleapis.com)
2. Verify service account permissions
3. Check credentials file path

### "No documents found"

- Check files are in `uploads/` directory
- Verify file naming follows convention
- Ensure file extensions are supported (.pdf, .txt, .doc, .docx)

### Port 8080 already in use

```bash
PORT=8081 go run main.go
```

## What's Next?

- **Gmail Integration**: Set up OAuth2 to fetch CVs from email → [SETUP.md](SETUP.md#step-4-optional-set-up-gmail-integration)
- **Docker Deployment**: Containerize your application → [README.md](README.md#deployment-options)
- **Customize Scoring**: Modify weights in `internal/scoring/scorer.go`
- **Add More Features**: See [ARCHITECTURE.md](ARCHITECTURE.md#future-enhancements)

## Need Help?

1. Check [SETUP.md](SETUP.md) for detailed setup
2. Read [README.md](README.md) for complete documentation
3. Review [ARCHITECTURE.md](ARCHITECTURE.md) for technical details
4. Open an issue on GitHub

## Useful Commands

```bash
# Build binary
make build

# Run application
make run

# Run tests
make test

# Format code
make fmt

# Clean artifacts
make clean

# View all commands
make help
```

## Tips for Best Results

### Job Descriptions
- Be specific about required vs. nice-to-have
- Include years of experience needed
- List concrete skills and technologies
- Describe key responsibilities clearly

### File Preparation
- Use consistent naming across all candidates
- Keep filenames simple (no spaces)
- Ensure documents are readable (not scanned images)
- Include both CV and cover letter when available

### Interpreting Results
- Focus on reasoning, not just scores
- Consider required qualifications heavily
- Use rankings as guidance, not absolute truth
- Review borderline candidates manually

## Example Workflow

```bash
# 1. Prepare your job description
vim my_job.json

# 2. Collect CVs and name them properly
ls uploads/
# Alice_CV.pdf  Alice_CoverLetter.pdf
# Bob_CV.pdf    Bob_CoverLetter.pdf
# Carol_CV.pdf  Carol_CoverLetter.pdf

# 3. Start the server
make run &

# 4. Process the candidates
curl -X POST http://localhost:8080/ingest \
  -F "method=upload" \
  -F "job_description=$(cat my_job.json)" \
  -F "files=@uploads/Alice_CV.pdf" \
  -F "files=@uploads/Alice_CoverLetter.pdf" \
  -F "files=@uploads/Bob_CV.pdf" \
  -F "files=@uploads/Bob_CoverLetter.pdf" \
  -F "files=@uploads/Carol_CV.pdf" \
  -F "files=@uploads/Carol_CoverLetter.pdf"

# 5. Get ranked results
curl http://localhost:8080/report | jq . > results.json

# 6. Review the rankings
cat results.json
```

Happy recruiting! 🎉
