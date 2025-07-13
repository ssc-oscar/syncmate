package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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

	configs, err := LoadConfigs(configFile)
	if err != nil {
		t.Fatalf("LoadConfigs() failed: %v", err)
	}

	expectedCount := 3
	if len(configs) != expectedCount {
		t.Errorf("LoadConfigs() loaded %d configs, want %d", len(configs), expectedCount)
	}

	// 验证特定配置
	if config, ok := configs["test1.txt"]; ok {
		if config.SourcePath != "source1.txt" {
			t.Errorf("Config test1.txt source_path = %q, want %q", config.SourcePath, "source1.txt")
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

			_, err = LoadConfigs(configFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestIntegration_MountCGOFunction 测试 MountCGO 函数的集成测试
func TestIntegration_MountCGOFunction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := setupTestDir(t)

	// 创建源文件
	sourceFile1 := filepath.Join(tmpDir, "source1.txt")
	sourceFile2 := filepath.Join(tmpDir, "source2.txt")
	content1 := "This is the content of source file 1"
	content2 := "Source file 2 has different content that is longer"

	createTestFile(t, sourceFile1, content1)
	createTestFile(t, sourceFile2, content2)

	// 创建配置文件
	configFile := filepath.Join(tmpDir, "mount_config.jsonl")
	configContent := fmt.Sprintf(`{"virtual_path": "virtual1.txt", "source_path": "%s", "offset": 0, "size": 0, "read_only": false}
{"virtual_path": "virtual2.txt", "source_path": "%s", "offset": 8, "size": 20, "read_only": true}
# Comment: This is a test configuration
{"virtual_path": "virtual3.txt", "source_path": "%s", "offset": 0, "size": 15, "read_only": false}`,
		sourceFile1, sourceFile2, sourceFile1)

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// 创建挂载点
	mountpoint := filepath.Join(tmpDir, "mountpoint")
	err = os.Mkdir(mountpoint, 0755)
	if err != nil {
		t.Fatalf("Failed to create mountpoint: %v", err)
	}

	// 测试配置加载 - 这是 cmd 包的核心功能
	configs, err := LoadConfigs(configFile)
	if err != nil {
		t.Fatalf("LoadConfigs() failed: %v", err)
	}

	if len(configs) != 3 {
		t.Errorf("Expected 3 configs, got %d", len(configs))
	}

	// 验证配置内容
	validateConfig := func(virtualPath string, expectedSource string, expectedOffset int64, expectedSize int64) {
		if config := configs[virtualPath]; config == nil {
			t.Errorf("%s config not found", virtualPath)
		} else {
			if config.SourcePath != expectedSource {
				t.Errorf("%s source = %s, want %s", virtualPath, config.SourcePath, expectedSource)
			}
			if config.Offset != expectedOffset {
				t.Errorf("%s offset = %d, want %d", virtualPath, config.Offset, expectedOffset)
			}
			if config.Size != expectedSize {
				t.Errorf("%s size = %d, want %d", virtualPath, config.Size, expectedSize)
			}
		}
	}

	validateConfig("virtual1.txt", sourceFile1, 0, 0)
	validateConfig("virtual2.txt", sourceFile2, 8, 20)
	validateConfig("virtual3.txt", sourceFile1, 0, 15)

	t.Log("Mount CGO function config validation completed successfully")
}

// TestIntegration_MountCommandValidation 测试挂载命令的参数验证
func TestIntegration_MountCommandValidation(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name       string
		configFile string
		setupFunc  func() error
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid config",
			configFile: filepath.Join(tmpDir, "valid.jsonl"),
			setupFunc: func() error {
				sourceFile := filepath.Join(tmpDir, "source.txt")
				createTestFile(t, sourceFile, "test content")
				configContent := fmt.Sprintf(`{"virtual_path": "test.txt", "source_path": "%s", "offset": 0, "size": 0, "read_only": false}`, sourceFile)
				return os.WriteFile(filepath.Join(tmpDir, "valid.jsonl"), []byte(configContent), 0644)
			},
			wantErr: false,
		},
		{
			name:       "non-existent config file",
			configFile: filepath.Join(tmpDir, "nonexistent.jsonl"),
			setupFunc:  func() error { return nil },
			wantErr:    true,
			errMsg:     "failed to open config file",
		},
		{
			name:       "invalid json config",
			configFile: filepath.Join(tmpDir, "invalid.jsonl"),
			setupFunc: func() error {
				invalidContent := `{"virtual_path": "test.txt", invalid json}`
				return os.WriteFile(filepath.Join(tmpDir, "invalid.jsonl"), []byte(invalidContent), 0644)
			},
			wantErr: true,
			errMsg:  "failed to parse line",
		},
		{
			name:       "source file not found",
			configFile: filepath.Join(tmpDir, "missing_source.jsonl"),
			setupFunc: func() error {
				configContent := `{"virtual_path": "test.txt", "source_path": "/nonexistent/file.txt", "offset": 0, "size": 0, "read_only": false}`
				return os.WriteFile(filepath.Join(tmpDir, "missing_source.jsonl"), []byte(configContent), 0644)
			},
			wantErr: true,
			errMsg:  "invalid config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置测试数据
			err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			// 测试配置加载
			_, err = LoadConfigs(tt.configFile)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadConfigs() expected error but got none")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("LoadConfigs() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("LoadConfigs() unexpected error = %v", err)
				}
			}
		})
	}
}

