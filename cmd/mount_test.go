package cmd

import (
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

			_, err = LoadConfigs(configFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
