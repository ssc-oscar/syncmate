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

func isFileIgnored(filePath string) (bool, error) {
	var statfs syscall.Statfs_t

	err := syscall.Statfs(filePath, &statfs)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // File does not exist, treat as ignored
		}
		return false, err
	}

	if statfs.Type == NFS_SUPER_MAGIC {
		return true, nil
	}

	return false, nil
}

func generateTasks(
	srcProfile,
	dstProfile *woc.ParsedWocProfile) (map[string]*woc.WocSyncTask, error) {
	tasksMap := woc.GenerateFileLists(dstProfile, srcProfile)
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
			task.SourcePath = "/" + strings.TrimPrefix(task.SourcePath, "/"+shortHostName+"_")
			logger.WithField("file", task.VirtualPath).Debugf("Resolved source path to %s", task.SourcePath)
		}

		if isIgnored, err := isFileIgnored(task.SourcePath); err != nil {
			return nil, err
		} else if isIgnored {
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
		if localOnly {
			fileList, err = generateTasks(srcProfile, dstProfile)
			if err != nil {
				cmd.PrintErrf("Failed to generate tasks: %v\n", err)
				return
			}
		} else {
			fileList = woc.GenerateFileLists(dstProfile, srcProfile)
		}

		if err := writeFileListToJSONL(fileList, outputPath); err != nil {
			cmd.PrintErrf("Failed to write file list to %s: %v\n", outputPath, err)
			return
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
