package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds application configuration
type Config struct {
	GoogleCloudProject    string `json:"google_cloud_project"`
	GoogleCloudLocation   string `json:"google_cloud_location"`
	GoogleCredentialsPath string `json:"google_credentials_path"`
	GmailCredentialsPath  string `json:"gmail_credentials_path"`
	UploadsDir            string `json:"uploads_dir"`
}

// DefaultConfig returns a new config with default values
func DefaultConfig() *Config {
	return &Config{
		GoogleCloudLocation: "us-central1",
		UploadsDir:          "uploads",
	}
}

// GetConfigPath returns the path to the configuration file
// On Windows: %APPDATA%/CVReviewAgent/config.json
// On Unix: ~/.config/CVReviewAgent/config.json
func GetConfigPath() (string, error) {
	var configDir string

	if os.Getenv("APPDATA") != "" {
		// Windows
		configDir = filepath.Join(os.Getenv("APPDATA"), "CVReviewAgent")
	} else {
		// Unix-like systems
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		configDir = filepath.Join(homeDir, ".config", "CVReviewAgent")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return filepath.Join(configDir, "config.json"), nil
}

// Load loads configuration from the default config path
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	return LoadFrom(configPath)
}

// LoadFrom loads configuration from a specific path
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// Save saves the configuration to the default config path
func (c *Config) Save() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	return c.SaveTo(configPath)
}

// SaveTo saves the configuration to a specific path
func (c *Config) SaveTo(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.GoogleCloudProject == "" {
		return fmt.Errorf("google_cloud_project is required")
	}

	if c.GoogleCloudLocation == "" {
		return fmt.Errorf("google_cloud_location is required")
	}

	if c.GoogleCredentialsPath != "" {
		if _, err := os.Stat(c.GoogleCredentialsPath); err != nil {
			return fmt.Errorf("google credentials file not found: %w", err)
		}
	}

	if c.GmailCredentialsPath != "" {
		if _, err := os.Stat(c.GmailCredentialsPath); err != nil {
			return fmt.Errorf("gmail credentials file not found: %w", err)
		}
	}

	return nil
}

// ApplyToEnv applies configuration values to environment variables
func (c *Config) ApplyToEnv() {
	if c.GoogleCloudProject != "" {
		os.Setenv("GOOGLE_CLOUD_PROJECT", c.GoogleCloudProject)
	}
	if c.GoogleCloudLocation != "" {
		os.Setenv("GOOGLE_CLOUD_LOCATION", c.GoogleCloudLocation)
	}
	if c.GoogleCredentialsPath != "" {
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", c.GoogleCredentialsPath)
	}
}
