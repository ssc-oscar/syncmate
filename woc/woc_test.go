package woc

import (
	"encoding/json"
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
