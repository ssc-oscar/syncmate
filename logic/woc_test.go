package logic

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestParseWocProfile(t *testing.T) {
	testFile := "/Users/hrz/Documents/GitHub/offsetfs/woc_local.json"
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer file.Close()

	var profile WocProfile
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&profile)
	if err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if len(profile.Maps) == 0 {
		t.Error("Expected non-empty Maps in WocProfile")
	}

	if len(profile.Objects) == 0 {
		t.Error("Expected non-empty Objects in WocProfile")
	}

	fmt.Printf("Parsed WocProfile: %+v\n", profile)
}

func TestGenerateFileList(t *testing.T) {
	srcPath := "/Users/hrz/Documents/GitHub/offsetfs/woc_local.json"
	dstPath := "/Users/hrz/Documents/GitHub/offsetfs/woc_remote.json"
	// Assuming ParseWocProfile is a function that reads and parses the WocProfile from a file
	dstProfile, err := ParseWocProfile(&srcPath)
	if err != nil {
		t.Fatalf("Failed to parse destination profile: %v", err)
	}
	srcProfile, err := ParseWocProfile(&dstPath)
	if err != nil {
		t.Fatalf("Failed to parse source profile: %v", err)
	}

	fileList := GenerateFileLists(dstProfile, srcProfile)
	// print the file list for debugging
	for path, task := range fileList {
		fmt.Printf("File: %s, Source Digest: %s, Target Digest: %s\n", path, *task.SourceDigest, *task.TargetDigest)
	}
}
