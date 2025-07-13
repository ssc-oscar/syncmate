package offsetfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/winfsp/cgofuse/fuse"
)

// 测试用的临时目录
func setupTestDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "cgofs_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})
	return tmpDir
}

// 创建测试文件
func createTestFile(t *testing.T, path string, content string) {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	err = os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
}

func TestFileConfig_Validation(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	createTestFile(t, testFile, "test content")

	tests := []struct {
		name      string
		config    FileConfig
		wantError bool
	}{
		{
			name: "valid config",
			config: FileConfig{
				VirtualPath: "valid.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        0,
				ReadOnly:    false,
			},
			wantError: false,
		},
		{
			name: "empty virtual path",
			config: FileConfig{
				VirtualPath: "",
				SourcePath:  testFile,
				Offset:      0,
				Size:        0,
				ReadOnly:    false,
			},
			wantError: true,
		},
		{
			name: "empty source path",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  "",
				Offset:      0,
				Size:        0,
				ReadOnly:    false,
			},
			wantError: true,
		},
		{
			name: "negative offset",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  testFile,
				Offset:      -1,
				Size:        0,
				ReadOnly:    false,
			},
			wantError: true,
		},
		{
			name: "negative size",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        -1,
				ReadOnly:    false,
			},
			wantError: true,
		},
		{
			name: "virtual path with slash",
			config: FileConfig{
				VirtualPath: "path/test.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        0,
				ReadOnly:    false,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(&tt.config)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateConfig() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestOffsetFS_Getattr(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World! This is a test file with some content."
	createTestFile(t, testFile, testContent)

	configs := map[string]*FileConfig{
		"direct.txt": {
			VirtualPath: "direct.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    true,
		},
		"offset.txt": {
			VirtualPath: "offset.txt",
			SourcePath:  testFile,
			Offset:      7,
			Size:        0,
			ReadOnly:    true,
		},
		"sized.txt": {
			VirtualPath: "sized.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        10,
			ReadOnly:    true,
		},
		"offset_sized.txt": {
			VirtualPath: "offset_sized.txt",
			SourcePath:  testFile,
			Offset:      7,
			Size:        5,
			ReadOnly:    true,
		},
	}

	tests := []struct {
		name       string
		path       string
		expectSize int64
	}{
		{
			name:       "direct access",
			path:       "/direct.txt",
			expectSize: int64(len(testContent)),
		},
		{
			name:       "offset access",
			path:       "/offset.txt",
			expectSize: int64(len(testContent) - 7),
		},
		{
			name:       "sized access",
			path:       "/sized.txt",
			expectSize: 10,
		},
		{
			name:       "offset and sized",
			path:       "/offset_sized.txt",
			expectSize: 5,
		},
	}

	fs := NewOffsetFS(configs)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stat fuse.Stat_t
			result := fs.Getattr(tt.path, &stat, 0)

			if result != 0 {
				t.Errorf("Getattr() result = %v, want 0", result)
				return
			}

			if stat.Size != tt.expectSize {
				t.Errorf("Getattr() size = %v, want %v", stat.Size, tt.expectSize)
			}
		})
	}
}

func TestOffsetFS_Read(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "0123456789abcdefghijklmnopqrstuvwxyz"
	createTestFile(t, testFile, testContent)

	configs := map[string]*FileConfig{
		"direct.txt": {
			VirtualPath: "direct.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    true,
		},
		"offset.txt": {
			VirtualPath: "offset.txt",
			SourcePath:  testFile,
			Offset:      5,
			Size:        0,
			ReadOnly:    true,
		},
		"sized.txt": {
			VirtualPath: "sized.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        15,
			ReadOnly:    true,
		},
		"limited.txt": {
			VirtualPath: "limited.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        10,
			ReadOnly:    true,
		},
	}

	tests := []struct {
		name         string
		path         string
		readOffset   int64
		readSize     int
		expectedData string
		expectedLen  int
	}{
		{
			name:         "direct read from start",
			path:         "/direct.txt",
			readOffset:   0,
			readSize:     10,
			expectedData: "0123456789",
			expectedLen:  10,
		},
		{
			name:         "offset read",
			path:         "/offset.txt",
			readOffset:   0,
			readSize:     10,
			expectedData: "56789abcde",
			expectedLen:  10,
		},
		{
			name:         "sized read",
			path:         "/sized.txt",
			readOffset:   5,
			readSize:     20,
			expectedData: "56789abcde",
			expectedLen:  10,
		},
		{
			name:         "read beyond virtual file",
			path:         "/limited.txt",
			readOffset:   15,
			readSize:     5,
			expectedData: "",
			expectedLen:  0,
		},
	}

	fs := NewOffsetFS(configs)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 首先打开文件
			retcode, _ := fs.Open(tt.path, fuse.O_RDONLY)
			if retcode != 0 {
				t.Errorf("Open() failed with code %v", retcode)
				return
			}

			buff := make([]byte, tt.readSize)
			bytesRead := fs.Read(tt.path, buff, tt.readOffset, 0)

			if bytesRead < 0 {
				t.Errorf("Read() failed with code %v", bytesRead)
				return
			}

			if bytesRead != tt.expectedLen {
				t.Errorf("Read() bytesRead = %v, want %v", bytesRead, tt.expectedLen)
			}

			if bytesRead > 0 {
				data := string(buff[:bytesRead])
				if data != tt.expectedData {
					t.Errorf("Read() data = %q, want %q", data, tt.expectedData)
				}
			}
		})
	}
}

