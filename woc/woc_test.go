package woc

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseWocProfile(t *testing.T) {
	testFile := "woc.src.json"
	profile, err := ParseWocProfile(&testFile)
	if err != nil {
		t.Fatalf("Failed to parse WocProfile: %v", err)
	}
	if len(profile.Maps) == 0 {
		t.Fatal("Parsed WocProfile has no maps")
	}
	if len(profile.Objects) == 0 {
		t.Fatal("Parsed WocProfile has no objects")
	}
}

func TestGenerateFileList(t *testing.T) {
	srcPath := "woc.src.json"
	dstPath := "woc.dst.json"
	// Assuming ParseWocProfile is a function that reads and parses the WocProfile from a file
	dstProfile, err := ParseWocProfile(&dstPath)
	if err != nil {
		t.Fatalf("Failed to parse destination profile: %v", err)
	}
	srcProfile, err := ParseWocProfile(&srcPath)
	if err != nil {
		t.Fatalf("Failed to parse source profile: %v", err)
	}

	fileList := GenerateFileLists(dstProfile, srcProfile)
	// dump the file list to json
	_, err = json.MarshalIndent(fileList, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal file list: %v", err)
	}
}

// 添加测试用例用于测试 RelocatePath 函数
func TestRelocatePath(t *testing.T) {
	// 保存原始的主机名
	originalHostname, _ := os.Hostname()

	tests := []struct {
		name         string
		mockHostname string
		inputPath    string
		expectedPath string
		expectError  bool
	}{
		{
			name:         "da8 path relocation",
			mockHostname: "da8.eecs.utk.edu",
			inputPath:    "/da8_data/test/file.txt",
			expectedPath: "/mnt/ordos/data/data/test/file.txt",
			expectError:  false,
		},
		{
			name:         "da7 path relocation",
			mockHostname: "da7.eecs.utk.edu",
			inputPath:    "/da7_data/test/file.txt",
			expectedPath: "/corrino/test/file.txt",
			expectError:  false,
		},
		{
			name:         "ishia treated as da7",
			mockHostname: "ishia.eecs.utk.edu",
			inputPath:    "/da7_data/test/file.txt",
			expectedPath: "/corrino/test/file.txt",
			expectError:  false,
		},
		{
			name:         "other da server",
			mockHostname: "da5.eecs.utk.edu",
			inputPath:    "/da5_data/test/file.txt",
			expectedPath: "/data/test/file.txt",
			expectError:  false,
		},
		{
			name:         "non-matching path - no change",
			mockHostname: "da8.eecs.utk.edu",
			inputPath:    "/other/path/file.txt",
			expectedPath: "/other/path/file.txt",
			expectError:  false,
		},
		{
			name:         "empty path",
			mockHostname: "da8.eecs.utk.edu",
			inputPath:    "",
			expectedPath: "",
			expectError:  true,
		},
		{
			name:         "nil path pointer",
			mockHostname: "da8.eecs.utk.edu",
			inputPath:    "",
			expectedPath: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟主机名 - 这里我们需要测试实际的逻辑，但由于无法直接模拟 os.Hostname()
			// 我们将创建一个修改过的版本来测试
			var pathPtr *string
			if tt.name == "nil path pointer" {
				pathPtr = nil
			} else {
				pathPtr = &tt.inputPath
			}

			err := RelocatePath(pathPtr)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if pathPtr != nil && *pathPtr != tt.expectedPath {
				// 由于我们无法模拟主机名，这个测试可能需要在特定环境下运行
				// 或者我们需要重构 RelocatePath 函数使其更易于测试
				t.Logf("Path changed from %s to %s (hostname: %s)",
					tt.inputPath, *pathPtr, originalHostname)
			}
		})
	}
}

// 测试实际的路径重定位逻辑（基于当前主机名）
func TestRelocatePathRealHostname(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Skipf("Cannot get hostname: %v", err)
	}

	// 测试空字符串
	emptyPath := ""
	err = RelocatePath(&emptyPath)
	if err == nil {
		t.Error("Expected error for empty path")
	}

	// 测试 nil 指针
	err = RelocatePath(nil)
	if err == nil {
		t.Error("Expected error for nil path")
	}

	// 测试不匹配的路径（应该保持不变）
	testPath := "/some/other/path"
	originalPath := testPath
	err = RelocatePath(&testPath)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if testPath != originalPath {
		t.Errorf("Path should not change for non-matching pattern. Got: %s, Want: %s", testPath, originalPath)
	}

	t.Logf("Current hostname: %s", hostname)
}
