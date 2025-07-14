package cmd

import (
	"encoding/json"
	"os"

	"github.com/hrz6976/syncmate/woc"
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

var taskCmd = &cobra.Command{
	Use:   "taskgen",
	Short: "Generate tasks for WoC transfer",
	Long:  "Generate tasks for WoC transfer based on source and destination profiles.",
	Run: func(cmd *cobra.Command, args []string) {
		srcPath, _ := cmd.Flags().GetString("src")
		dstPath, _ := cmd.Flags().GetString("dst")
		outputPath, _ := cmd.Flags().GetString("output")

		if srcPath == "" || dstPath == "" {
			cmd.Help()
			return
		}

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

		fileList := woc.GenerateFileLists(dstProfile, srcProfile)

		if err := writeFileListToJSONL(fileList, outputPath); err != nil {
			cmd.PrintErrf("Failed to write file list to %s: %v\n", outputPath, err)
			return
		}
	},
}

func init() {
	taskCmd.Flags().StringP("src", "s", "", "WoC profile of the transfer source")
	taskCmd.Flags().StringP("dst", "d", "", "Woc profile of the transfer destination")
	taskCmd.Flags().StringP("output", "o", "", "Output file for the generated tasks")
	RootCmd.AddCommand(taskCmd)
}
