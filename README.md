# CV Review Agent

A Go-based intelligent CV Review Agent that uses Google's VertexAI Gemini LLM to evaluate job applicants against a job description. The agent ingests CVs and cover letters from local uploads or Gmail, matches them intelligently, and provides detailed scoring and ranking.

## Features

- **Multiple Ingestion Methods**:
  - Local file upload via HTTP API
  - Gmail inbox integration with subject-based filtering
  
- **Intelligent Document Matching**:
  - Automatically matches CVs to cover letters using filename conventions
  - Supports multiple file formats (PDF, TXT, DOC, DOCX)
  
- **AI-Powered Evaluation**:
  - Experience match scoring (0-50 points)
  - Education match scoring (0-20 points)
  - Duties/responsibilities alignment (0-20 points)
  - Cover letter quality assessment (0-10 points)
  
- **Qualification Differentiation**:
  - Clear distinction between required and nice-to-have qualifications
  - Weighted penalties for missing required qualifications
  - Minimal impact for missing nice-to-have items
  
- **Detailed Reporting**:
  - Comprehensive reasoning for each score component
  - Ranked applicant list by total score
  - JSON API for easy integration

## Prerequisites

- Go 1.20 or higher
- Google Cloud Project with VertexAI API enabled
- (Optional) Gmail API credentials for Gmail integration

## Installation

1. Clone the repository:
```bash
git clone https://github.com/fmuoria/CV-Review-agent.git
cd CV-Review-agent
```

2. Install dependencies:
```bash
go mod tidy
```

3. Set up Google Cloud credentials:
```bash
export GOOGLE_CLOUD_PROJECT="your-project-id"
export GOOGLE_CLOUD_LOCATION="us-central1"
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"
```

