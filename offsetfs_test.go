package main

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// 测试用的临时目录
func setupTestDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "offsetfs_test_*")
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
			err := validateConfig(&tt.config)
			if (err != nil) != tt.wantError {
				t.Errorf("validateConfig() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestOffsetFileNode_Getattr(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World! This is a test file with some content."
	createTestFile(t, testFile, testContent)

	tests := []struct {
		name       string
		config     FileConfig
		expectSize uint64
	}{
		{
			name: "direct access",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        0,
				ReadOnly:    true,
			},
			expectSize: uint64(len(testContent)),
		},
		{
			name: "offset access",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  testFile,
				Offset:      7,
				Size:        0,
				ReadOnly:    true,
			},
			expectSize: uint64(len(testContent) - 7),
		},
		{
			name: "sized access",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        10,
				ReadOnly:    true,
			},
			expectSize: 10,
		},
		{
			name: "offset and sized",
			config: FileConfig{
				VirtualPath: "test.txt",
				SourcePath:  testFile,
				Offset:      7,
				Size:        5,
				ReadOnly:    true,
			},
			expectSize: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &OffsetFileNode{config: &tt.config}
			var out fuse.AttrOut

			errno := node.Getattr(context.Background(), nil, &out)
			if errno != 0 {
				t.Errorf("Getattr() errno = %v, want 0", errno)
				return
			}

			if out.Size != tt.expectSize {
				t.Errorf("Getattr() size = %v, want %v", out.Size, tt.expectSize)
			}
		})
	}
}

func TestOffsetFileNode_Read(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "0123456789abcdefghijklmnopqrstuvwxyz"
	createTestFile(t, testFile, testContent)

	tests := []struct {
		name          string
		config        FileConfig
		readOffset    int64
		readSize      int
		expectedData  string
		expectedErrno int
	}{
		{
			name: "direct read from start",
			config: FileConfig{
				VirtualPath: "direct.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        0,
				ReadOnly:    true,
			},
			readOffset:    0,
			readSize:      10,
			expectedData:  "0123456789",
			expectedErrno: 0,
		},
		{
			name: "offset read",
			config: FileConfig{
				VirtualPath: "offset.txt",
				SourcePath:  testFile,
				Offset:      5,
				Size:        0,
				ReadOnly:    true,
			},
			readOffset:    0,
			readSize:      10,
			expectedData:  "56789abcde",
			expectedErrno: 0,
		},
		{
			name: "sized read",
			config: FileConfig{
				VirtualPath: "sized.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        15,
				ReadOnly:    true,
			},
			readOffset:    5,
			readSize:      20, // 超过可用大小
			expectedData:  "56789abcde",
			expectedErrno: 0,
		},
		{
			name: "read beyond virtual file",
			config: FileConfig{
				VirtualPath: "limited.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        10,
				ReadOnly:    true,
			},
			readOffset:    15, // 超过虚拟文件大小
			readSize:      5,
			expectedData:  "",
			expectedErrno: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &OffsetFileNode{config: &tt.config}
			dest := make([]byte, tt.readSize)

			result, errno := node.Read(context.Background(), nil, dest, tt.readOffset)
			if int(errno) != tt.expectedErrno {
				t.Errorf("Read() errno = %v, want %v", errno, tt.expectedErrno)
				return
			}

			if errno == 0 {
				data, _ := result.Bytes(dest)
				if string(data) != tt.expectedData {
					t.Errorf("Read() data = %q, want %q", string(data), tt.expectedData)
				}
			}
		})
	}
}

