package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hrz6976/syncmate/db"
	"github.com/hrz6976/syncmate/offsetfs"
	of "github.com/hrz6976/syncmate/offsetfs"
	"github.com/hrz6976/syncmate/rclone"
	"github.com/hrz6976/syncmate/woc"
	"github.com/rclone/rclone/fs"
	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
)

type CloudflareCredentials struct {
	// Explicitly define the fields to avoid duplicate json tags
	AccountID  string `json:"account_id"`
	ApiToken   string `json:"api_token,omitempty"`
	AccessKey  string `json:"access_key,omitempty"`
	SecretKey  string `json:"secret_key,omitempty"`
	Bucket     string `json:"bucket,omitempty"`
	DatabaseID string `json:"database_id,omitempty"`
}

var dbHandle *db.DB
var config *CloudflareCredentials

func connectDB() (*db.DB, error) {
	if dbHandle != nil {
		return dbHandle, nil
	}
	cloudflareD1Creds := db.CloudflareD1Credentials{
		APIToken:   config.ApiToken,
		DatabaseID: config.DatabaseID,
		AccountID:  config.AccountID,
	}
	gormDB, err := db.ConnectDB(cloudflareD1Creds)
	if err != nil {
		return nil, err
	}
	dbHandle = db.NewDB(gormDB)
	return dbHandle, nil
}

