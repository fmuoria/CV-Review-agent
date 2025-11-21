# Windows Quick Start Guide

Get started with CV Review Agent GUI on Windows in just a few steps!

## Option 1: Using Pre-built Installer (Recommended)

### Download and Install

1. **Download the installer:**
   - Download `CVReviewAgent_Setup_1.0.0.exe` from the releases page
   - Or build it yourself (see GUI_README.md)

2. **Run the installer:**
   - Double-click the installer
   - Follow the installation wizard
   - Choose installation directory (default: `C:\Program Files\CV Review Agent`)
   - Optionally create desktop shortcut

3. **Launch the application:**
   - From Start Menu: "CV Review Agent"
   - Or from Desktop shortcut (if created)

### Initial Configuration

1. **Open Settings tab** in the application

2. **Configure Google Cloud:**
   - **Project ID**: Your Google Cloud Project ID
   - **Location**: `us-central1` (or your preferred region)
   - **Google Credentials**: Browse to your service account JSON file
   - **Gmail Credentials**: Browse to your OAuth2 credentials.json file

3. **Click "Save Settings"**

4. **Click "Test Connection"** to verify setup

## Option 2: Building from Source

### Prerequisites

1. **Install Go:**
   - Download from https://golang.org/dl/
   - Install version 1.20 or later
   - Verify: `go version`

2. **Install MinGW-w64 (for CGO):**
   - Download from https://www.mingw-w64.org/
   - Install to `C:\mingw-w64`
   - Add `C:\mingw-w64\mingw64\bin` to PATH

3. **Install Git:**
   - Download from https://git-scm.com/download/win

### Build Steps

1. **Clone repository:**
   ```powershell
   git clone https://github.com/fmuoria/CV-Review-agent.git
   cd CV-Review-agent
   ```

2. **Download dependencies:**
   ```powershell
   go mod download
   ```

3. **Build GUI application:**
   ```powershell
   scripts\build_windows.bat
   ```

4. **Run the application:**
   ```powershell
   cmd\gui\cv-review-agent-gui.exe
   ```

## Setting Up Google Cloud

### 1. Create Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create new project or select existing
3. Note your Project ID

### 2. Enable APIs

1. Enable **Vertex AI API**:
   - Go to "APIs & Services" → "Library"
   - Search for "Vertex AI API"
   - Click "Enable"

2. Enable **Gmail API** (for email processing):
   - Search for "Gmail API"
   - Click "Enable"

### 3. Create Service Account (for Vertex AI)

1. Go to "IAM & Admin" → "Service Accounts"
2. Click "Create Service Account"
3. Name: `cv-review-agent`
4. Grant role: "Vertex AI User"
5. Click "Create Key" → Choose "JSON"
6. Download and save as `google-credentials.json`

### 4. Create OAuth Credentials (for Gmail)

1. Go to "APIs & Services" → "Credentials"
2. Click "Create Credentials" → "OAuth client ID"
3. Application type: "Desktop app"
4. Name: `CV Review Agent`
5. Download and save as `credentials.json`

### 5. Configure Application

Put both JSON files in a safe location, then:
1. Open CV Review Agent
2. Go to Settings tab
3. Browse to each JSON file
4. Save settings

## First Use

### Authenticate Gmail

1. In the application, click **"Authenticate Gmail"**
2. Browser opens with Google sign-in
3. Choose your Google account
4. Grant permissions to access Gmail
5. Return to application (status shows "Authenticated")

### Process Your First Batch

1. **Enter Email Subject Filter:**
   - Example: "Job Application"
   - This filters which emails to process

2. **Fill Job Description:**
   ```
   Job Title: Senior Software Engineer
   
   Required Experience:
   5+ years of Go programming
   Microservices architecture
   RESTful API design
   
   Required Education:
   Bachelor's degree in Computer Science
   
   Required Duties:
   Design scalable systems
   Code reviews
   Mentor junior developers
   
   Nice-to-Have Experience:
   Kubernetes
   Cloud platforms (GCP/AWS)
   ```

3. **Click "Start Processing":**
   - Progress bar shows status
   - Processing takes 2-5 minutes per 100 emails
   - Can cancel anytime

4. **View Results:**
   - Table shows ranked candidates
   - Sorted by total score
   - Color-coded performance

5. **Export to Excel:**
   - Click "Export to Excel"
   - Choose save location
   - Excel file created with detailed analysis

## Troubleshooting

### "Failed to initialize LLM client"
**Solution:** 
- Verify Google Cloud credentials path is correct
- Check Vertex AI API is enabled
- Ensure service account has "Vertex AI User" role

### "Unable to retrieve messages"
**Solution:**
- Click "Authenticate Gmail" again
- Check credentials.json is in correct location
- Verify Gmail API is enabled

### "No messages found"
**Solution:**
- Check email subject filter matches actual emails
- Ensure emails have attachments
- Try a broader search term

### Application won't start
**Solution:**
- Run from command line to see error messages:
  ```powershell
  cmd\gui\cv-review-agent-gui.exe
  ```
- Check Windows Event Viewer for errors
- Verify all DLL dependencies are available

## Tips for Best Results

### Email Organization
- Have applicants use consistent subject lines
- Request specific attachment names:
  - `FirstName_LastName_CV.pdf`
  - `FirstName_LastName_CoverLetter.pdf`

### Job Descriptions
- Be specific with requirements
- Separate required from nice-to-have clearly
- Use bullet points (one per line)
- Include keywords applicants might use

### Processing Large Batches
- Start with small test batch (10-20 CVs)
- Verify results before processing hundreds
- Processing time: ~2 minutes per 10 CVs
- Large batches (500+) may take 1-2 hours

### Excel Results
- Summary sheet: Quick overview and statistics
- Ranked Candidates: At-a-glance comparison
- Detailed Analysis: Full AI reasoning
- Use Excel filters to focus on top candidates

## Next Steps

- Read [GUI_README.md](GUI_README.md) for advanced features
- Check [README.md](README.md) for API server usage
- See [SETUP.md](SETUP.md) for detailed Google Cloud setup

## Support

Having issues? 
- Check [GUI_README.md](GUI_README.md) troubleshooting section
- Open issue on [GitHub](https://github.com/fmuoria/CV-Review-agent/issues)
- Include error messages and screenshots

## Security Notes

- Keep credentials.json and google-credentials.json secure
- Don't share token.json (contains OAuth tokens)
- Configuration stored in: `%APPDATA%\CVReviewAgent\`
- No CV data is stored on external servers
- All processing is local (except API calls to Google)
