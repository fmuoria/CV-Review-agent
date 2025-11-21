package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/fmuoria/CV-Review-agent/internal/agent"
	"github.com/fmuoria/CV-Review-agent/internal/config"
	"github.com/fmuoria/CV-Review-agent/internal/export"
	"github.com/fmuoria/CV-Review-agent/internal/ingestion"
	"github.com/fmuoria/CV-Review-agent/internal/models"
)

const (
	// gmailCredentialsFilename is the expected filename for Gmail API credentials
	gmailCredentialsFilename = "credentials.json"
)

// App represents the main GUI application
type App struct {
	fyneApp    fyne.App
	mainWindow fyne.Window
	config     *config.Config
	agent      *agent.CVReviewAgent
	ctx        context.Context
	cancelFunc context.CancelFunc

	// UI Components
	gmailStatusLabel     *widget.Label
	authenticateBtn      *widget.Button
	subjectEntry         *widget.Entry
	jobTitleEntry        *widget.Entry
	requiredExpText      *widget.Entry
	requiredEduText      *widget.Entry
	requiredDutiesText   *widget.Entry
	niceToHaveExpText    *widget.Entry
	niceToHaveEduText    *widget.Entry
	niceToHaveDutiesText *widget.Entry
	jobDescText          *widget.Entry
	processBtn           *widget.Button
	cancelBtn            *widget.Button
	progressBar          *widget.ProgressBar
	progressLabel        *widget.Label
	resultsTable         *widget.Table
	exportBtn            *widget.Button

	results []models.ApplicantResult
}

// NewApp creates a new GUI application
func NewApp() *App {
	a := app.New()
	w := a.NewWindow("CV Review Agent")
	w.Resize(fyne.NewSize(1000, 700))

	guiApp := &App{
		fyneApp:    a,
		mainWindow: w,
		agent:      agent.NewCVReviewAgent(),
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load configuration: %v", err)
		cfg = config.DefaultConfig()
	}
	guiApp.config = cfg

	// Apply config to environment
	cfg.ApplyToEnv()

	// Setup UI
	guiApp.setupUI()

	return guiApp
}

// Run starts the GUI application
func (a *App) Run() {
	a.mainWindow.ShowAndRun()
}

// setupUI initializes all UI components
func (a *App) setupUI() {
	// Create tabs
	tabs := container.NewAppTabs(
		container.NewTabItem("Process CVs", a.createProcessTab()),
		container.NewTabItem("Settings", a.createSettingsTab()),
	)

	a.mainWindow.SetContent(tabs)
}

