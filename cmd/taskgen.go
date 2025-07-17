package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"syscall"

	"github.com/hrz6976/syncmate/woc"
	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func writeFileListToJSONL(fileList map[string]*woc.WocSyncTask, outputPath string) error {
	// if outputPath is empty, use stdout
	var file *os.File
	if outputPath == "" {
		file = os.Stdout
	} else {
		// Create or truncate the output file
		var err error
		file, err = os.Create(outputPath)
		if err != nil {
			return err
		}
		defer file.Close()
	}
	for _, task := range fileList {
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		if _, err := file.WriteString(string(data) + "\n"); err != nil {
			return err
		}
	}
	return nil
}

const NFS_SUPER_MAGIC = 0x6969

func isFileLocal(filePath string) (bool, error) {
	var statfs syscall.Statfs_t

	err := syscall.Statfs(filePath, &statfs)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // File does not exist, treat as non-local
		}
		// Other errors are unexpected, return them
		return false, err
	}

	if statfs.Type == NFS_SUPER_MAGIC {
		return false, nil
	}

	return true, nil
}

func generateTasks(
	srcProfile,
	dstProfile *woc.ParsedWocProfile,
	localOnly bool,
) (map[string]*woc.WocSyncTask, error) {
	tasksMap := woc.GenerateFileLists(dstProfile, srcProfile)
	logger.WithField("taskCount", len(tasksMap)).Debug("Generated tasks for file transfer")
	var finishedFiles []string
	var err error
	if dbHandle != nil {
		finishedFiles, err = dbHandle.ListFinishedVirtualPaths()
		if err != nil {
			return nil, err
		}
	}
	finishedFilesMap := make(map[string]bool)
	for _, file := range finishedFiles {
		finishedFilesMap[file] = true
	}
	for _, task := range tasksMap {
		if task.VirtualPath != "" && finishedFilesMap[task.VirtualPath] {
			logger.WithField("file", task.VirtualPath).Debug("Skipping already finished task")
			delete(tasksMap, task.VirtualPath)
		}

		// quirk on da* servers: resolve /da?_data to /data on da?.eecs.utk.edu
		// the NFS trick does not work anymore because /da?_data are mounted as NFS
		hostName, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		shortHostName := strings.Split(hostName, ".")[0]
		if strings.HasPrefix(task.SourcePath, "/"+shortHostName) {
			switch shortHostName {
			case "da8":
				task.SourcePath = "/mnt/ordos/data/data/" + strings.TrimPrefix(task.SourcePath, "/da8_data")
			case "da7":
				task.SourcePath = "/corrino/" + strings.TrimPrefix(task.SourcePath, "/da7_data")
			default:
				task.SourcePath = "/" + strings.TrimPrefix(task.SourcePath, "/"+shortHostName+"_")
			}
			logger.WithField("file", task.VirtualPath).Debugf("Resolved source path to %s", task.SourcePath)
		}

		if isLocal, err := isFileLocal(task.SourcePath); err != nil {
			return nil, err
		} else if !isLocal && localOnly {
			// Skip tasks for files on NFS
			logger.WithField("file", task.SourcePath).Debug("File ignored")
			delete(tasksMap, task.VirtualPath)
			continue
		}
	}
	return tasksMap, nil
}

var taskCmd = &cobra.Command{
	Use:   "taskgen",
	Short: "Generate tasks for WoC transfer",
	Long:  "Generate tasks for WoC transfer based on source and destination profiles.",
	Run: func(cmd *cobra.Command, args []string) {
		srcPath, _ := cmd.Flags().GetString("src")
		dstPath, _ := cmd.Flags().GetString("dst")
		outputPath, _ := cmd.Flags().GetString("output")
		localOnly, _ := cmd.Flags().GetBool("local-only")

		srcProfile, err := woc.ParseWocProfile(&srcPath)
		if err != nil {
			cmd.PrintErrf("Failed to parse source profile: %v\n", err)
			return
		}

		dstProfile, err := woc.ParseWocProfile(&dstPath)
		if err != nil {
			cmd.PrintErrf("Failed to parse destination profile: %v\n", err)
			return
		}

		var fileList map[string]*woc.WocSyncTask
		fileList, err = generateTasks(srcProfile, dstProfile, localOnly)
		if err != nil {
			panic(err)
		}
		if err := writeFileListToJSONL(fileList, outputPath); err != nil {
			panic(err)
		}
	},
}

func init() {
	taskCmd.Flags().StringP("src", "s", "woc.src.json", "WoC profile of the transfer source")
	taskCmd.Flags().StringP("dst", "d", "woc.dst.json", "Woc profile of the transfer destination")
	taskCmd.Flags().StringP("output", "o", "", "Output file for the generated tasks")
	taskCmd.Flags().Bool("local-only", false, "Generate tasks for local files only, ignoring nonexisting files")
	RootCmd.AddCommand(taskCmd)
}