func TestOffsetFS_Write(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "0123456789abcdefghijklmnopqrstuvwxyz"
	createTestFile(t, testFile, testContent)

	configs := map[string]*FileConfig{
		"readonly.txt": {
			VirtualPath: "readonly.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    true,
		},
		"direct.txt": {
			VirtualPath: "direct.txt",
			SourcePath:  filepath.Join(tmpDir, "write_test1.txt"),
			Offset:      0,
			Size:        0,
			ReadOnly:    false,
		},
		"offset.txt": {
			VirtualPath: "offset.txt",
			SourcePath:  filepath.Join(tmpDir, "write_test2.txt"),
			Offset:      5,
			Size:        0,
			ReadOnly:    false,
		},
		"sized.txt": {
			VirtualPath: "sized.txt",
			SourcePath:  filepath.Join(tmpDir, "write_test3.txt"),
			Offset:      0,
			Size:        10,
			ReadOnly:    false,
		},
		"sized_exceed.txt": {
			VirtualPath: "sized_exceed.txt",
			SourcePath:  filepath.Join(tmpDir, "write_test4.txt"),
			Offset:      0,
			Size:        3,
			ReadOnly:    false,
		},
	}

	tests := []struct {
		name          string
		path          string
		writeOffset   int64
		writeData     string
		expectedCode  int
		checkContent  bool
		expectedAfter string
	}{
		{
			name:         "write to readonly file",
			path:         "/readonly.txt",
			writeOffset:  0,
			writeData:    "HELLO",
			expectedCode: -fuse.EACCES,
			checkContent: false,
		},
		{
			name:          "direct write",
			path:          "/direct.txt",
			writeOffset:   0,
			writeData:     "HELLO",
			expectedCode:  5,
			checkContent:  true,
			expectedAfter: "HELLO",
		},
		{
			name:         "offset write",
			path:         "/offset.txt",
			writeOffset:  0,
			writeData:    "HELLO",
			expectedCode: 5,
			checkContent: false,
		},
		{
			name:          "sized write within limit",
			path:          "/sized.txt",
			writeOffset:   0,
			writeData:     "HELLO",
			expectedCode:  5,
			checkContent:  true,
			expectedAfter: "HELLO",
		},
		{
			name:          "sized write exceeding limit",
			path:          "/sized_exceed.txt",
			writeOffset:   0,
			writeData:     "HELLO",
			expectedCode:  3,
			checkContent:  true,
			expectedAfter: "HEL",
		},
	}

	fs := NewOffsetFS(configs)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 首先打开文件
			retcode, _ := fs.Open(tt.path, fuse.O_RDWR)
			if retcode != 0 && tt.expectedCode >= 0 {
				// 对于预期成功的操作，打开失败是错误
				t.Errorf("Open() failed with code %v", retcode)
				return
			}

			bytesWritten := fs.Write(tt.path, []byte(tt.writeData), tt.writeOffset, 0)

			if bytesWritten != tt.expectedCode {
				t.Errorf("Write() result = %v, want %v", bytesWritten, tt.expectedCode)
				return
			}

			if tt.expectedCode > 0 && tt.checkContent {
				// 验证写入的内容
				config := configs[tt.path[1:]] // 移除前导斜杠
				content, err := os.ReadFile(config.SourcePath)
				if err != nil {
					t.Errorf("Failed to read written file: %v", err)
					return
				}

				if string(content) != tt.expectedAfter {
					t.Errorf("Write() result content = %q, want %q", string(content), tt.expectedAfter)
				}
			}
		})
	}
}