func runSend(
	tasksMap map[string]*woc.WocSyncTask,
) error {
	// 1. Populate the remote database
	if dbHandle != nil {
		for _, task := range tasksMap {
			var srcDigest, dstDigest string
			if task.SourceDigest != nil {
				srcDigest = *task.SourceDigest
			}
			if task.TargetDigest != nil {
				dstDigest = *task.TargetDigest
			}
			if err := dbHandle.UpdateTask(&db.Task{
				VirtualPath: task.VirtualPath,
				Status:      db.Uploading,
				SrcDigest:   srcDigest,
				DstDigest:   dstDigest,
				SrcPath:     task.SourcePath,
				SrcSize:     task.Size,
				DstSize:     task.Offset,
			}); err != nil {
				return fmt.Errorf("failed to upsert task %s: %w", task.VirtualPath, err)
			}
		}
	}

	// 2. Mount OffsetFS (don't block the main thread, listen to signals)
	offsetConfigs := make(map[string]*of.FileConfig)
	for _, task := range tasksMap {
		offsetConfigs[task.VirtualPath] = &of.FileConfig{
			VirtualPath: task.VirtualPath,
			SourcePath:  task.SourcePath,
			Offset:      task.Offset,
			Size:        task.Size,
		}
	}

	mountpoint := "/tmp/syncmate_offsetfs"
	// does the dir exist?
	if _, err := os.Stat(mountpoint); os.IsNotExist(err) {
		// Create the mountpoint directory if it doesn't exist
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return fmt.Errorf("failed to create mountpoint: %w", err)
		}
	} else {
		// If Mountpoint exists, clean it up
		if err := offsetfs.UmountExec(mountpoint); err != nil {
			logger.WithError(err).Error("Failed to unmount existing mountpoint")
		}
	}

	// Clean up any existing mount at this location
	_ = offsetfs.UmountExec(mountpoint)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	filesystem := of.NewOffsetFS(offsetConfigs, true)
	host := fuse.NewFileSystemHost(filesystem)

	options := []string{
		"-o", "fsname=syncmate_offsetfs",
		"-o", "default_permissions",
	}

	var mountWg sync.WaitGroup
	mountWg.Add(1)

	go func() {
		defer mountWg.Done()

		logger.WithField("mountpoint", mountpoint).Info("Mounting OffsetFS...")

		if !host.Mount(mountpoint, options) {
			logger.WithField("mountpoint", mountpoint).Error("Failed to mount OffsetFS")
			return
		}

		<-ctx.Done()
		logger.Info("Unmounting OffsetFS...")
		host.Unmount()
		logger.Info("OffsetFS unmounted successfully")
	}()

	time.Sleep(1 * time.Second) // Give some time for the mount to complete

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	taskDone := make(chan bool, 1)

	go func() {
		select {
		case <-sigChan:
			logger.Info("Received interrupt signal, cleaning up...")
			cancel()
			err := offsetfs.UmountExec(mountpoint)
			if err != nil {
				logger.WithError(err).Error("Failed to unmount OffsetFS on interrupt")
			}
		case <-taskDone:
			logger.Info("Tasks completed, cleaning up...")
			cancel()
			err := offsetfs.UmountExec(mountpoint)
			if err != nil {
				logger.WithError(err).Error("Failed to unmount OffsetFS after tasks completed")
			}
		}
	}()

	go func() {
		defer func() {
			taskDone <- true
		}()

		logger.Info("Starting sync tasks...")

		// 准备要上传的文件列表
		var fileList []string
		for virtualPath := range offsetConfigs {
			fileList = append(fileList, virtualPath)
		}

		if len(fileList) == 0 {
			logger.Info("No files to upload")
			return
		}

		logger.WithField("count", len(fileList)).Info("Uploading files to R2...")

		syncCtx := rclone.InjectConfig(ctx)
		syncCtx = rclone.InjectFileList(syncCtx, fileList)
		r2Creds := &rclone.CloudflareR2Credentials{
			AccessKey: config.AccessKey,
			SecretKey: config.SecretKey,
			AccountID: config.AccountID,
			Bucket:    config.Bucket,
		}
		fdst, err := rclone.NewR2Backend(syncCtx, r2Creds)
		if err != nil {
			logger.WithError(err).Error("Failed to create R2 backend")
			return
		}

		select {
		case <-ctx.Done():
			logger.Info("Upload cancelled before creating local filesystem")
			return
		default:
		}

		fsrc, err := fs.NewFs(syncCtx, mountpoint)
		if err != nil {
			logger.WithError(err).Error("Failed to create local filesystem")
			return
		}

		select {
		case <-ctx.Done():
			logger.Info("Upload cancelled before starting file transfer")
			return
		default:
		}

		uploadDone := make(chan error, 1)

		// 在单独的goroutine中执行上传
		go func() {
			uploadDone <- rclone.Run(syncCtx, func() error {
				return rclone.CopyFiles(syncCtx, fsrc, fdst, fileList)
			})
		}()

		// 等待上传完成或被中断
		select {
		case err := <-uploadDone:
			if err != nil {
				logger.WithError(err).Error("File upload failed")
				return
			}
			logger.Info("File upload completed successfully")
		case <-ctx.Done():
			logger.Info("Upload cancelled by user interrupt")
			// 这里可以添加清理逻辑，比如取消正在进行的上传
			return
		}

		// 更新数据库状态为完成
		if dbHandle != nil {
			logger.Info("Updating task status in database...")
			for _, task := range tasksMap {
				// 再次检查是否被中断
				select {
				case <-ctx.Done():
					logger.Info("Database update cancelled by user interrupt")
					return
				default:
				}

				if err := dbHandle.UpdateTask(&db.Task{
					VirtualPath: task.VirtualPath,
					Status:      db.Uploaded,
					SrcPath:     task.SourcePath,
					SrcSize:     task.Size,
					DstSize:     task.Offset,
					SrcDigest:   *task.SourceDigest,
					DstDigest:   *task.TargetDigest,
				}); err != nil {
					logger.WithError(err).WithField("virtualPath", task.VirtualPath).Error("Failed to update task status in database")
				}
			}
		}

		logger.Info("Sync tasks completed successfully")
	}()

	mountWg.Wait()

	return nil
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Upload files to S3-compatible storage",
	Long:  "Upload files to S3-compatible storage",
	Run: func(cmd *cobra.Command, args []string) {
		srcPath, _ := cmd.Flags().GetString("src")
		dstPath, _ := cmd.Flags().GetString("dst")
		configPath, _ := cmd.Flags().GetString("config")
		skipDB, _ := cmd.Flags().GetBool("skip-db")

		if srcPath == "" || dstPath == "" || configPath == "" {
			cmd.Help()
			return
		}

		if configData, err := os.ReadFile(configPath); err != nil {
			cmd.PrintErrf("Failed to read config file %s: %v\n", configPath, err)
			return
		} else {
			if err := json.Unmarshal(configData, &config); err != nil {
				cmd.PrintErrf("Failed to parse config file %s: %v\n", configPath, err)
				return
			}
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

		if !skipDB {
			_, err = connectDB()
			if err != nil {
				cmd.PrintErrf("Failed to connect to database: %v\n", err)
				return
			}
		}

		tasksMap, err := generateTasks(srcProfile, dstProfile, true)
		if err != nil {
			cmd.PrintErrf("Failed to generate tasks: %v\n", err)
			return
		}

		logger.WithField("taskCount", len(tasksMap)).Info("Generated tasks for file transfer")

		if len(tasksMap) > 0 {
			if err := runSend(tasksMap); err != nil {
				cmd.PrintErrf("Failed to run send operation: %v\n", err)
				return
			}
		} else {
			logger.Info("No tasks to execute")
		}
	},
}

func init() {
	sendCmd.Flags().StringP("src", "s", "woc.src.json", "WoC profile of the transfer source")
	sendCmd.Flags().StringP("dst", "d", "woc.dst.json", "Woc profile of the transfer destination")
	sendCmd.Flags().StringP("config", "c", "config.json", "Path to the configuration file")
	sendCmd.Flags().Bool("skip-db", false, "Skip database operations")
	RootCmd.AddCommand(sendCmd)
}