// createProcessTab creates the main processing tab
func (a *App) createProcessTab() fyne.CanvasObject {
	// Gmail authentication section
	a.gmailStatusLabel = widget.NewLabel("Gmail: Not Authenticated")
	a.authenticateBtn = widget.NewButton("Authenticate Gmail", a.handleAuthenticate)

	authSection := container.NewVBox(
		widget.NewLabel("Gmail Authentication"),
		container.NewHBox(a.gmailStatusLabel, a.authenticateBtn),
	)

	// Email filter section
	a.subjectEntry = widget.NewEntry()
	a.subjectEntry.SetPlaceHolder("e.g., Job Application")

	filterSection := container.NewVBox(
		widget.NewLabel("Email Subject Filter"),
		a.subjectEntry,
	)

	// Job description section
	a.jobTitleEntry = widget.NewEntry()
	a.jobTitleEntry.SetPlaceHolder("e.g., Senior Software Engineer")

	a.jobDescText = widget.NewMultiLineEntry()
	a.jobDescText.SetPlaceHolder("Enter detailed job description...")
	a.jobDescText.SetMinRowsVisible(3)

	a.requiredExpText = widget.NewMultiLineEntry()
	a.requiredExpText.SetPlaceHolder("One per line, e.g.:\n5+ years of Go programming\nMicroservices architecture")
	a.requiredExpText.SetMinRowsVisible(3)

	a.requiredEduText = widget.NewMultiLineEntry()
	a.requiredEduText.SetPlaceHolder("One per line, e.g.:\nBachelor's degree in Computer Science")
	a.requiredEduText.SetMinRowsVisible(2)

	a.requiredDutiesText = widget.NewMultiLineEntry()
	a.requiredDutiesText.SetPlaceHolder("One per line, e.g.:\nDesign scalable systems\nCode reviews")
	a.requiredDutiesText.SetMinRowsVisible(3)

	a.niceToHaveExpText = widget.NewMultiLineEntry()
	a.niceToHaveExpText.SetPlaceHolder("One per line, e.g.:\nKubernetes experience\nCloud platforms")
	a.niceToHaveExpText.SetMinRowsVisible(2)

	a.niceToHaveEduText = widget.NewMultiLineEntry()
	a.niceToHaveEduText.SetPlaceHolder("One per line, e.g.:\nMaster's degree")
	a.niceToHaveEduText.SetMinRowsVisible(2)

	a.niceToHaveDutiesText = widget.NewMultiLineEntry()
	a.niceToHaveDutiesText.SetPlaceHolder("One per line, e.g.:\nMentor junior developers")
	a.niceToHaveDutiesText.SetMinRowsVisible(2)

	jobSection := container.NewVBox(
		widget.NewLabel("Job Description"),
		widget.NewForm(
			widget.NewFormItem("Job Title", a.jobTitleEntry),
			widget.NewFormItem("Description", a.jobDescText),
			widget.NewFormItem("Required Experience", a.requiredExpText),
			widget.NewFormItem("Required Education", a.requiredEduText),
			widget.NewFormItem("Required Duties", a.requiredDutiesText),
			widget.NewFormItem("Nice-to-Have Experience", a.niceToHaveExpText),
			widget.NewFormItem("Nice-to-Have Education", a.niceToHaveEduText),
			widget.NewFormItem("Nice-to-Have Duties", a.niceToHaveDutiesText),
		),
	)

	// Progress section
	a.progressBar = widget.NewProgressBar()
	a.progressLabel = widget.NewLabel("Ready")
	a.processBtn = widget.NewButton("Start Processing", a.handleProcess)
	a.cancelBtn = widget.NewButton("Cancel", a.handleCancel)
	a.cancelBtn.Disable()

	progressSection := container.NewVBox(
		a.progressLabel,
		a.progressBar,
		container.NewHBox(a.processBtn, a.cancelBtn),
	)

	// Results section
	a.resultsTable = widget.NewTable(
		func() (int, int) {
			return len(a.results) + 1, 6 // +1 for header
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("Template")
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)
			if id.Row == 0 {
				// Header
				headers := []string{"Rank", "Name", "Total Score", "Experience", "Education", "Duties"}
				if id.Col < len(headers) {
					label.SetText(headers[id.Col])
					label.TextStyle = fyne.TextStyle{Bold: true}
				}
			} else if id.Row-1 < len(a.results) {
				result := a.results[id.Row-1]
				switch id.Col {
				case 0:
					label.SetText(fmt.Sprintf("%d", result.Rank))
				case 1:
					label.SetText(result.Name)
				case 2:
					label.SetText(fmt.Sprintf("%.2f", result.Scores.TotalScore))
				case 3:
					label.SetText(fmt.Sprintf("%.2f", result.Scores.ExperienceScore))
				case 4:
					label.SetText(fmt.Sprintf("%.2f", result.Scores.EducationScore))
				case 5:
					label.SetText(fmt.Sprintf("%.2f", result.Scores.DutiesScore))
				}
			}
		},
	)
	a.resultsTable.SetColumnWidth(0, 60)
	a.resultsTable.SetColumnWidth(1, 200)
	a.resultsTable.SetColumnWidth(2, 100)
	a.resultsTable.SetColumnWidth(3, 100)
	a.resultsTable.SetColumnWidth(4, 100)
	a.resultsTable.SetColumnWidth(5, 100)

	a.exportBtn = widget.NewButton("Export to Excel", a.handleExport)
	a.exportBtn.Disable()

	resultsSection := container.NewVBox(
		widget.NewLabel("Results"),
		container.NewScroll(a.resultsTable),
		a.exportBtn,
	)

	// Main layout with scrolling
	content := container.NewVScroll(
		container.NewVBox(
			authSection,
			widget.NewSeparator(),
			filterSection,
			widget.NewSeparator(),
			jobSection,
			widget.NewSeparator(),
			progressSection,
			widget.NewSeparator(),
			resultsSection,
		),
	)

	return content
}