// containsString 检查字符串是否包含子字符串
func containsString(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestIntegration_MountCGOWithInvalidConfig 测试 MountCGO 函数处理无效配置的情况
func TestIntegration_MountCGOWithInvalidConfig(t *testing.T) {
	tmpDir := setupTestDir(t)

	// 创建无效的配置文件
	configFile := filepath.Join(tmpDir, "invalid_config.jsonl")
	configContent := `{"virtual_path": "test.txt", "source_path": "/nonexistent/file.txt", "offset": 0, "size": 0, "read_only": false}`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	mountpoint := filepath.Join(tmpDir, "mountpoint")
	err = os.Mkdir(mountpoint, 0755)
	if err != nil {
		t.Fatalf("Failed to create mountpoint: %v", err)
	}

	// 测试 MountCGO 函数应该失败，因为源文件不存在
	// 注意：由于 MountCGO 中使用了 log.Fatalf，我们不能直接测试它
	// 相反，我们测试 LoadConfigs 函数，这是 MountCGO 的关键部分
	_, err = LoadConfigs(configFile)
	if err == nil {
		t.Error("Expected LoadConfigs to fail with invalid source file, but it succeeded")
	}

	// 验证错误消息
	if !containsString(err.Error(), "invalid config") {
		t.Errorf("Expected error to contain 'invalid config', got: %v", err)
	}

	t.Log("Mount CGO invalid config test completed successfully")
}

// TestMountCGO_Parameters 测试 MountCGO 函数的参数处理
func TestMountCGO_Parameters(t *testing.T) {
	tmpDir := setupTestDir(t)

	// 创建有效的源文件和配置
	sourceFile := filepath.Join(tmpDir, "source.txt")
	createTestFile(t, sourceFile, "test content for mount cgo")

	configFile := filepath.Join(tmpDir, "mount_params.jsonl")
	configContent := fmt.Sprintf(`{"virtual_path": "test.txt", "source_path": "%s", "offset": 0, "size": 0, "read_only": false}`, sourceFile)

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	mountpoint := filepath.Join(tmpDir, "mountpoint")
	err = os.Mkdir(mountpoint, 0755)
	if err != nil {
		t.Fatalf("Failed to create mountpoint: %v", err)
	}

	// 测试不同的参数组合
	testCases := []struct {
		name        string
		configFile  string
		debug       bool
		allowOther  bool
		expectError bool
	}{
		{
			name:        "valid_config_no_debug",
			configFile:  configFile,
			debug:       false,
			allowOther:  false,
			expectError: false,
		},
		{
			name:        "valid_config_with_debug",
			configFile:  configFile,
			debug:       true,
			allowOther:  false,
			expectError: false,
		},
		{
			name:        "valid_config_allow_other",
			configFile:  configFile,
			debug:       false,
			allowOther:  true,
			expectError: false,
		},
		{
			name:        "invalid_config_file",
			configFile:  "/nonexistent/config.jsonl",
			debug:       false,
			allowOther:  false,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 由于 MountCGO 实际上会尝试挂载文件系统并且会阻塞，
			// 我们只测试到配置加载这一步
			if tc.expectError {
				_, err := LoadConfigs(tc.configFile)
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				configs, err := LoadConfigs(tc.configFile)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(configs) == 0 {
					t.Error("Expected at least one config")
				}

				// 验证配置被正确加载
				if config, exists := configs["test.txt"]; !exists {
					t.Error("Expected test.txt config not found")
				} else {
					if config.SourcePath != sourceFile {
						t.Errorf("Expected source path %s, got %s", sourceFile, config.SourcePath)
					}
				}
			}
		})
	}

	t.Log("Mount CGO parameters test completed successfully")
}

// TestLoadConfigs_CommentHandling 测试配置文件中注释的处理
func TestLoadConfigs_CommentHandling(t *testing.T) {
	tmpDir := setupTestDir(t)

	// 创建源文件
	sourceFile := filepath.Join(tmpDir, "source.txt")
	createTestFile(t, sourceFile, "test content")

	// 创建包含各种注释格式的配置文件
	configFile := filepath.Join(tmpDir, "comments_config.jsonl")
	configContent := fmt.Sprintf(`# This is a comment at the beginning
{"virtual_path": "file1.txt", "source_path": "%s", "offset": 0, "size": 0, "read_only": false}
// This is another comment style
{"virtual_path": "file2.txt", "source_path": "%s", "offset": 5, "size": 10, "read_only": true}

# Empty lines and comments should be ignored

{"virtual_path": "file3.txt", "source_path": "%s", "offset": 0, "size": 0, "read_only": false}
// Final comment`, sourceFile, sourceFile, sourceFile)

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	configs, err := LoadConfigs(configFile)
	if err != nil {
		t.Fatalf("LoadConfigs() failed: %v", err)
	}

	// 应该正确加载3个配置，忽略所有注释
	expectedCount := 3
	if len(configs) != expectedCount {
		t.Errorf("Expected %d configs, got %d", expectedCount, len(configs))
	}

	// 验证每个配置都被正确加载
	expectedConfigs := []struct {
		virtualPath string
		offset      int64
		size        int64
	}{
		{"file1.txt", 0, 0},
		{"file2.txt", 5, 10},
		{"file3.txt", 0, 0},
	}

	for _, expected := range expectedConfigs {
		if config, exists := configs[expected.virtualPath]; !exists {
			t.Errorf("Config %s not found", expected.virtualPath)
		} else {
			if config.Offset != expected.offset {
				t.Errorf("%s offset = %d, want %d", expected.virtualPath, config.Offset, expected.offset)
			}
			if config.Size != expected.size {
				t.Errorf("%s size = %d, want %d", expected.virtualPath, config.Size, expected.size)
			}
		}
	}

	t.Log("Comment handling test completed successfully")
}