func TestOffsetFileNode_Write(t *testing.T) {
	tmpDir := setupTestDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "0123456789abcdefghijklmnopqrstuvwxyz"
	createTestFile(t, testFile, testContent)

	tests := []struct {
		name          string
		config        FileConfig
		writeOffset   int64
		writeData     string
		expectedErrno int
		checkContent  bool
		expectedAfter string
	}{
		{
			name: "write to readonly file",
			config: FileConfig{
				VirtualPath: "readonly.txt",
				SourcePath:  testFile,
				Offset:      0,
				Size:        0,
				ReadOnly:    true,
			},
			writeOffset:   0,
			writeData:     "HELLO",
			expectedErrno: int(fuse.EACCES),
			checkContent:  false,
		},
		{
			name: "direct write",
			config: FileConfig{
				VirtualPath: "direct.txt",
				SourcePath:  filepath.Join(tmpDir, "write_test1.txt"),
				Offset:      0,
				Size:        0,
				ReadOnly:    false,
			},
			writeOffset:   0,
			writeData:     "HELLO",
			expectedErrno: 0,
			checkContent:  true,
			expectedAfter: "HELLO",
		},
		{
			name: "offset write",
			config: FileConfig{
				VirtualPath: "offset.txt",
				SourcePath:  filepath.Join(tmpDir, "write_test2.txt"),
				Offset:      5,
				Size:        0,
				ReadOnly:    false,
			},
			writeOffset:   0,
			writeData:     "HELLO",
			expectedErrno: 0,
			checkContent:  false, // 复杂的验证在其他地方
		},
		{
			name: "sized write within limit",
			config: FileConfig{
				VirtualPath: "sized.txt",
				SourcePath:  filepath.Join(tmpDir, "write_test3.txt"),
				Offset:      0,
				Size:        10,
				ReadOnly:    false,
			},
			writeOffset:   0,
			writeData:     "HELLO",
			expectedErrno: 0,
			checkContent:  true,
			expectedAfter: "HELLO",
		},
		{
			name: "sized write exceeding limit",
			config: FileConfig{
				VirtualPath: "sized_exceed.txt",
				SourcePath:  filepath.Join(tmpDir, "write_test4.txt"),
				Offset:      0,
				Size:        3,
				ReadOnly:    false,
			},
			writeOffset:   0,
			writeData:     "HELLO",
			expectedErrno: 0,
			checkContent:  true,
			expectedAfter: "HEL", // 应该被截断
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &OffsetFileNode{config: &tt.config}

			bytesWritten, errno := node.Write(context.Background(), nil, []byte(tt.writeData), tt.writeOffset)
			if int(errno) != tt.expectedErrno {
				t.Errorf("Write() errno = %v, want %v", errno, tt.expectedErrno)
				return
			}

			if errno == 0 && tt.checkContent {
				// 验证写入的内容
				content, err := os.ReadFile(tt.config.SourcePath)
				if err != nil {
					t.Errorf("Failed to read written file: %v", err)
					return
				}

				if string(content) != tt.expectedAfter {
					t.Errorf("Write() result content = %q, want %q", string(content), tt.expectedAfter)
				}

				expectedBytes := len(tt.expectedAfter)
				if int(bytesWritten) != expectedBytes {
					t.Errorf("Write() bytesWritten = %v, want %v", bytesWritten, expectedBytes)
				}
			}
		})
	}
}

