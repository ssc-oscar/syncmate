package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hrz6976/syncmate/logger"
	of "github.com/hrz6976/syncmate/offsetfs" // Assuming offsetfs is the package where WocFile, WocObject, WocMap, and WocProfile are defined
)

// WocFile represents a file in the WoC database.
type WocFile struct {
	// Path to the file in the local filesystem.
	Path string `json:"path"`

	// Size of file in bytes.
	Size *int `json:"size,omitempty"`

	// 16-char digest calculated by woc.utils.fast_digest.
	Digest *string `json:"digest,omitempty"`
}

// WocObject represents a base object with sharding information.
type WocObject struct {
	// Number of bits used for sharding.
	ShardingBits int `json:"sharding_bits"`

	// List of shard files.
	Shards []WocFile `json:"shards"`
}

// WocMap represents a map object that extends WocObject.
type WocMap struct {
	// Version of the map, e.g. 'R', 'U'.
	Version string `json:"version"`

	// Number of bits used for sharding.
	ShardingBits int `json:"sharding_bits"`

	// List of shard files.
	Shards []WocFile `json:"shards"`

	// Large files associated with the map.
	Larges map[string]WocFile `json:"larges"`

	// Data types of the map, e.g. ["h", "cs"], ["h", "hhwww"].
	// Use a fixed-length array for correct JSON unmarshalling.
	DTypes []string `json:"dtypes"`
}

// WocProfile represents the main configuration structure for WoC.
type WocProfile struct {
	// Maps contains all the map objects indexed by name.
	Maps map[string][]WocMap `json:"maps"`

	// Objects contains all the object files indexed by name.
	Objects map[string]WocObject `json:"objects"`
}

type ParsedWocProfile struct {
	Maps    map[string]WocMap    `json:"maps"`
	Objects map[string]WocObject `json:"objects"`
}

func ParseWocProfile(profilePath *string) (*ParsedWocProfile, error) {
	// Read the JSON file
	data, err := os.ReadFile(*profilePath)
	if err != nil {
		return nil, err
	}

	// Parse the JSON into WocProfile structure
	var profile WocProfile
	err = json.Unmarshal(data, &profile)
	if err != nil {
		return nil, err
	}

	var parsedProfile ParsedWocProfile = ParsedWocProfile{
		Maps:    make(map[string]WocMap),
		Objects: make(map[string]WocObject),
	}
	// Set the Name field for each object based on the map key
	for name, obj := range profile.Objects {
		profile.Objects[name] = obj
	}

	// pick the map entry with the latest version
	for name, maps := range profile.Maps {
		latestMap := maps[0]
		for _, m := range maps {
			if m.Version > latestMap.Version {
				latestMap = m
			}
		}
		parsedProfile.Maps[name] = latestMap
	}
	parsedProfile.Objects = profile.Objects
	return &parsedProfile, nil
}

type WocSyncTask struct {
	of.FileConfig
	SourceDigest *string // Source file digest for verification
	TargetDigest *string // Target file digest for verification
}

// produce file lists by comparing two WocProfile objects
func GenerateFileLists(dstProfile, srcProfile *ParsedWocProfile) map[string]*WocSyncTask {
	var fileList = make(map[string]*WocSyncTask)

	calcDigests := func(file WocFile) {
		if file.Size == nil {
			panic(fmt.Errorf("shard size is nil for file %s", file.Path))
		}
		if file.Digest == nil {
			res, err := SampleMD5(file.Path, 0, 0)
			if err == nil {
				file.Digest = &res.Digest
			} else {
				panic(fmt.Errorf("the digest was not found in profile and failed to calculate for file %s: %v", file.Path, err))
			}
		}
	}

	addFullCopyTask := func(srcFile WocFile) {
		virtualPath := filepath.Base(srcFile.Path)
		if srcFile.Size == nil {
			panic(fmt.Errorf("shard size is nil for file %s", srcFile.Path))
		}
		if srcFile.Digest == nil {
			calcDigests(srcFile)
		}
		fileList[virtualPath] = &WocSyncTask{
			FileConfig: of.FileConfig{
				VirtualPath: virtualPath,
				SourcePath:  srcFile.Path, // Assuming we take the first shard as source
				Offset:      0,
				Size:        int64(*srcFile.Size),
			},
			SourceDigest: srcFile.Digest,
			TargetDigest: nil, // Target digest does not matter
		}
	}

	addPartialCopyTask := func(srcFile WocFile, dstFile WocFile) {
		virtualPath := fmt.Sprintf("%s.offset.%d", filepath.Base(srcFile.Path), int64(*dstFile.Size))
		if srcFile.Size == nil || dstFile.Size == nil {
			panic(fmt.Errorf("shard size is nil for file %s", srcFile.Path))
		}
		if *srcFile.Size < *dstFile.Size {
			print(fmt.Errorf("source file %s size %d is smaller than destination file %s size %d",
				srcFile.Path, *srcFile.Size, dstFile.Path, *dstFile.Size))
			return
		}
		if srcFile.Digest == nil {
			calcDigests(srcFile)
			if srcFile.Digest == nil {
				calcDigests(srcFile)
			}
			if dstFile.Digest == nil {
				calcDigests(dstFile)
			}

			fileList[virtualPath] = &WocSyncTask{
				FileConfig: of.FileConfig{
					VirtualPath: virtualPath,
					SourcePath:  srcFile.Path, // Assuming we take the first shard as source
					Offset:      int64(*dstFile.Size),
					Size:        int64(*srcFile.Size) - int64(*dstFile.Size),
				},
				SourceDigest: srcFile.Digest,
				TargetDigest: nil, // Target digest does not matter
			}
		}
	}

	for k, v := range srcProfile.Maps {
		oldMap, exists := dstProfile.Maps[k]
		if !exists || (exists && v.Version > oldMap.Version) {
			// If versions differ, add the new map to the file list
			// virtual path is the base name of the file
			// add shards
			// Convert larges map to a slice
			var largesSlice []WocFile
			for _, large := range v.Larges {
				largesSlice = append(largesSlice, large)
			}
			shards := append(v.Shards, largesSlice...)
			for _, shard := range shards {
				addFullCopyTask(shard)
			}
		}
	}
	for k, v := range srcProfile.Objects {
		oldMap, exists := dstProfile.Objects[k]
		for i, shard := range v.Shards {
			if !exists {
				addFullCopyTask(shard)
				continue
			}
			oldShard := oldMap.Shards[i]
			partialMd5, err := SampleMD5(shard.Path, 0, int64(*oldShard.Size))
			if oldShard.Size == nil || oldShard.Digest == nil || *oldShard.Digest == "" {
				logger.Error(fmt.Sprintf("the digest was not found in profile for shard %s", shard.Path))
				panic(fmt.Errorf("the digest was not found in profile for shard %s", shard.Path))
			}
			if *oldShard.Size > *shard.Size {
				logger.Warn(fmt.Sprintf("source file %s size %d is smaller than destination file %s size %d",
					shard.Path, *shard.Size, oldShard.Path, *oldShard.Size))
				addFullCopyTask(shard)
				continue
			}
			if err != nil {
				// print the name of the shard and the error
				panic(fmt.Sprintf("failed to calculate digest for shard %s: %v\n", shard.Path, err))
			}
			if partialMd5.Digest != *oldShard.Digest {
				addFullCopyTask(shard)
			} else {
				addPartialCopyTask(shard, oldShard)
			}
		}
	}
	return fileList
}
