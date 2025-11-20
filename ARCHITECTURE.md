# Architecture Overview

This document provides a detailed overview of the CV Review Agent's architecture, design decisions, and component interactions.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         HTTP Client                          │
│              (curl, Postman, Web Browser)                    │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            │ HTTP/JSON
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                       API Server                             │
│                  (internal/api/server.go)                    │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ POST /ingest │  │  GET /report │  │  GET /health │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            │ Function Calls
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                      CV Review Agent                         │
│                  (internal/agent/agent.go)                   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ Orchestration Logic                                   │  │
│  │ - Process job description                             │  │
│  │ - Load applicant documents                            │  │
│  │ - Coordinate scoring                                  │  │
│  │ - Generate rankings                                   │  │
│  └──────────────────────────────────────────────────────┘  │
└───────┬──────────────────────┬──────────────────┬───────────┘
        │                      │                  │
        │                      │                  │
        ▼                      ▼                  ▼
┌──────────────┐     ┌──────────────┐    ┌──────────────┐
│  Ingestion   │     │   Scoring    │    │     LLM      │
│   Module     │     │   Module     │    │   Client     │
└──────────────┘     └──────────────┘    └──────────────┘
        │                      │                  │
        ▼                      ▼                  ▼
┌──────────────┐     ┌──────────────┐    ┌──────────────┐
│ File Handler │     │ Score Calc   │    │ VertexAI API │
│ Gmail Handler│     │ Prompt Build │    │   Gemini     │
└──────────────┘     └──────────────┘    └──────────────┘
```

## Component Details

### 1. Main Application (`main.go`)

**Responsibilities:**
- Initialize the CV Review Agent
- Create and configure HTTP server
- Handle graceful shutdown
- Read environment variables

**Key Features:**
- Configurable port via `PORT` environment variable
- Clean initialization and dependency injection

### 2. API Server (`internal/api/server.go`)

**Responsibilities:**
- Handle HTTP requests and responses
- Route requests to appropriate handlers
- Validate input data
- Format responses as JSON

**Endpoints:**

#### `POST /ingest`
- Accepts multipart form data
- Parameters:
  - `method`: "upload" or "gmail"
  - `job_description`: JSON string with job requirements
  - `files`: Multiple file uploads (for "upload" method)
  - `gmail_subject`: Email subject filter (for "gmail" method)
- Returns: Success/error status

#### `GET /report`
- No parameters required
- Returns: Ranked list of applicants with scores and reasoning

#### `GET /health`
- Health check endpoint
- Returns: Service status

### 3. CV Review Agent (`internal/agent/agent.go`)

**Responsibilities:**
- Orchestrate the entire review process
- Coordinate between ingestion, scoring, and reporting
- Manage application state

**Key Methods:**

- `IngestFromUpload(jobDescJSON string)`: Process local uploads
- `IngestFromGmail(subject, jobDescJSON string)`: Process Gmail attachments
- `processApplicants(documents)`: Evaluate and rank applicants
- `GetReport()`: Return evaluation results

**State Management:**
- Job description
- Applicant results
- LLM client instance

### 4. Ingestion Module

#### File Handler (`internal/ingestion/file_handler.go`)

**Responsibilities:**
- Save uploaded files to disk
- Load documents from uploads directory
- Match CVs with cover letters
- Clear uploads directory

**File Naming Convention:**
- CV: `ApplicantName_CV.{ext}` or `ApplicantName_Resume.{ext}`
- Cover Letter: `ApplicantName_CoverLetter.{ext}` or `ApplicantName_Letter.{ext}`

**Matching Logic:**
1. Extract applicant name from filename (part before first underscore)
2. Determine document type from remaining filename
3. Group documents by applicant name
4. Return only applicants with at least a CV

#### Gmail Handler (`internal/ingestion/gmail_handler.go`)

**Responsibilities:**
- Connect to Gmail API using OAuth2
- Search for emails by subject
- Download attachments
- Rename files to match convention

**Authentication Flow:**
1. Read `credentials.json` (OAuth2 credentials)
2. Check for existing `token.json`
3. If no token, initiate OAuth flow
4. User authorizes in browser
5. Save token for future use

### 5. Scoring Module (`internal/scoring/scorer.go`)

**Responsibilities:**
- Build comprehensive prompts for LLM
- Send evaluation requests to LLM
- Parse and validate LLM responses
- Calculate total scores

**Scoring Criteria:**

#### Experience Score (0-50 points)
- **Required qualifications**: Missing = -10 to -15 points each
- **Nice-to-have qualifications**: Missing = -2 to -5 points each
- Evaluates depth, relevance, and years of experience

#### Education Score (0-20 points)
- **Required education**: Missing = -10+ points
- **Nice-to-have education**: Missing = -2 to -3 points
- Considers degree level and field relevance

#### Duties Score (0-20 points)
- **Required duties**: Cannot perform = -5 to -7 points each
- Evaluates demonstrated ability to fulfill job responsibilities

#### Cover Letter Score (0-10 points)
- No cover letter = 0 points
- Assesses quality, relevance, and alignment
- Evaluates communication skills and enthusiasm

**Prompt Engineering:**
- Clear instructions for structured output
- JSON format specification
- Emphasis on required vs. nice-to-have distinction
- Detailed scoring guidelines

### 6. LLM Client (`internal/llm/vertexai.go`)

**Responsibilities:**
- Initialize VertexAI client
- Configure Gemini model parameters
- Send prompts and receive responses
- Handle API errors

**Model Configuration:**
- Model: `gemini-1.5-flash`
- Temperature: 0.2 (for consistent scoring)
- Max output tokens: 2048
- Top-K: 40, Top-P: 0.95

**Environment Variables:**
- `GOOGLE_CLOUD_PROJECT`: GCP project ID
- `GOOGLE_CLOUD_LOCATION`: API region (default: us-central1)
- `GOOGLE_APPLICATION_CREDENTIALS`: Service account key path

### 7. Data Models (`internal/models/models.go`)

**Core Structures:**

```go
type JobDescription struct {
    Title                string
    RequiredExperience   []string
    RequiredEducation    []string
    RequiredDuties       []string
    NiceToHaveExperience []string
    NiceToHaveEducation  []string
    NiceToHaveDuties     []string
    Description          string
}