// createSettingsTab creates the settings tab
func (a *App) createSettingsTab() fyne.CanvasObject {
	projectEntry := widget.NewEntry()
	projectEntry.SetText(a.config.GoogleCloudProject)

	locationEntry := widget.NewEntry()
	locationEntry.SetText(a.config.GoogleCloudLocation)

	googleCredsEntry := widget.NewEntry()
	googleCredsEntry.SetText(a.config.GoogleCredentialsPath)

	gmailCredsEntry := widget.NewEntry()
	gmailCredsEntry.SetText(a.config.GmailCredentialsPath)

	googleCredsBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err == nil && uc != nil {
				googleCredsEntry.SetText(uc.URI().Path())
				uc.Close()
			}
		}, a.mainWindow)
	})

	gmailCredsBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err == nil && uc != nil {
				gmailCredsEntry.SetText(uc.URI().Path())
				uc.Close()
			}
		}, a.mainWindow)
	})

	form := widget.NewForm(
		widget.NewFormItem("Google Cloud Project", projectEntry),
		widget.NewFormItem("Google Cloud Location", locationEntry),
		widget.NewFormItem("Google Credentials", container.NewBorder(nil, nil, nil, googleCredsBtn, googleCredsEntry)),
		widget.NewFormItem("Gmail Credentials", container.NewBorder(nil, nil, nil, gmailCredsBtn, gmailCredsEntry)),
	)

	saveBtn := widget.NewButton("Save Settings", func() {
		a.config.GoogleCloudProject = projectEntry.Text
		a.config.GoogleCloudLocation = locationEntry.Text
		a.config.GoogleCredentialsPath = googleCredsEntry.Text
		a.config.GmailCredentialsPath = gmailCredsEntry.Text

		if err := a.config.Save(); err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}

		// Apply to environment
		a.config.ApplyToEnv()

		dialog.ShowInformation("Success", "Settings saved successfully", a.mainWindow)
	})

	testBtn := widget.NewButton("Test Connection", func() {
		if err := a.config.Validate(); err != nil {
			dialog.ShowError(fmt.Errorf("validation failed: %w", err), a.mainWindow)
			return
		}
		dialog.ShowInformation("Success", "Configuration is valid", a.mainWindow)
	})

	return container.NewVBox(
		form,
		container.NewHBox(saveBtn, testBtn),
	)
}

// handleAuthenticate handles Gmail authentication
func (a *App) handleAuthenticate() {
	// Check if credentials file exists
	credsPath := a.config.GmailCredentialsPath
	if credsPath == "" {
		credsPath = gmailCredentialsFilename
	}

	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		dialog.ShowError(fmt.Errorf("%s not found. Please configure Gmail credentials in Settings", gmailCredentialsFilename), a.mainWindow)
		return
	}

	// Show loading dialog
	progressDialog := dialog.NewCustomWithoutButtons("Authenticating",
		widget.NewLabel("Authenticating with Gmail...\nCheck the console for the OAuth URL if your browser doesn't open."),
		a.mainWindow)
	progressDialog.Show()

	// Disable authenticate button
	a.authenticateBtn.Disable()

	// Run authentication in background
	go func() {
		// Handle credentials path - Gmail handler expects credentials.json in current directory
		needsCleanup := false
		if credsPath != gmailCredentialsFilename {
			// Check if credentials.json already exists in current directory
			if stat, err := os.Stat(gmailCredentialsFilename); err != nil {
				if !os.IsNotExist(err) {
					// An error other than "not exists" occurred
					log.Printf("Failed to stat %s: %v", gmailCredentialsFilename, err)
					fyne.Do(func() {
						progressDialog.Hide()
						a.authenticateBtn.Enable()
						dialog.ShowError(fmt.Errorf("failed to check credentials file: %w", err), a.mainWindow)
					})
					return
				}
				// File doesn't exist, we need to copy it
				data, err := os.ReadFile(credsPath)
				if err != nil {
					log.Printf("Failed to read credentials from %s: %v", credsPath, err)
					fyne.Do(func() {
						progressDialog.Hide()
						a.authenticateBtn.Enable()
						dialog.ShowError(fmt.Errorf("failed to read credentials file: %w", err), a.mainWindow)
					})
					return
				}

				if err := os.WriteFile(gmailCredentialsFilename, data, 0600); err != nil {
					log.Printf("Failed to write temporary %s: %v", gmailCredentialsFilename, err)
					fyne.Do(func() {
						progressDialog.Hide()
						a.authenticateBtn.Enable()
						dialog.ShowError(fmt.Errorf("failed to create temporary credentials file: %w", err), a.mainWindow)
					})
					return
				}
				needsCleanup = true
			} else if stat.IsDir() {
				// credentials.json exists but is a directory
				log.Printf("%s exists but is a directory", gmailCredentialsFilename)
				fyne.Do(func() {
					progressDialog.Hide()
					a.authenticateBtn.Enable()
					dialog.ShowError(fmt.Errorf("%s is a directory, not a file", gmailCredentialsFilename), a.mainWindow)
				})
				return
			}
		}

		// Ensure cleanup happens even if OAuth flow fails
		defer func() {
			if needsCleanup {
				if err := os.Remove(gmailCredentialsFilename); err != nil {
					log.Printf("Warning: Failed to clean up temporary %s: %v", gmailCredentialsFilename, err)
				}
			}
		}()

		// Try to create Gmail handler (this will trigger OAuth flow if token.json doesn't exist)
		// The handler creation verifies that authentication works
		uploadsDir := a.config.UploadsDir
		if uploadsDir == "" {
			uploadsDir = "uploads"
		}

		_, err := ingestion.NewGmailHandlerWithCallback(uploadsDir, nil)

		// All UI updates must be done on the main thread using fyne.Do
		if err != nil {
			fyne.Do(func() {
				progressDialog.Hide()
				a.authenticateBtn.Enable()
				dialog.ShowError(fmt.Errorf("authentication failed: %w", err), a.mainWindow)
			})
			return
		}

		// Update UI on main thread
		fyne.Do(func() {
			progressDialog.Hide()
			a.authenticateBtn.Enable()
			a.gmailStatusLabel.SetText("Gmail: Authenticated")
			dialog.ShowInformation("Success", "Gmail authenticated successfully!\nYou can now process CVs from Gmail.", a.mainWindow)
		})
	}()
}

