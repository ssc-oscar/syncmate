package woc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	of "github.com/hrz6976/syncmate/offsetfs" // Assuming offsetfs is the package where WocFile, WocObject, WocMap, and WocProfile are defined
	logger "github.com/sirupsen/logrus"
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

// quirk on da* servers: resolve /da?_data to /data on da?.eecs.utk.edu
// the NFS trick does not work anymore because /da?_data are mounted as NFS
func RelocatePath(fname *string) error {
	hostName, err := os.Hostname()
	if err != nil {
		return err
	}
	if fname == nil || *fname == "" {
		return fmt.Errorf("file name cannot be empty")
	}
	shortHostName := strings.Split(hostName, ".")[0]
	if shortHostName == "ishia" {
		shortHostName = "da7" // treat ishia as da7 for compatibility
	}
	if strings.HasPrefix(*fname, "/"+shortHostName) {
		switch shortHostName {
		case "da8":
			*fname = "/mnt/ordos/data/data" + strings.TrimPrefix(*fname, "/da8_data")
		case "da7":
			*fname = "/corrino" + strings.TrimPrefix(*fname, "/da7_data")
		default:
			*fname = "/" + strings.TrimPrefix(*fname, "/"+shortHostName+"_")
		}
		logger.WithField("file", *fname).Debugf("Resolved source path to %s", *fname)
	}
	return nil
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
	TargetPath   string  `json:"target_path"`             // Destination path for the file
	SourceDigest *string `json:"source_digest,omitempty"` // Source file digest for verification
	TargetDigest *string `json:"target_digest,omitempty"` // Target file digest for verification
}

// produce file lists by comparing two WocProfile objects
func GenerateFileLists(dstProfile, srcProfile *ParsedWocProfile) map[string]*WocSyncTask {
	var fileList = make(map[string]*WocSyncTask)

	calcDigests := func(file WocFile) {
		if file.Size == nil {
			panic(fmt.Errorf("shard size is nil for file %s", file.Path))
		}
		if file.Digest == nil {
			logger.WithFields(logger.Fields{
				"path": file.Path,
				"size": *file.Size,
			}).Debug("Calculating sample MD5 for file")
			res, err := SampleMD5(file.Path, 0, 0)
			if err == nil {
				file.Digest = &res.Digest
			} else {
				logger.WithField("path", file.Path).WithError(err).Error("failed to calculate sample md5, were the profiles generated with --with-digest?")
				panic(err)
			}
		}
	}

	addFullCopyTask := func(srcFile WocFile, dstFile *WocFile) {
		virtualPath := filepath.Base(srcFile.Path)
		if srcFile.Size == nil {
			panic(fmt.Errorf("shard size is nil for file %s", srcFile.Path))
		}
		if srcFile.Digest == nil {
			calcDigests(srcFile)
		}
		var tarPath string
		if dstFile != nil {
			tarPath = dstFile.Path
		}
		fileList[virtualPath] = &WocSyncTask{
			FileConfig: of.FileConfig{
				VirtualPath: virtualPath,
				SourcePath:  srcFile.Path, // Assuming we take the first shard as source
				Offset:      0,
				Size:        int64(*srcFile.Size),
			},
			TargetPath:   tarPath,        // Destination path for the file
			SourceDigest: srcFile.Digest, // Source digest for verification
			TargetDigest: nil,            // Target digest does not matter
		}
	}

	addPartialCopyTask := func(srcFile WocFile, dstFile WocFile) {
		virtualPath := fmt.Sprintf("%s.offset.%d", filepath.Base(srcFile.Path), int64(*dstFile.Size))
		if srcFile.Size == nil || dstFile.Size == nil {
			panic(fmt.Errorf("shard size is nil for file %s", srcFile.Path))
		}
		if *srcFile.Size < *dstFile.Size {
			logger.Warn(fmt.Sprintf("source file %s size %d is smaller than destination file %s size %d",
				srcFile.Path, *srcFile.Size, dstFile.Path, *dstFile.Size))
			addFullCopyTask(srcFile, &dstFile)
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
		}
		fileList[virtualPath] = &WocSyncTask{
			FileConfig: of.FileConfig{
				VirtualPath: virtualPath,
				SourcePath:  srcFile.Path, // Assuming we take the first shard as source
				Offset:      int64(*dstFile.Size),
				Size:        int64(*srcFile.Size) - int64(*dstFile.Size),
			},
			TargetPath:   dstFile.Path, // Destination path for the file
			SourceDigest: srcFile.Digest,
			TargetDigest: dstFile.Digest,
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
				addFullCopyTask(shard, nil)
			}
		}
	}
	for k, v := range srcProfile.Objects {
		oldMap, exists := dstProfile.Objects[k]
		for i, shard := range v.Shards {
			if !exists {
				addFullCopyTask(shard, nil)
				continue
			}
			oldShard := oldMap.Shards[i]
			if *oldShard.Size > *shard.Size {
				logger.Warn(fmt.Sprintf("source file %s size %d is smaller than destination file %s size %d",
					shard.Path, *shard.Size, oldShard.Path, *oldShard.Size))
				addFullCopyTask(shard, &oldShard)
				continue
			}

			// rewrite shard.Path to the full path
			if err := RelocatePath(&shard.Path); err != nil {
				logger.WithField("path", shard.Path).WithError(err).Error("Failed to relocate path")
				panic(err)
			}

			// On the destination, we can never check the digest of source files.
			// So it adds both the full copy and the partial copy tasks.
			// File will be copied in full if the file exists on the remote.
			partialMd5, err := SampleMD5(shard.Path, 0, int64(*oldShard.Size))
			if err != nil {
				if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") {
					logger.Debug("Source file missing. Add both full and partial tasks and skip digest verification.", "path", shard.Path)
					addFullCopyTask(shard, &oldShard)
				} else {
					logger.WithError(err).WithField("path", shard.Path).Error("Failed to calculate sample MD5")
					panic(err)
				}
			} else { // here we have a valid partial MD5
				logger.WithFields(logger.Fields{
					"path":   shard.Path,
					"size":   *shard.Size,
					"digest": partialMd5.Digest,
				}).Debug("Calculated partial MD5 for shard")
				if partialMd5.Digest != *oldShard.Digest {
					logger.Debug(fmt.Sprintf("partial MD5 mismatch for shard %s: %s != %s",
						shard.Path, partialMd5.Digest, *oldShard.Digest))
					addFullCopyTask(shard, &oldShard)
					continue
				}
			}
			addPartialCopyTask(shard, oldShard)
		}
	}
	return fileList
}