func TestOffsetFileNode_FileCreation(t *testing.T) {
	tmpDir := setupTestDir(t)
	newFile := filepath.Join(tmpDir, "new_file.txt")

	// 确保文件不存在
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatalf("Test file should not exist initially")
	}

	config := &FileConfig{
		VirtualPath: "new.txt",
		SourcePath:  newFile,
		Offset:      0,
		Size:        0,
		ReadOnly:    false,
	}

	node := &OffsetFileNode{config: config}

	// 写入数据应该创建文件
	writeData := "Hello, new file!"
	bytesWritten, errno := node.Write(context.Background(), nil, []byte(writeData), 0)

	if errno != 0 {
		t.Errorf("Write() to new file failed with errno = %v", errno)
		return
	}

	if int(bytesWritten) != len(writeData) {
		t.Errorf("Write() bytesWritten = %v, want %v", bytesWritten, len(writeData))
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

func TestLoadConfigs(t *testing.T) {
	tmpDir := setupTestDir(t)

	// 创建测试配置文件
	configFile := filepath.Join(tmpDir, "test_config.jsonl")
	configContent := `{"virtual_path": "test1.txt", "source_path": "source1.txt", "offset": 0, "size": 0, "read_only": false}
{"virtual_path": "test2.txt", "source_path": "source2.txt", "offset": 10, "size": 20, "read_only": true}
# This is a comment
{"virtual_path": "test3.txt", "source_path": "source3.txt", "offset": 5, "size": 0, "read_only": false}

// Another comment
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// 创建源文件以通过验证
	for i := 1; i <= 3; i++ {
		sourceFile := filepath.Join(tmpDir, "source"+string(rune(i+'0'))+".txt")
		createTestFile(t, sourceFile, "test content")
	}

	// 临时改变工作目录
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	configs, err := loadConfigs(configFile)
	if err != nil {
		t.Fatalf("loadConfigs() failed: %v", err)
	}

	expectedCount := 3
	if len(configs) != expectedCount {
		t.Errorf("loadConfigs() loaded %d configs, want %d", len(configs), expectedCount)
	}

	// 验证特定配置
	if config, ok := configs["test1.txt"]; ok {
		if config.SourcePath != "source1.txt" {
			t.Errorf("Config test1.txt source_path = %q, want %q", config.SourcePath, "source1.txt")
		}
		if config.ReadOnly != false {
			t.Errorf("Config test1.txt read_only = %v, want %v", config.ReadOnly, false)
		}
	} else {
		t.Error("Config test1.txt not found")
	}

	if config, ok := configs["test2.txt"]; ok {
		if config.Offset != 10 {
			t.Errorf("Config test2.txt offset = %d, want %d", config.Offset, 10)
		}
		if config.Size != 20 {
			t.Errorf("Config test2.txt size = %d, want %d", config.Size, 20)
		}
		if config.ReadOnly != true {
			t.Errorf("Config test2.txt read_only = %v, want %v", config.ReadOnly, true)
		}
	} else {
		t.Error("Config test2.txt not found")
	}
}

func TestLoadConfigs_Errors(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "invalid JSON",
			content: `{"virtual_path": "test.txt", invalid json}`,
			wantErr: true,
		},
		{
			name: "duplicate virtual path",
			content: `{"virtual_path": "test.txt", "source_path": "source1.txt", "offset": 0, "size": 0, "read_only": false}
{"virtual_path": "test.txt", "source_path": "source2.txt", "offset": 0, "size": 0, "read_only": false}`,
			wantErr: true,
		},
		{
			name:    "empty file",
			content: ``,
			wantErr: true,
		},
		{
			name: "only comments",
			content: `# Comment 1
// Comment 2`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := filepath.Join(tmpDir, "config_"+tt.name+".jsonl")
			err := os.WriteFile(configFile, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create config file: %v", err)
			}

			_, err = loadConfigs(configFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSimpleFileOperations 简化的文件操作测试
func TestSimpleFileOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "offsetfs_simple_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建源文件
	sourceFile := filepath.Join(tmpDir, "source.txt")
	originalContent := "Hello, OffsetFS!"
	err = os.WriteFile(sourceFile, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// 创建配置
	config := &FileConfig{
		VirtualPath: "test.txt",
		SourcePath:  sourceFile,
		Offset:      0,
		Size:        0,
		ReadOnly:    false,
	}

	// 直接测试 OffsetFileNode 而不挂载文件系统
	node := &OffsetFileNode{
		config: config,
	}

	ctx := context.Background()

	// 测试 Getattr
	var attrOut fuse.AttrOut
	errno := node.Getattr(ctx, nil, &attrOut)
	if errno != 0 {
		t.Fatalf("Getattr failed: %v", errno)
	}
	t.Logf("File size: %d, mode: %o", attrOut.Size, attrOut.Mode)

	// 测试 Open
	handle, flags, errno := node.Open(ctx, syscall.O_RDWR)
	if errno != 0 {
		t.Fatalf("Open failed: %v", errno)
	}
	t.Logf("Open successful, handle: %v, flags: %v", handle, flags)

	// 测试 Read
	readBuf := make([]byte, 100)
	result, errno := node.Read(ctx, handle, readBuf, 0)
	if errno != 0 {
		t.Fatalf("Read failed: %v", errno)
	}

	readData, status := result.Bytes(readBuf)
	if status != fuse.OK {
		t.Fatalf("Read result error: %v", status)
	}
	t.Logf("Read data: %s", string(readData))

	if string(readData) != originalContent {
		t.Errorf("Read content = %q, want %q", string(readData), originalContent)
	}

	// 测试 Write
	newContent := "Updated content!"
	written, errno := node.Write(ctx, handle, []byte(newContent), 0)
	if errno != 0 {
		t.Fatalf("Write failed: %v", errno)
	}
	t.Logf("Wrote %d bytes", written)

	// 验证写入结果
	sourceContent, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("Failed to read source file: %v", err)
	}

	if string(sourceContent) != newContent {
		t.Errorf("Source file content = %q, want %q", string(sourceContent), newContent)
	}

	t.Log("Simple file operations test passed!")
}

// TestMountWithMinimalOptions 使用最小选项的挂载测试
func TestMountWithMinimalOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mount test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "offsetfs_minimal_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建源文件
	sourceFile := filepath.Join(tmpDir, "source.txt")
	content := "Minimal test content"
	err = os.WriteFile(sourceFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// 配置
	configs := map[string]*FileConfig{
		"minimal.txt": {
			VirtualPath: "minimal.txt",
			SourcePath:  sourceFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    false,
		},
	}

	root := &OffsetFSRoot{
		configs: configs,
	}

	mountpoint := filepath.Join(tmpDir, "mount")
	err = os.Mkdir(mountpoint, 0755)
	if err != nil {
		t.Fatalf("Failed to create mountpoint: %v", err)
	}

	// 使用最小的挂载选项
	server, err := fs.Mount(mountpoint, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: false,
		},
	})
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	defer func() {
		err := server.Unmount()
		if err != nil {
			t.Logf("Unmount failed: %v", err)
		}
	}()

	// 给文件系统一些时间来初始化
	time.Sleep(100 * time.Millisecond)

	// 测试列出文件
	entries, err := os.ReadDir(mountpoint)
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "minimal.txt" {
		t.Fatalf("Expected one file 'minimal.txt', got: %v", entries)
	}

	// 测试读取
	virtualFile := filepath.Join(mountpoint, "minimal.txt")
	readContent, err := os.ReadFile(virtualFile)
	if err != nil {
		t.Fatalf("Failed to read virtual file: %v", err)
	}

	if string(readContent) != content {
		t.Errorf("Read content = %q, want %q", string(readContent), content)
	}

	t.Log("Minimal mount test passed!")
}
