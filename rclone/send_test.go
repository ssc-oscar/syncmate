package rclone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/stretchr/testify/require"
)

// Integration test with real R2 backend (if credentials are available)
func TestCopyFiles_WithR2Backend(t *testing.T) {
	configPath := "../config.json"

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Config file not found, skipping R2 integration test")
	}

	creds, err := loadCredentialsFromConfig(configPath)
	if err != nil {
		t.Skip("Failed to load credentials from config, skipping R2 integration test")
	}

	// Create source directory with test file
	srcDir, err := os.MkdirTemp("", "syncmate_r2_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	testFile := filepath.Join(srcDir, "r2_test.txt")
	testContent := fmt.Sprintf("Test file for R2 integration at %s", time.Now().Format(time.RFC3339))
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	ctx := InjectConfig(context.Background(), []string{testFile})
	fsrc, err := fs.NewFs(ctx, srcDir)
	require.NoError(t, err)

	// Create R2 backend
	fdst, err := NewR2Backend(ctx, creds)
	if err != nil {
		t.Skipf("Failed to create R2 backend (credentials may be invalid): %v", err)
	}
	// Delete if it exists
	if err := fdst.Rmdir(ctx, "r2_test.txt"); err != nil && err != fs.ErrorObjectNotFound {
		t.Fatalf("Failed to delete existing test file in R2: %v", err)
	}

	// Test copy to R2
	err = CopyFiles(ctx, fsrc, fdst, []string{"r2_test.txt"})
	if err != nil {
		t.Logf("Copy to R2 failed (may be expected if credentials are test values): %v", err)
	} else {
		t.Log("Copy to R2 succeeded")
	}
	// Verify file exists in R2
	entries, err := fdst.List(ctx, "")
	require.NoError(t, err)
	found := false
	for _, entry := range entries {
		if entry.Remote() == "r2_test.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Test file not found in R2 backend after copy")
	} else {
		t.Log("Test file successfully copied to R2 backend")
	}
}
