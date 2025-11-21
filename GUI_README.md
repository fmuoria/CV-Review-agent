# CV Review Agent - GUI Application

This document describes the desktop GUI application for CV Review Agent, built with the Fyne framework.

## Features

The GUI application provides:

1. **Gmail Authentication** - Easy OAuth2 authentication with visual status indicator
2. **Job Description Form** - Dynamic input fields for required and nice-to-have qualifications
3. **Email Subject Filter** - Filter emails by subject line to process specific applications
4. **Batch Processing** - Process 500+ CVs automatically with progress tracking
5. **Results Table** - View ranked candidates with scores in a sortable table
6. **Excel Export** - Export results to professionally formatted Excel file with:
   - Summary sheet with statistics
   - Ranked candidates sheet with color-coding
   - Detailed analysis sheet with full reasoning

## System Requirements

### Windows
- Windows 10 or later (64-bit)
- 4GB RAM minimum (8GB recommended)
- Internet connection for Gmail and Google Cloud API access

### Build Requirements (For Developers)
- Go 1.20 or later
- GCC compiler (MinGW-w64 for Windows)
- Fyne system dependencies

## Building from Source

### Windows Build

1. **Install Prerequisites:**
   ```batch
   # Install Go from https://golang.org/dl/
   # Install MinGW-w64 from https://www.mingw-w64.org/
   # Add MinGW-w64 bin directory to PATH
   ```

2. **Clone Repository:**
   ```batch
   git clone https://github.com/fmuoria/CV-Review-agent.git
   cd CV-Review-agent
   ```

3. **Install Dependencies:**
   ```batch
   go mod download
   ```

4. **Build GUI Application:**
   ```batch
   # Run the build script
   scripts\build_windows.bat
   
   # OR manually:
   go build -ldflags="-H windowsgui" -o cmd/gui/cv-review-agent-gui.exe cmd/gui/main.go
   ```

### Linux Build (for development/testing)

1. **Install System Dependencies:**
   ```bash
   # Ubuntu/Debian
   sudo apt-get install gcc libgl1-mesa-dev xorg-dev
   
   # Fedora
   sudo dnf install gcc mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel
   ```

2. **Build:**
   ```bash
   go build -o cv-review-agent-gui cmd/gui/main.go
   ```

### macOS Build

1. **Install Xcode Command Line Tools:**
   ```bash
   xcode-select --install
   ```

2. **Build:**
   ```bash
   go build -o cv-review-agent-gui cmd/gui/main.go
   ```

## Creating Windows Installer

1. **Install Inno Setup:**
   - Download from https://jrsoftware.org/isinfo.php
   - Install Inno Setup 6 or later

2. **Build the Application:**
   ```batch
   scripts\build_windows.bat
   ```

3. **Create Installer:**
   ```batch
   # Using Inno Setup GUI
   # Open installer\windows\setup.iss and click Compile
   
   # OR using command line
   "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\windows\setup.iss
   ```

4. **Distribute:**
   - The installer will be created in `installer\windows\output\`
   - File name: `CVReviewAgent_Setup_1.0.0.exe`

## Configuration

### First Run

On first run, configure the application:

1. **Open Settings Tab**
2. **Configure Google Cloud:**
   - Google Cloud Project ID
   - Google Cloud Location (default: us-central1)
   - Path to Google Cloud credentials JSON file
   - Path to Gmail OAuth credentials JSON file

3. **Save Settings**
   - Configuration is saved to `%APPDATA%\CVReviewAgent\config.json`

### Gmail OAuth Setup

1. **Create OAuth Credentials:**
   - Go to [Google Cloud Console](https://console.cloud.google.com)
   - Enable Gmail API
   - Create OAuth 2.0 Client ID (Desktop app type)
   - Download as `credentials.json`

2. **Place Credentials:**
   - Put `credentials.json` in the application directory
   - Or specify path in Settings

3. **Authenticate:**
   - Click "Authenticate Gmail" button
   - Browser window opens for OAuth consent
   - Grant permissions
   - Token saved to `token.json`

## Usage

### Processing CVs from Gmail

1. **Authenticate Gmail** (if not already done)

2. **Enter Email Subject Filter:**
   - e.g., "Job Application"
   - This filters which emails to process

3. **Fill Job Description:**
   - Job Title
   - Job Description
   - Required qualifications (experience, education, duties)
   - Nice-to-have qualifications

4. **Click "Start Processing":**
   - Progress bar shows current status
   - Can take several minutes for large batches
   - Click "Cancel" to stop processing

5. **View Results:**
   - Ranked candidates table shows scores
   - Color-coded by performance
   - Sorted by total score

6. **Export to Excel:**
   - Click "Export to Excel"
   - Choose save location
   - Excel file generated with all details

## Excel Export Format

The exported Excel file contains three sheets:

### 1. Summary Sheet
- Job title and generation date
- Total candidates processed
- Statistics breakdown:
  - Excellent (90-100): Green
  - Good (70-89): Yellow
  - Fair (50-69): Orange
  - Poor (<50): Red
- Average score

### 2. Ranked Candidates Sheet
- Rank, Name, Total Score
- Individual scores (Experience, Education, Duties, Cover Letter)
- Color-coded rows based on total score
- Auto-filter enabled
- Frozen header row

### 3. Detailed Analysis Sheet
- Full reasoning for each score component
- Each candidate has 4 rows (one per category)
- Text wrapping for readability
- Detailed explanations from AI evaluation

## Troubleshooting

### Build Issues

**"Package gl was not found":**
- Install OpenGL development libraries
- Windows: Included with MinGW-w64
- Linux: Install mesa-libGL-devel or libgl1-mesa-dev

**"X11/Xlib.h: No such file or directory":**
- Linux only: Install X11 development headers
- `sudo apt-get install xorg-dev`

### Runtime Issues

**"Failed to initialize LLM client":**
- Verify Google Cloud credentials are correct
- Ensure GOOGLE_APPLICATION_CREDENTIALS is set
- Check VertexAI API is enabled in GCP

**"Unable to retrieve messages":**
- Check Gmail OAuth credentials are valid
- Re-authenticate if token expired
- Verify Gmail API is enabled

**"No documents found":**
- Check email subject filter is correct
- Ensure emails have attachments
- Verify attachments are PDF, DOC, or DOCX

## File Locations

### Windows
- Configuration: `%APPDATA%\CVReviewAgent\config.json`
- Application: `C:\Program Files\CV Review Agent\`
- Uploads: `uploads\` (in application directory)

### Linux/macOS
- Configuration: `~/.config/CVReviewAgent/config.json`
- Uploads: `uploads\` (in working directory)

## Performance

### Email Processing
- 100 emails: ~2-3 minutes
- 500 emails: ~10-15 minutes
- Depends on:
  - Attachment sizes
  - Network speed
  - Gmail API rate limits

### CV Evaluation
- 10 candidates: ~2-3 minutes
- 50 candidates: ~10-15 minutes
- 100 candidates: ~20-30 minutes
- Depends on:
  - LLM response time
  - CV length and complexity
  - Available VertexAI quota

## Security Notes

- OAuth tokens stored in `token.json` (keep secure)
- Credentials never transmitted to third parties
- All processing done locally except:
  - Gmail API calls (to Google)
  - LLM API calls (to Google VertexAI)
- No data stored on external servers

## Support

For issues, questions, or feature requests:
- GitHub Issues: https://github.com/fmuoria/CV-Review-agent/issues
- Documentation: See README.md for API details

## License

This project is licensed under the MIT License - see LICENSE file for details.
