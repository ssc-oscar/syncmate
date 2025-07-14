package rclone

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/rclone/rclone/fs"
)

// loadCredentialsFromConfig loads credentials from the config.json file
func loadCredentialsFromConfig(configPath string) (*CloudflareR2Credentials, error) {
	file, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var creds CloudflareR2Credentials
	if err := json.Unmarshal(file, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

func TestNewR2Backend_WithRealCredentials(t *testing.T) {
	configPath := "../config.json"

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Config file not found, skipping integration test")
	}

	creds, err := loadCredentialsFromConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load credentials from config: %v", err)
	}

	ctx := InjectGlobalConfig(context.Background())
	backend, err := NewR2Backend(ctx, creds)

	if err != nil {
		t.Fatalf("backend creation failed (expected if credentials are invalid): %v", err)
		// Don't fail the test here as the credentials might be test/dummy values
		return
	}

	// Verify backend implements fs.Fs interface
	var _ fs.Fs = backend

	// Test basic backend properties
	name := backend.Name()
	if name == "" {
		t.Error("Backend name should not be empty")
	}

	root := backend.Root()
	t.Logf("Backend created successfully: name=%s, root=%s", name, root)

	// Test if the backend can list files
	files, err := backend.List(context.Background(), "")
	if err != nil {
		t.Errorf("Failed to list files: %v", err)
	} else {
		t.Logf("Files listed successfully: %v", files)
	}
}