func TestOffsetFS_FileCreation(t *testing.T) {
	tmpDir := setupTestDir(t)
	newFile := filepath.Join(tmpDir, "new_file.txt")

	// 确保文件不存在
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatalf("Test file should not exist initially")
	}

	configs := map[string]*FileConfig{
		"new.txt": {
			VirtualPath: "new.txt",
			SourcePath:  newFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    false,
		},
	}

	fs := NewOffsetFS(configs)

	// 打开文件
	retcode, _ := fs.Open("/new.txt", fuse.O_RDWR)
	if retcode != 0 {
		t.Errorf("Open() failed with code %v", retcode)
		return
	}

	// 写入数据应该创建文件
	writeData := "Hello, new file!"
	bytesWritten := fs.Write("/new.txt", []byte(writeData), 0, 0)

	if bytesWritten != len(writeData) {
		t.Errorf("Write() bytesWritten = %v, want %v", bytesWritten, len(writeData))
		return
	}

	// 验证文件确实被创建
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("File was not created: %v", err)
		return
	}

	// 验证内容
	content, err := os.ReadFile(newFile)
	if err != nil {
		t.Errorf("Failed to read created file: %v", err)
		return
	}

	if string(content) != writeData {
		t.Errorf("Created file content = %q, want %q", string(content), writeData)
	}
}

func TestOffsetFS_Readdir(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile1 := filepath.Join(tmpDir, "source1.txt")
	testFile2 := filepath.Join(tmpDir, "source2.txt")
	createTestFile(t, testFile1, "content1")
	createTestFile(t, testFile2, "content2")

	configs := map[string]*FileConfig{
		"file1.txt": {
			VirtualPath: "file1.txt",
			SourcePath:  testFile1,
			Offset:      0,
			Size:        0,
			ReadOnly:    false,
		},
		"file2.txt": {
			VirtualPath: "file2.txt",
			SourcePath:  testFile2,
			Offset:      0,
			Size:        0,
			ReadOnly:    true,
		},
	}

	fs := NewOffsetFS(configs)

	// 测试根目录列举
	var entries []string
	fillFunc := func(name string, stat *fuse.Stat_t, ofst int64) bool {
		entries = append(entries, name)
		return true
	}

	result := fs.Readdir("/", fillFunc, 0, 0)
	if result != 0 {
		t.Errorf("Readdir() failed with code %v", result)
		return
	}

	// 检查是否包含预期的文件
	expectedFiles := map[string]bool{
		".":         true,
		"..":        true,
		"file1.txt": true,
		"file2.txt": true,
	}

	if len(entries) != len(expectedFiles) {
		t.Errorf("Readdir() returned %d entries, want %d", len(entries), len(expectedFiles))
	}

	for _, entry := range entries {
		if !expectedFiles[entry] {
			t.Errorf("Unexpected entry in readdir: %s", entry)
		}
	}

	// 测试非根目录
	result = fs.Readdir("/nonexistent", fillFunc, 0, 0)
	if result != -fuse.ENOENT {
		t.Errorf("Readdir() on non-existent directory should return ENOENT, got %v", result)
	}
}

// 基准测试
func BenchmarkOffsetFS_Read(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "cgofs_bench_*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一个较大的测试文件
	testFile := filepath.Join(tmpDir, "large_test.txt")
	content := make([]byte, 1024*1024) // 1MB
	for i := range content {
		content[i] = byte(i % 256)
	}
	err = os.WriteFile(testFile, content, 0644)
	if err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	configs := map[string]*FileConfig{
		"large.txt": {
			VirtualPath: "large.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    true,
		},
	}

	fs := NewOffsetFS(configs)
	buff := make([]byte, 4096) // 4KB buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		offset := int64(i % (len(content) - len(buff)))
		fs.Read("/large.txt", buff, offset, 0)
	}
}

func BenchmarkOffsetFS_Write(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "cgofs_bench_*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "write_test.txt")

	configs := map[string]*FileConfig{
		"writable.txt": {
			VirtualPath: "writable.txt",
			SourcePath:  testFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    false,
		},
	}

	fs := NewOffsetFS(configs)
	data := []byte("benchmark data for writing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		offset := int64(i * len(data))
		fs.Write("/writable.txt", data, offset, 0)
	}
}
