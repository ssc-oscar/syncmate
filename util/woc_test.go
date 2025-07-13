package util

import (
	"encoding/json"
	"fmt"
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
	// print the parsed profile for debugging
	for name, obj := range profile.Objects {
		fmt.Printf("Object: %s, Name: %s", name, obj.Shards[0].Path)
	}
	for name, m := range profile.Maps {
		fmt.Printf("Map: %s, Version: %s, Shards: %d\n", name, m.Version, len(m.Shards))
		for _, shard := range m.Shards {
			fmt.Printf("  Shard: Path: %s, Size: %d, Digest: %s\n", shard.Path, shard.Size, *shard.Digest)
		}
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
	jsonData, err := json.MarshalIndent(fileList, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal file list: %v", err)
	}
	fmt.Printf("Generated File List:\n%s\n", jsonData)
}
