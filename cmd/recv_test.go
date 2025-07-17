package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hrz6976/syncmate/offsetfs"
	"github.com/hrz6976/syncmate/woc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanDownloadedFiles(t *testing.T) {
	// 创建临时目录作为缓存目录
	tmpDir, err := os.MkdirTemp("", "syncmate_test_cache_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建一些测试文件
	testFiles := map[string]string{
		"file1.txt":         "content of file 1",
		"subdir/file2.txt":  "content of file 2",
		"file3.offset.1024": "partial content",
	}

	for relPath, content := range testFiles {
		fullPath := filepath.Join(tmpDir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	// 创建任务映射
	tasksMap := map[string]*woc.WocSyncTask{
		"file1.txt": {
			FileConfig: offsetfs.FileConfig{
				VirtualPath: "file1.txt",
				SourcePath:  "/source/file1.txt",
				Size:        17,
			},
		},
		"subdir/file2.txt": {
			FileConfig: offsetfs.FileConfig{
				VirtualPath: "subdir/file2.txt",
				SourcePath:  "/source/subdir/file2.txt",
				Size:        17,
			},
		},
		"file3.offset.1024": {
			FileConfig: offsetfs.FileConfig{
				VirtualPath: "file3.offset.1024",
				SourcePath:  "/source/file3.txt",
				Size:        15,
			},
		},
	}

	// 测试扫描功能
	cacheDir = tmpDir // 使用临时目录作为缓存目录
	downloadedFiles, err := scanDownloadedFiles(tasksMap)
	require.NoError(t, err)

	// 验证结果
	assert.Len(t, downloadedFiles, 3)

	// 验证每个文件都被正确识别
	found := make(map[string]bool)
	for _, fileInfo := range downloadedFiles {
		found[fileInfo.task.VirtualPath] = true
		assert.NotNil(t, fileInfo.task)
		assert.NotEmpty(t, fileInfo.filePath)
		assert.NotEmpty(t, fileInfo.destPath)
	}

	assert.True(t, found["file1.txt"])
	assert.True(t, found["subdir/file2.txt"])
	assert.True(t, found["file3.offset.1024"])
}

func TestScanDownloadedFiles_EmptyCache(t *testing.T) {
	// 测试空缓存目录
	cacheDir = ""
	downloadedFiles, err := scanDownloadedFiles(map[string]*woc.WocSyncTask{})
	require.NoError(t, err)
	assert.Empty(t, downloadedFiles)
}

func TestScanDownloadedFiles_NonExistentCache(t *testing.T) {
	// 测试不存在的缓存目录
	cacheDir = "/non/existent/path"
	downloadedFiles, err := scanDownloadedFiles(map[string]*woc.WocSyncTask{})
	require.NoError(t, err)
	assert.Empty(t, downloadedFiles)
}
