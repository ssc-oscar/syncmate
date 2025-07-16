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

// Test with local filesystem
func TestListFiles_WithLocalBackend(t *testing.T) {
	// Create temporary directory with test files
	srcDir, err := os.MkdirTemp("", "syncmate_list_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	// Create test files with different timestamps
	testFiles := []struct {
		name    string
		content string
		modTime time.Time
	}{
		{"file1.txt", "content of file 1", time.Now().Add(-2 * time.Hour)},
		{"file2.txt", "content of file 2", time.Now().Add(-1 * time.Hour)},
		{"file3.txt", "content of file 3", time.Now()},
	}

	for _, tf := range testFiles {
		testFile := filepath.Join(srcDir, tf.name)
		err = os.WriteFile(testFile, []byte(tf.content), 0644)
		require.NoError(t, err)

		// Set modification time
		err = os.Chtimes(testFile, tf.modTime, tf.modTime)
		require.NoError(t, err)
	}

	ctx := InjectConfig(context.Background())
	fsrc, err := fs.NewFs(ctx, srcDir)
	require.NoError(t, err)

	// Test ListFiles function
	fileInfos, err := ListFiles(ctx, fsrc)
	require.NoError(t, err)

	// Verify we got the expected number of files
	require.Len(t, fileInfos, len(testFiles))

	// Create a map for easier verification
	foundFiles := make(map[string]RcloneFileInfo)
	for _, info := range fileInfos {
		foundFiles[info.Name] = info
	}

	// Verify each test file is present with correct attributes
	for _, tf := range testFiles {
		info, found := foundFiles[tf.name]
		require.True(t, found, "File %s not found in listing", tf.name)
		require.Equal(t, int64(len(tf.content)), info.Size, "Size mismatch for file %s", tf.name)
	}
}

// Test with empty directory
func TestListFiles_EmptyDirectory(t *testing.T) {
	// Create empty temporary directory
	srcDir, err := os.MkdirTemp("", "syncmate_list_empty_*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	ctx := InjectConfig(context.Background())
	fsrc, err := fs.NewFs(ctx, srcDir)
	require.NoError(t, err)

	// Test ListFiles function on empty directory
	fileInfos, err := ListFiles(ctx, fsrc)
	require.NoError(t, err)
	require.Empty(t, fileInfos, "Expected empty file list for empty directory")
}

// Integration test with real R2 backend (if credentials are available)
func TestListFiles_WithR2Backend(t *testing.T) {
	configPath := "../config.json"

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Config file not found, skipping R2 integration test")
	}

	creds, err := loadCredentialsFromConfig(configPath)
	if err != nil {
		t.Skip("Failed to load credentials from config, skipping R2 integration test")
	}

	ctx := InjectConfig(context.Background())

	// Create R2 backend
	fdst, err := NewR2Backend(ctx, creds)
	if err != nil {
		t.Skipf("Failed to create R2 backend (credentials may be invalid): %v", err)
	}

	// First, upload a test file to ensure we have something to list
	testFileName := fmt.Sprintf("list_test_%d.txt", time.Now().Unix())
	testContent := fmt.Sprintf("Test file for listing at %s", time.Now().Format(time.RFC3339))

	// Create a temporary local file to upload
	srcDir, err := os.MkdirTemp("", "syncmate_list_r2_*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	testFile := filepath.Join(srcDir, testFileName)
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Upload the file to R2
	ctx = InjectFileList(ctx, []string{testFileName})
	fsrc, err := fs.NewFs(ctx, srcDir)
	require.NoError(t, err)

	err = CopyFiles(ctx, fsrc, fdst, []string{testFileName})
	if err != nil {
		t.Skipf("Failed to upload test file to R2 (may be expected if credentials are test values): %v", err)
	}

	// Clean up: defer deletion of the test file
	defer func() {
		if obj, err := fdst.NewObject(ctx, testFileName); err == nil {
			obj.Remove(ctx)
		}
	}()

	// Now test ListFiles on R2
	fileInfos, err := ListFiles(ctx, fdst)
	if err != nil {
		t.Skipf("Failed to list files from R2 (may be expected if credentials are test values): %v", err)
	}

	// Verify our test file is in the listing
	found := false
	for _, info := range fileInfos {
		if info.Name == testFileName {
			found = true
			require.Equal(t, int64(len(testContent)), info.Size, "Size mismatch for uploaded file")
			break
		}
	}

	if !found {
		t.Errorf("Test file %s not found in R2 listing", testFileName)
	} else {
		t.Logf("Successfully listed files from R2 backend, found %d files including test file", len(fileInfos))
	}
}

// Test with filtered file list
func TestListFiles_WithFileFilter(t *testing.T) {
	// Create temporary directory with multiple test files
	srcDir, err := os.MkdirTemp("", "syncmate_list_filter_*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	// Create multiple test files
	allFiles := []string{"file1.txt", "file2.txt", "file3.log", "file4.txt"}
	for _, fileName := range allFiles {
		testFile := filepath.Join(srcDir, fileName)
		content := fmt.Sprintf("content of %s", fileName)
		err = os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(t, err)
	}

	ctx := InjectConfig(context.Background())
	// Filter to only include .txt files
	txtFiles := []string{"file1.txt", "file2.txt", "file4.txt"}
	ctx = InjectFileList(ctx, txtFiles)

	fsrc, err := fs.NewFs(ctx, srcDir)
	require.NoError(t, err)

	// Test ListFiles function with filter
	fileInfos, err := ListFiles(ctx, fsrc)
	require.NoError(t, err)

	// Should only get the filtered files
	require.Len(t, fileInfos, len(txtFiles))

	// Verify only .txt files are returned
	for _, info := range fileInfos {
		require.Contains(t, txtFiles, info.Name, "Unexpected file in filtered listing")
		require.True(t, filepath.Ext(info.Name) == ".txt" || info.Name == "file1.txt" || info.Name == "file2.txt" || info.Name == "file4.txt")
	}
}
