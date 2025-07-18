package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/hrz6976/syncmate/db"
	"github.com/hrz6976/syncmate/rclone"
	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

type StatusSummary struct {
	Count int64
	Size  int64
}

func runStatus(configPath string, skipDB bool) error {
	// Load configuration
	if configData, err := os.ReadFile(configPath); err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	} else {
		if err := json.Unmarshal(configData, &config); err != nil {
			return fmt.Errorf("failed to parse config file %s: %w", configPath, err)
		}
	}

	stats := make(map[db.Status]StatusSummary)

	// Database statistics
	if !skipDB {
		dbHandle, err := connectDB()
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		// Get statistics for each status
		statuses := []struct {
			status db.Status
			name   string
		}{
			{db.Uploading, "Uploading"},
			{db.Downloaded, "Downloaded"},
		}

		for _, statusInfo := range statuses {
			summary, err := dbHandle.GetTasksByStatus(statusInfo.status)
			if err != nil {
				logger.WithError(err).WithField("status", statusInfo.name).Warn("Failed to get status summary")
				stats[statusInfo.status] = StatusSummary{
					Count: 0,
					Size:  0,
				}
				continue
			}
			stats[statusInfo.status] = StatusSummary{
				Count: summary.Count,
				Size:  summary.Size,
			}
		}
	} else {
		fmt.Println("Database Status: Skipped (--skip-db flag)")
	}

	ctx := rclone.InjectConfig(context.Background())
	r2Creds := &rclone.CloudflareR2Credentials{
		AccessKey: config.AccessKey,
		SecretKey: config.SecretKey,
		AccountID: config.AccountID,
		Bucket:    config.Bucket,
	}

	fdst, err := rclone.NewR2Backend(ctx, r2Creds)
	if err != nil {
		fmt.Printf("R2 Backend: Error connecting - %v\n", err)
	}

	fileInfos, err := rclone.ListFiles(ctx, fdst)
	if err != nil {
		fmt.Printf("R2 Backend: Error listing files - %v\n", err)
	}

	var totalSize int64
	for _, fileInfo := range fileInfos {
		totalSize += fileInfo.Size
	}
	stats[db.Uploaded] = StatusSummary{
		Count: int64(len(fileInfos)),
		Size:  totalSize,
	}
	// recalculate uploading: should be uploading - uploaded
	// because we can't run a callback after each file is uploaded
	// stats[db.Uploading] = StatusSummary{
	// 	Count: stats[db.Uploading].Count,
	// 	Size:  stats[db.Uploading].Size - stats[db.Uploaded].Size,
	// }

	fmt.Printf("%-12s %-8s %-12s\n", "Status", "Count", "Total Size")
	fmt.Printf("%-12s %-8s %-12s\n", "------", "-----", "----------")
	for k, stat := range stats {
		fmt.Printf("%-12s %-8d %-12s\n", k.String(), stat.Count, formatSize(stat.Size))
	}

	return err
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show transfer progress and statistics",
	Long:  "Display statistics about file transfer progress, including database status and R2 backend file counts",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		skipDB, _ := cmd.Flags().GetBool("skip-db")

		if configPath == "" {
			cmd.Help()
			return
		}

		if err := runStatus(configPath, skipDB); err != nil {
			panic(err)
		}
	},
}

func init() {
	statusCmd.Flags().StringP("config", "c", "config.json", "Path to the configuration file")
	statusCmd.Flags().Bool("skip-db", false, "Skip database operations")
	RootCmd.AddCommand(statusCmd)
}
