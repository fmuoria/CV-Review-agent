# Setup Guide

This guide will help you set up the CV Review Agent for local development and testing.

## Prerequisites

1. **Go 1.20 or higher**
   ```bash
   go version  # Should show 1.20 or higher
   ```

2. **Google Cloud Project**
   - Create a project at [Google Cloud Console](https://console.cloud.google.com)
   - Note your project ID

3. **(Optional) jq for JSON parsing**
   ```bash
   # macOS
   brew install jq
   
   # Ubuntu/Debian
   sudo apt-get install jq
   ```

## Step 1: Clone the Repository

```bash
git clone https://github.com/fmuoria/CV-Review-agent.git
cd CV-Review-agent
```

## Step 2: Install Dependencies

```bash
make deps
# or
go mod download
go mod tidy
```

## Step 3: Set Up Google Cloud VertexAI

### Enable VertexAI API

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Select your project
3. Navigate to "APIs & Services" > "Library"
4. Search for "Vertex AI API"
5. Click "Enable"

### Create Service Account

1. Go to "IAM & Admin" > "Service Accounts"
2. Click "Create Service Account"
3. Name: `cv-review-agent`
4. Click "Create and Continue"
5. Grant roles:
   - Vertex AI User
   - Vertex AI Service Agent
6. Click "Continue" and "Done"

### Generate Service Account Key

1. Click on the created service account
2. Go to "Keys" tab
3. Click "Add Key" > "Create new key"
4. Choose JSON format
5. Download the key file
6. Save it securely (e.g., `~/keys/cv-review-agent-key.json`)

### Set Environment Variables

```bash
export GOOGLE_CLOUD_PROJECT="your-project-id"
export GOOGLE_CLOUD_LOCATION="us-central1"
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/keys/cv-review-agent-key.json"
```

Add these to your `~/.bashrc` or `~/.zshrc` to persist:

```bash
echo 'export GOOGLE_CLOUD_PROJECT="your-project-id"' >> ~/.bashrc
echo 'export GOOGLE_CLOUD_LOCATION="us-central1"' >> ~/.bashrc
echo 'export GOOGLE_APPLICATION_CREDENTIALS="$HOME/keys/cv-review-agent-key.json"' >> ~/.bashrc
source ~/.bashrc
```

## Step 4: (Optional) Set Up Gmail Integration

If you want to use the Gmail ingestion feature:

### Enable Gmail API

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Navigate to "APIs & Services" > "Library"
3. Search for "Gmail API"
4. Click "Enable"

### Create OAuth 2.0 Credentials

1. Go to "APIs & Services" > "Credentials"
2. Click "Create Credentials" > "OAuth client ID"
3. If prompted, configure OAuth consent screen:
   - User Type: External (for testing)
   - App name: CV Review Agent
   - User support email: your email
   - Developer contact: your email
   - Click "Save and Continue"
   - Scopes: Leave default, click "Save and Continue"
   - Test users: Add your Gmail address
   - Click "Save and Continue"
4. Back to OAuth client ID creation:
   - Application type: Desktop app
   - Name: CV Review Agent
   - Click "Create"
5. Download the credentials JSON file
6. Rename it to `credentials.json`
7. Place it in the project root directory

### Test Gmail Authorization

On first run with Gmail method, you'll be prompted to authorize:

```bash
# The application will print a URL
# Visit the URL in your browser
# Sign in with your Gmail account
# Copy the authorization code
# Paste it back into the terminal
```

The token will be saved in `token.json` for future use.

## Step 5: Verify Installation

```bash
# Build the project
make build

# Check the binary was created
ls -lh cv-review-agent
```

## Step 6: Run the Server

```bash
make run
# or
go run main.go
```

You should see:
```
Starting CV Review Agent on port 8080...
Endpoints:
  POST /ingest - Upload documents or fetch from Gmail
  GET /report - Get ranked applicant results
```

## Step 7: Test the Application

In another terminal:

```bash
# Test health endpoint
curl http://localhost:8080/health

# Run the example test
./examples/test_upload.sh
```

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GOOGLE_CLOUD_PROJECT` | Yes | - | Your GCP project ID |
| `GOOGLE_CLOUD_LOCATION` | No | `us-central1` | VertexAI API location |
| `GOOGLE_APPLICATION_CREDENTIALS` | Yes | - | Path to service account key |
| `PORT` | No | `8080` | Server port |

## Troubleshooting

### "GOOGLE_CLOUD_PROJECT environment variable not set"

**Solution:** Set the environment variable:
```bash
export GOOGLE_CLOUD_PROJECT="your-project-id"
```

### "failed to create Vertex AI client: rpc error"

**Solutions:**
1. Check VertexAI API is enabled in your GCP project
2. Verify service account has correct permissions
3. Ensure `GOOGLE_APPLICATION_CREDENTIALS` points to valid key file
4. Check your GCP project has billing enabled

### "unable to read credentials file"

For Gmail integration:

**Solution:** 
1. Ensure `credentials.json` exists in project root
2. Check file permissions: `chmod 600 credentials.json`
3. Re-download credentials from GCP Console if corrupted

### "no messages found with subject"

**Solutions:**
1. Verify emails exist in your Gmail with that exact subject
2. Check emails have attachments
3. Ensure your Gmail account was added as a test user in OAuth consent screen

### Server won't start / Port already in use

**Solution:** Change the port:
```bash
PORT=8081 go run main.go
```

### Files not being processed

**Solutions:**
1. Check file naming convention: `Name_CV.ext` or `Name_CoverLetter.ext`
2. Verify file extensions are supported (.pdf, .txt, .doc, .docx)
3. Check files are in the `uploads/` directory

## Next Steps

1. Read the [main README](README.md) for usage instructions
2. Check the [examples/](examples/) directory for sample files
3. Customize the job description JSON for your use case
4. Start uploading CVs and getting evaluations!

## Security Best Practices

1. **Never commit credentials to version control**
   - `credentials.json` and `token.json` are in `.gitignore`
   - Service account keys should never be committed

2. **Restrict service account permissions**
   - Only grant minimum necessary roles
   - Regularly rotate service account keys

3. **Use environment variables for secrets**
   - Don't hardcode credentials in code
   - Consider using secret management services in production

4. **Limit OAuth scope**
   - Gmail integration only needs `gmail.readonly` scope
   - Don't grant unnecessary permissions

## Additional Resources

- [VertexAI Documentation](https://cloud.google.com/vertex-ai/docs)
- [Gmail API Documentation](https://developers.google.com/gmail/api)
- [Go OAuth2 Package](https://pkg.go.dev/golang.org/x/oauth2)
- [Gemini API Documentation](https://cloud.google.com/vertex-ai/docs/generative-ai/model-reference/gemini)