type ApplicantDocument struct {
    Name      string
    CVContent string
    CVPath    string
    CLContent string
    CLPath    string
}

type Scores struct {
    ExperienceScore      float64
    EducationScore       float64
    DutiesScore          float64
    CoverLetterScore     float64
    TotalScore           float64
    ExperienceReasoning  string
    EducationReasoning   string
    DutiesReasoning      string
    CoverLetterReasoning string
}

type ApplicantResult struct {
    Name   string
    Scores Scores
    Rank   int
}
```

## Data Flow

### Upload Method

1. **Client** sends POST request with files and job description
2. **API Server** validates request and saves files
3. **Agent** parses job description JSON
4. **File Handler** loads documents from uploads directory
5. **Agent** matches CVs with cover letters
6. For each applicant:
   - **Scorer** builds comprehensive prompt
   - **LLM Client** sends to VertexAI Gemini
   - **Scorer** parses response and calculates total score
7. **Agent** ranks applicants by total score
8. **Agent** stores results
9. **Client** retrieves report via GET /report

### Gmail Method

1. **Client** sends POST request with subject and job description
2. **API Server** validates request
3. **Agent** parses job description JSON
4. **Gmail Handler** authenticates with Gmail API
5. **Gmail Handler** searches for emails with subject
6. **Gmail Handler** downloads attachments
7. **File Handler** renames files to match convention
8. (Continue as Upload Method from step 4)

## Design Decisions

### 1. Modular Architecture
**Rationale:** Separation of concerns makes code maintainable and testable

**Benefits:**
- Easy to test individual components
- Clear responsibility boundaries
- Simple to extend or replace modules

### 2. File-Based Document Matching
**Rationale:** Filename conventions provide reliable matching without complex heuristics

**Advantages:**
- Simple to implement
- Predictable behavior
- Easy for users to understand

**Trade-offs:**
- Requires users to follow naming convention
- Less flexible than content-based matching

### 3. Structured LLM Prompts
**Rationale:** JSON responses ensure consistent, parseable output

**Advantages:**
- Reliable data extraction
- Type-safe processing
- Clear expectations for LLM

**Challenges:**
- LLM may occasionally produce malformed JSON
- Requires explicit format instructions

### 4. Weighted Scoring System
**Rationale:** Differentiate between must-have and nice-to-have qualifications

**Implementation:**
- Higher point values for critical categories
- Larger penalties for missing required items
- Smaller penalties for missing nice-to-have items

### 5. RESTful API Design
**Rationale:** Standard HTTP methods for intuitive integration

**Benefits:**
- Easy to test with curl/Postman
- Language-agnostic client support
- Familiar to developers

## Security Considerations

### 1. Credential Management
- Service account keys never committed
- OAuth tokens stored locally only
- Environment variables for sensitive data

### 2. Input Validation
- File type restrictions
- Size limits on uploads
- JSON schema validation

### 3. API Security
- No authentication implemented (suitable for local use)
- Consider adding API keys for production
- Rate limiting not implemented (add for public deployment)

## Performance Considerations

### 1. LLM API Calls
- Each applicant requires one API call
- Sequential processing (not parallel)
- Typically 2-5 seconds per applicant

**Optimization Opportunities:**
- Batch processing for multiple applicants
- Caching for repeated evaluations
- Parallel API calls with rate limiting

### 2. File Handling
- Files stored on disk (not in memory)
- Suitable for moderate numbers of applicants
- Consider cloud storage for large scale

### 3. Concurrency
- Single-threaded processing currently
- Could add goroutines for parallel evaluation
- Must respect API rate limits

## Testing Strategy

### Unit Tests
- Data model serialization
- File operations
- Score calculations

### Integration Tests
- API endpoint validation
- End-to-end workflows
- Error handling

### Manual Testing
- Real document uploads
- Gmail integration
- LLM response quality

## Future Enhancements

### Potential Features
1. **Advanced Matching**: Content-based CV-CL matching
2. **Batch Processing**: Parallel applicant evaluation
3. **Web UI**: Browser-based interface
4. **Database Storage**: Persistent result storage
5. **Report Formats**: PDF/Excel export
6. **Custom Scoring**: Configurable weights
7. **Multi-language**: Support for non-English CVs
8. **Resume Parsing**: Extract structured data
9. **Interview Scheduling**: Integration with calendar
10. **Feedback Loop**: Learn from hiring decisions

### Scalability Improvements
1. Message queue for async processing
2. Horizontal scaling with load balancer
3. Distributed file storage
4. Result caching layer
5. Connection pooling for APIs

## Deployment Options

### 1. Local Development
```bash
go run main.go
```

### 2. Standalone Binary
```bash
go build -o cv-review-agent .
./cv-review-agent
```

### 3. Docker Container
```bash
docker build -t cv-review-agent .
docker run -p 8080:8080 cv-review-agent
```

### 4. Docker Compose
```bash
docker-compose up
```

### 5. Cloud Deployment
- Google Cloud Run
- AWS ECS/Fargate
- Azure Container Instances
- Kubernetes cluster

## Monitoring and Observability

### Current State
- Basic HTTP logging
- Console output for processing steps

### Recommended Additions
1. **Structured Logging**: Use proper logging library
2. **Metrics**: Prometheus metrics endpoint
3. **Tracing**: Distributed tracing for API calls
4. **Health Checks**: Detailed component status
5. **Alerting**: Error rate monitoring

## Contributing to Architecture

When proposing architectural changes:
1. Document the problem being solved
2. Explain alternative approaches considered
3. Justify the chosen solution
4. Update this document
5. Include migration path if breaking changes

## References

- [VertexAI Documentation](https://cloud.google.com/vertex-ai/docs)
- [Gmail API Documentation](https://developers.google.com/gmail/api)
- [Go Best Practices](https://golang.org/doc/effective_go)
- [RESTful API Design](https://restfulapi.net/)
