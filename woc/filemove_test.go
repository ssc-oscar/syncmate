package woc

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelper provides common functionality for file move tests
type FileMoveTestHelper struct {
	tempDir string
	t       *testing.T
}

func NewFileMoveTestHelper(t *testing.T) *FileMoveTestHelper {
	tempDir, err := ioutil.TempDir("", "filemove_test_")
	require.NoError(t, err, "Failed to create temp directory")

	return &FileMoveTestHelper{
		tempDir: tempDir,
		t:       t,
	}
}

func (th *FileMoveTestHelper) Cleanup() {
	os.RemoveAll(th.tempDir)
}

func (th *FileMoveTestHelper) CreateTestFile(name string, content string) string {
	filePath := filepath.Join(th.tempDir, name)
	require.NoError(th.t, os.MkdirAll(filepath.Dir(filePath), 0755))
	require.NoError(th.t, ioutil.WriteFile(filePath, []byte(content), 0644))
	return filePath
}

func (th *FileMoveTestHelper) ReadFile(path string) string {
	content, err := ioutil.ReadFile(path)
	require.NoError(th.t, err, "Failed to read file %s", path)
	return string(content)
}

func (th *FileMoveTestHelper) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (th *FileMoveTestHelper) GetTempPath(name string) string {
	return filepath.Join(th.tempDir, name)
}

func (th *FileMoveTestHelper) GetFileMD5(path string) string {
	result, err := SampleMD5(path, 0, 0)
	require.NoError(th.t, err, "Failed to calculate MD5 for %s", path)
	return result.Digest
}

// Test basic file move with overwrite mode
func TestMoveFile_OverwriteMode(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	// Create source file
	srcContent := "Hello, World!"
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.GetTempPath("destination.txt")

	// Calculate expected MD5
	expectedMD5 := th.GetFileMD5(srcPath)

	// Perform move operation
	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, expectedMD5, -1)
	assert.NoError(t, err, "Move operation should succeed")

	// Verify source file is deleted
	assert.False(t, th.FileExists(srcPath), "Source file should be deleted after move")

	// Verify destination file exists and has correct content
	assert.True(t, th.FileExists(dstPath), "Destination file should exist")
	assert.Equal(t, srcContent, th.ReadFile(dstPath), "Destination content should match source")
}

// Test file move with append mode
func TestMoveFile_AppendMode(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	// Create source and destination files
	srcContent := "World!"
	dstContent := "Hello, "
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.CreateTestFile("destination.txt", dstContent)

	// Get destination size before append
	dstStat, err := os.Stat(dstPath)
	require.NoError(t, err)
	expectedDstSizeBeforeTransfer := dstStat.Size()

	// Calculate expected MD5 after append
	expectedContent := dstContent + srcContent
	tempFile := th.CreateTestFile("temp_for_md5.txt", expectedContent)
	expectedMD5 := th.GetFileMD5(tempFile)

	// Perform move operation
	err = MoveFile(srcPath, dstPath, CopyModeAppend, expectedMD5, expectedDstSizeBeforeTransfer)
	assert.NoError(t, err, "Move operation should succeed")

	// Verify source file is deleted
	assert.False(t, th.FileExists(srcPath), "Source file should be deleted after move")

	// Verify destination file has appended content
	assert.Equal(t, expectedContent, th.ReadFile(dstPath), "Destination should contain appended content")
}

// Test move with incorrect MD5 digest (should fail)
func TestMoveFile_IncorrectMD5(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcContent := "Test content"
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.GetTempPath("destination.txt")

	// Use incorrect MD5
	incorrectMD5 := "incorrectmd5hash"

	// Move should fail
	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, incorrectMD5, -1)
	assert.Error(t, err, "Move should fail with incorrect MD5")
	assert.Contains(t, err.Error(), "digest mismatch", "Error should mention digest mismatch")

	// Source file should still exist since move failed
	assert.True(t, th.FileExists(srcPath), "Source file should still exist after failed move")
	assert.False(t, th.FileExists(dstPath), "Destination file should not exist after failed move")
}

// Test append mode with incorrect expected destination size
func TestMoveFile_IncorrectDestinationSize(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcContent := "World!"
	dstContent := "Hello, "
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.CreateTestFile("destination.txt", dstContent)

	// Use incorrect expected size
	incorrectSize := int64(999)

	// Move should fail
	err := MoveFile(srcPath, dstPath, CopyModeAppend, "", incorrectSize)
	assert.Error(t, err, "Move should fail with incorrect destination size")
	assert.Contains(t, err.Error(), "size mismatch", "Error should mention size mismatch")

	// Both files should still exist since move failed
	assert.True(t, th.FileExists(srcPath), "Source file should still exist after failed move")
	assert.True(t, th.FileExists(dstPath), "Destination file should still exist after failed move")
	assert.Equal(t, dstContent, th.ReadFile(dstPath), "Destination content should be unchanged")
}

