# Examples Directory

This directory contains example files for testing the CV Review Agent.

## Files

- `job_description.json` - Sample job description for a Senior Software Engineer position
- `JohnDoe_CV.txt` - Sample CV for a highly qualified candidate
- `JohnDoe_CoverLetter.txt` - Sample cover letter for John Doe
- `JaneSmith_CV.txt` - Sample CV for a less experienced candidate
- `JaneSmith_CoverLetter.txt` - Sample cover letter for Jane Smith
- `test_upload.sh` - Bash script to test the upload functionality

## Usage

### Manual Testing

1. Start the server:
```bash
go run main.go
```

2. In another terminal, run the test script:
```bash
./examples/test_upload.sh
```

### Using curl directly

Upload documents:
```bash
curl -X POST http://localhost:8080/ingest \
  -F "method=upload" \
  -F "job_description=$(cat examples/job_description.json)" \
  -F "files=@examples/JohnDoe_CV.txt" \
  -F "files=@examples/JohnDoe_CoverLetter.txt" \
  -F "files=@examples/JaneSmith_CV.txt" \
  -F "files=@examples/JaneSmith_CoverLetter.txt"
```

Get the report:
```bash
curl http://localhost:8080/report | jq .
```

## Expected Results

John Doe should score higher than Jane Smith because:
- John has 7 years of experience vs Jane's 4 years
- John exceeds the required 5+ years of Go experience
- John has a Master's degree (nice-to-have)
- John has Kubernetes and cloud certifications
- John's cover letter demonstrates strong understanding of the role

Jane Smith should receive a lower score because:
- She has only 4 years of experience (below the required 5+)
- Missing several required qualifications
- Less extensive technical background
- Her cover letter acknowledges she's still building expertise

Both applicants should receive reasonable scores as they both have:
- Bachelor's degrees in relevant fields
- Go programming experience
- Microservices knowledge
- RESTful API experience