// handleProcess handles the processing of CVs
func (a *App) handleProcess() {
	// Validate inputs
	if a.subjectEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("please enter an email subject filter"), a.mainWindow)
		return
	}

	if a.jobTitleEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("please enter a job title"), a.mainWindow)
		return
	}

	// Build job description
	jobDesc := models.JobDescription{
		Title:                a.jobTitleEntry.Text,
		Description:          a.jobDescText.Text,
		RequiredExperience:   splitLines(a.requiredExpText.Text),
		RequiredEducation:    splitLines(a.requiredEduText.Text),
		RequiredDuties:       splitLines(a.requiredDutiesText.Text),
		NiceToHaveExperience: splitLines(a.niceToHaveExpText.Text),
		NiceToHaveEducation:  splitLines(a.niceToHaveEduText.Text),
		NiceToHaveDuties:     splitLines(a.niceToHaveDutiesText.Text),
	}

	jobDescJSON, err := json.Marshal(jobDesc)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to marshal job description: %w", err), a.mainWindow)
		return
	}

	// Disable buttons
	a.processBtn.Disable()
	a.cancelBtn.Enable()
	a.exportBtn.Disable()

	// Create cancellable context
	a.ctx, a.cancelFunc = context.WithCancel(context.Background())

	// Set progress callback
	a.agent.SetProgressCallback(func(current, total int, message string) {
		fyne.Do(func() {
			a.progressBar.SetValue(float64(current) / float64(total))
			a.progressLabel.SetText(message)
		})
	})

	// Process in background
	go func() {
		err := a.agent.IngestFromGmailWithContext(a.ctx, a.subjectEntry.Text, string(jobDescJSON))

		// Wrap ALL UI updates in fyne.Do()
		fyne.Do(func() {
			a.processBtn.Enable()
			a.cancelBtn.Disable()

			if err != nil {
				if err == context.Canceled {
					a.progressLabel.SetText("Processing canceled")
				} else {
					a.progressLabel.SetText("Error: " + err.Error())
					dialog.ShowError(err, a.mainWindow)
				}
				return
			}

			// Get results and update UI
			a.results = a.agent.GetResults()
			a.resultsTable.Refresh()
			a.exportBtn.Enable()

			a.progressLabel.SetText(fmt.Sprintf("Complete! Processed %d candidates", len(a.results)))

			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   "Processing Complete",
				Content: fmt.Sprintf("Successfully processed %d candidates", len(a.results)),
			})
		})
	}()
}

// handleCancel handles cancellation of processing
func (a *App) handleCancel() {
	if a.cancelFunc != nil {
		a.cancelFunc()
		a.progressLabel.SetText("Canceling...")
	}
}

// handleExport handles exporting results to Excel
func (a *App) handleExport() {
	if len(a.results) == 0 {
		dialog.ShowError(fmt.Errorf("no results to export"), a.mainWindow)
		return
	}

	// Create default filename with timestamp
	timestamp := time.Now().Format("2006-01-02_150405")
	defaultName := fmt.Sprintf("CV_Review_Results_%s.xlsx", timestamp)

	// Show save dialog
	dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}
		if uc == nil {
			return // User canceled
		}
		defer uc.Close()

		outputPath := uc.URI().Path()

		// Export to Excel
		jobDesc := a.agent.GetJobDescription()
		if err := export.ExportToExcel(a.results, jobDesc, outputPath); err != nil {
			dialog.ShowError(fmt.Errorf("failed to export: %w", err), a.mainWindow)
			return
		}

		dialog.ShowInformation("Success", "Results exported successfully to "+filepath.Base(outputPath), a.mainWindow)
	}, a.mainWindow)
}

// splitLines splits text by newlines and filters empty lines
func splitLines(text string) []string {
	if text == "" {
		return []string{}
	}

	lines := []string{}
	for _, line := range splitByNewline(text) {
		trimmed := trimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// Helper functions to avoid importing strings package issues
func splitByNewline(s string) []string {
	var lines []string
	var current string
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, current)
			current = ""
		} else if s[i] != '\r' {
			current += string(s[i])
		}
	}
	if current != "" || len(s) > 0 {
		lines = append(lines, current)
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