// Test append mode with MD5 verification failure and rollback
func TestMoveFile_AppendModeRollback(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcContent := "New content"
	dstContent := "Original content"
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.CreateTestFile("destination.txt", dstContent)

	// Get destination size before append
	dstStat, err := os.Stat(dstPath)
	require.NoError(t, err)
	expectedDstSizeBeforeTransfer := dstStat.Size()

	// Use incorrect MD5 to trigger rollback
	incorrectMD5 := "incorrectmd5hash"

	// Move should fail and rollback
	err = MoveFile(srcPath, dstPath, CopyModeAppend, incorrectMD5, expectedDstSizeBeforeTransfer)
	assert.Error(t, err, "Move should fail with incorrect MD5")
	assert.Contains(t, err.Error(), "digest mismatch", "Error should mention digest mismatch")

	// Source file should still exist since move failed
	assert.True(t, th.FileExists(srcPath), "Source file should still exist after failed move")

	// Destination file should be rolled back to original content
	assert.Equal(t, dstContent, th.ReadFile(dstPath), "Destination should be rolled back to original content")
}

// Test move with non-existent source file
func TestMoveFile_NonExistentSource(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcPath := th.GetTempPath("nonexistent.txt")
	dstPath := th.GetTempPath("destination.txt")

	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, "", -1)
	assert.Error(t, err, "Move should fail for non-existent source")
	assert.Contains(t, err.Error(), "unable to get source file info", "Error should mention source file info")
}

// Test move with directory as source (should fail)
func TestMoveFile_DirectoryAsSource(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	// Create a directory
	srcPath := th.GetTempPath("source_dir")
	require.NoError(t, os.MkdirAll(srcPath, 0755))

	dstPath := th.GetTempPath("destination.txt")

	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, "", -1)
	assert.Error(t, err, "Move should fail for directory source")
	assert.Contains(t, err.Error(), "not a regular file", "Error should mention regular file requirement")
}

// Test overwrite mode replacing existing destination file
func TestMoveFile_OverwriteExistingFile(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcContent := "New content"
	dstContent := "Old content"
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.CreateTestFile("destination.txt", dstContent)

	expectedMD5 := th.GetFileMD5(srcPath)

	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, expectedMD5, -1)
	assert.NoError(t, err, "Move should succeed")

	// Verify destination is overwritten
	assert.Equal(t, srcContent, th.ReadFile(dstPath), "Destination should be overwritten with source content")
	assert.False(t, th.FileExists(srcPath), "Source should be deleted")
}

// Test move without MD5 verification (empty digest)
func TestMoveFile_NoMD5Verification(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcContent := "Content without MD5 check"
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.GetTempPath("destination.txt")

	// Move without MD5 verification (empty string)
	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, "", -1)
	assert.NoError(t, err, "Move should succeed without MD5 verification")

	assert.False(t, th.FileExists(srcPath), "Source should be deleted")
	assert.True(t, th.FileExists(dstPath), "Destination should exist")
	assert.Equal(t, srcContent, th.ReadFile(dstPath), "Content should match")
}

// Test append mode without destination size check
func TestMoveFile_AppendModeNoSizeCheck(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	srcContent := "appended"
	dstContent := "original"
	srcPath := th.CreateTestFile("source.txt", srcContent)
	dstPath := th.CreateTestFile("destination.txt", dstContent)

	// Use -1 to skip size check
	err := MoveFile(srcPath, dstPath, CopyModeAppend, "", -1)
	assert.NoError(t, err, "Move should succeed without size check")

	expectedContent := dstContent + srcContent
	assert.Equal(t, expectedContent, th.ReadFile(dstPath), "Content should be appended")
	assert.False(t, th.FileExists(srcPath), "Source should be deleted")
}

// Test large file move (basic performance test)
func TestMoveFile_LargeFile(t *testing.T) {
	th := NewFileMoveTestHelper(t)
	defer th.Cleanup()

	// Create a larger file (100KB)
	largeContent := make([]byte, 100*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	srcPath := th.CreateTestFile("large_source.txt", string(largeContent))
	dstPath := th.GetTempPath("large_destination.txt")

	expectedMD5 := th.GetFileMD5(srcPath)

	err := MoveFile(srcPath, dstPath, CopyModeOverwrite, expectedMD5, -1)
	assert.NoError(t, err, "Large file move should succeed")

	// Verify content integrity
	dstContent := th.ReadFile(dstPath)
	assert.Equal(t, len(largeContent), len(dstContent), "File size should match")
	assert.Equal(t, string(largeContent), dstContent, "Content should match exactly")
	assert.False(t, th.FileExists(srcPath), "Source should be deleted")
}