4. (Optional) For Gmail integration, set up OAuth2 credentials:
   - Go to [Google Cloud Console](https://console.cloud.google.com)
   - Enable Gmail API
   - Create OAuth 2.0 credentials (Desktop app)
   - Download credentials as `credentials.json` in the project root

## Usage

### Starting the Server

```bash
go run main.go
```

The server starts on port 8080 by default (configurable via `PORT` environment variable).

### API Endpoints

#### 1. Health Check
```bash
curl http://localhost:8080/health
```

#### 2. Ingest Documents (Upload Method)

Prepare your files with the naming convention:
- CV files: `ApplicantName_CV.pdf` or `ApplicantName_Resume.pdf`
- Cover letters: `ApplicantName_CoverLetter.pdf` or `ApplicantName_Letter.pdf`

Create a job description JSON file (`job_desc.json`):
```json
{
  "title": "Senior Software Engineer",
  "description": "We are looking for an experienced software engineer...",
  "required_experience": [
    "5+ years of Go programming",
    "Experience with microservices architecture",
    "RESTful API design"
  ],
  "required_education": [
    "Bachelor's degree in Computer Science or related field"
  ],
  "required_duties": [
    "Design and implement scalable backend services",
    "Write clean, maintainable code",
    "Conduct code reviews"
  ],
  "nice_to_have_experience": [
    "Experience with Kubernetes",
    "Cloud platform experience (GCP, AWS)"
  ],
  "nice_to_have_education": [
    "Master's degree in Computer Science"
  ],
  "nice_to_have_duties": [
    "Mentor junior developers",
    "Contribute to technical documentation"
  ]
}
```

Upload documents:
```bash
curl -X POST http://localhost:8080/ingest \
  -F "method=upload" \
  -F "job_description=$(cat job_desc.json)" \
  -F "files=@JohnDoe_CV.pdf" \
  -F "files=@JohnDoe_CoverLetter.pdf" \
  -F "files=@JaneSmith_CV.pdf" \
  -F "files=@JaneSmith_CoverLetter.pdf"
```

#### 3. Ingest Documents (Gmail Method)

```bash
curl -X POST http://localhost:8080/ingest \
  -F "method=gmail" \
  -F "gmail_subject=Job Application" \
  -F "job_description=$(cat job_desc.json)"
```

On first run, you'll be prompted to authorize the application via a browser link.

#### 4. Get Evaluation Report

```bash
curl http://localhost:8080/report
```

Sample response:
```json
{
  "applicants": [
    {
      "name": "JohnDoe",
      "scores": {
        "experience_score": 45.0,
        "experience_reasoning": "Candidate has 7 years of Go experience, exceeding the required 5+ years. Strong background in microservices and RESTful API design. Missing Kubernetes experience (nice-to-have).",
        "education_score": 20.0,
        "education_reasoning": "Holds Bachelor's degree in Computer Science, meeting the required qualification. Also has a Master's degree (nice-to-have bonus).",
        "duties_score": 18.0,
        "duties_reasoning": "Demonstrated experience in designing scalable systems and code reviews. Strong evidence of clean code practices.",
        "cover_letter_score": 9.0,
        "cover_letter_reasoning": "Excellent cover letter showing clear understanding of role requirements and enthusiasm for the position.",
        "total_score": 92.0
      },
      "rank": 1
    },
    {
      "name": "JaneSmith",
      "scores": {
        "experience_score": 35.0,
        "experience_reasoning": "Has 4 years of Go experience (slightly below required 5+). Good microservices knowledge but limited API design experience.",
        "education_score": 15.0,
        "education_reasoning": "Bachelor's degree in related field meets requirement, but not specifically Computer Science.",
        "duties_score": 16.0,
        "duties_reasoning": "Some experience with backend services, needs more evidence of scalability focus.",
        "cover_letter_score": 7.0,
        "cover_letter_reasoning": "Good cover letter but could be more specific about relevant experiences.",
        "total_score": 73.0
      },
      "rank": 2
    }
  ],
  "job_title": "Senior Software Engineer",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Project Structure

```
CV-Review-agent/
├── main.go                     # Application entry point
├── internal/
│   ├── agent/                  # Core agent orchestration logic
│   │   └── agent.go
│   ├── api/                    # HTTP API handlers
│   │   └── server.go
│   ├── models/                 # Data models
│   │   └── models.go
│   ├── ingestion/              # Document ingestion
│   │   ├── file_handler.go    # Local file handling
│   │   └── gmail_handler.go   # Gmail integration
│   ├── llm/                    # LLM integration
│   │   └── vertexai.go        # VertexAI Gemini client
│   └── scoring/                # Scoring logic
│       └── scorer.go
├── uploads/                    # Temporary upload directory
└── README.md
```

## File Naming Conventions

For the agent to correctly match CVs with cover letters, follow these naming conventions:

- **CV files**: `ApplicantName_CV.ext` or `ApplicantName_Resume.ext`
- **Cover letters**: `ApplicantName_CoverLetter.ext` or `ApplicantName_Letter.ext` or `ApplicantName_CL.ext`

Where:
- `ApplicantName` should be the same for related documents (no spaces)
- `ext` can be `.pdf`, `.txt`, `.doc`, or `.docx`

Examples:
- `JohnDoe_CV.pdf` and `JohnDoe_CoverLetter.pdf`
- `JaneSmith_Resume.pdf` and `JaneSmith_Letter.pdf`

## Scoring Details

### Experience Score (0-50 points)
- **Required qualifications**: Missing any required experience item reduces score by 10-15 points
- **Nice-to-have qualifications**: Missing items reduce score by 2-5 points
- The LLM evaluates depth and relevance of experience

### Education Score (0-20 points)
- **Required education**: Missing required degrees reduces score by 10+ points
- **Nice-to-have education**: Missing items reduce score by 2-3 points
- Considers degree relevance and level

### Duties Score (0-20 points)
- **Required duties**: Missing capability for required duties reduces score by 5-7 points
- Evaluates demonstrated ability to perform job responsibilities

### Cover Letter Score (0-10 points)
- Assesses quality, relevance, and alignment with the role
- No cover letter = 0 points
- Considers enthusiasm, understanding of role, and communication skills

## Environment Variables

- `PORT`: Server port (default: 8080)
- `GOOGLE_CLOUD_PROJECT`: Your GCP project ID (required)
- `GOOGLE_CLOUD_LOCATION`: VertexAI location (default: us-central1)
- `GOOGLE_APPLICATION_CREDENTIALS`: Path to service account key file

## Testing Locally

1. Create a test job description JSON file
2. Prepare sample CV and cover letter files with proper naming
3. Start the server: `go run main.go`
4. Upload documents using curl or Postman
5. Retrieve the evaluation report

Example test files are in the `examples/` directory (if provided).

## Troubleshooting

### LLM Client Initialization Fails
- Verify `GOOGLE_CLOUD_PROJECT` is set correctly
- Ensure VertexAI API is enabled in your GCP project
- Check that service account has necessary permissions

### Gmail Integration Not Working
- Ensure `credentials.json` is in the project root
- Complete OAuth2 authorization flow when prompted
- Check Gmail API is enabled in GCP console

### Files Not Being Processed
- Verify file naming follows the convention
- Check file extensions are supported (.pdf, .txt, .doc, .docx)
- Ensure files are in the correct directory or uploaded properly

## Security Considerations

- Never commit `credentials.json` or `token.json` to version control
- Use service account keys with minimal necessary permissions
- Store sensitive credentials in environment variables or secret managers
- Implement rate limiting for production deployments

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For issues, questions, or suggestions, please open an issue on GitHub.