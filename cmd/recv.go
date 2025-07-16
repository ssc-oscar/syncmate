package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/hrz6976/syncmate/db"
	"github.com/hrz6976/syncmate/rclone"
	"github.com/hrz6976/syncmate/woc"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/operations"
	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// downloadedFileInfo 包含已下载文件的信息
type downloadedFileInfo struct {
	task     *woc.WocSyncTask
	filePath string
	destPath string
}

func onFileTransferred(task *woc.WocSyncTask, filePath string, destPath string) error {
	isPartial := strings.Contains(filePath, ".offset.")
	copyMode := woc.CopyModeOverwrite
	var expectedDstSizeBeforeTransfer int64
	if isPartial {
		copyMode = woc.CopyModeAppend
		parts := strings.Split(filePath, ".offset.")
		if len(parts) > 1 {
			if size, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil {
				expectedDstSizeBeforeTransfer = size
			}
		}
	}
	var sourceDigest string
	if task.SourceDigest != nil {
		sourceDigest = *task.SourceDigest
	}
	err := woc.MoveFile(
		filePath,
		destPath,
		copyMode,
		sourceDigest,
		expectedDstSizeBeforeTransfer,
	)
	if err != nil {
		return err
	}
	if dbHandle == nil {
		return nil
	}
	err = dbHandle.UpdateTask(&db.Task{
		VirtualPath: task.VirtualPath,
		SrcPath:     task.SourcePath,
		DstPath:     destPath,
		SrcSize:     task.Size,
		SrcDigest:   sourceDigest,
		DstSize:     task.Size,
		Status:      db.Downloaded,
	})
	if err != nil {
		logger.WithError(err).Errorf("Failed to update task for %s", task.VirtualPath)
		return err
	}
	return nil
}

func processDoneFiles(
	cacheDir string, tasksMap map[string]*woc.WocSyncTask,
) error {
	downloadedFiles, err := scanDownloadedFiles(cacheDir, tasksMap)
	if err != nil {
		return err
	}

	if len(downloadedFiles) == 0 {
		logger.Info("No downloaded files found in cache directory")
		return nil
	}

	logger.WithField("fileCount", len(downloadedFiles)).Info("Found downloaded files, processing with goroutines")

	const maxConcurrency = 10
	semaphore := make(chan struct{}, maxConcurrency)

	var wg sync.WaitGroup
	errChan := make(chan error, len(downloadedFiles))

	for _, fileInfo := range downloadedFiles {
		wg.Add(1)
		go func(info downloadedFileInfo) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := onFileTransferred(info.task, info.filePath, info.destPath); err != nil {
				logger.WithError(err).WithField("file", info.task.VirtualPath).Error("Failed to process transferred file")
				errChan <- err
			} else {
				logger.WithField("file", info.task.VirtualPath).Debug("Successfully processed transferred file")
			}
		}(fileInfo)
	}

	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		logger.WithField("errorCount", len(errs)).Error("Some files failed to process")
		return errs[0]
	}

	logger.Info("All downloaded files processed successfully")
	return nil
}

func scanDownloadedFiles(cacheDir string, tasksMap map[string]*woc.WocSyncTask) ([]downloadedFileInfo, error) {
	var downloadedFiles []downloadedFileInfo

	if cacheDir == "" {
		return downloadedFiles, nil
	}

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		logger.WithField("cacheDir", cacheDir).Debug("Cache directory does not exist")
		return downloadedFiles, nil
	}

	err := filepath.Walk(cacheDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			logger.WithError(err).WithField("path", filePath).Warn("Error accessing file")
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(cacheDir, filePath)
		if err != nil {
			logger.WithError(err).WithField("filePath", filePath).Warn("Failed to calculate relative path")
			return nil
		}

		virtualPath := filepath.ToSlash(relPath)

		task, exists := tasksMap[virtualPath]
		if !exists {
			logger.WithField("virtualPath", virtualPath).Debug("No task found for downloaded file")
			return nil
		}

		if task == nil {
			logger.WithField("virtualPath", virtualPath).Warn("Task is nil for downloaded file")
			return nil
		}

		destPath := filepath.Join(cacheDir, virtualPath)

		// check file size
		if info.Size() != task.Size {
			logger.WithFields(logger.Fields{
				"path":     filePath,
				"expected": task.Size,
				"actual":   info.Size(),
			}).Warn("File size mismatch, skipping file")
			return nil // skip files with size mismatch
		}

		downloadedFiles = append(downloadedFiles, downloadedFileInfo{
			task:     task,
			filePath: filePath,
			destPath: destPath,
		})

		logger.WithFields(logger.Fields{
			"virtualPath": virtualPath,
			"filePath":    filePath,
			"destPath":    destPath,
		}).Debug("Found downloaded file")

		return nil
	})

	if err != nil {
		return nil, err
	}

	return downloadedFiles, nil
}

func runRecv(
	cacheDir string,
	tasksMap map[string]*woc.WocSyncTask,
) error {
	var err error

	// run process done files
	if err := processDoneFiles(cacheDir, tasksMap); err != nil {
		return fmt.Errorf("failed to process downloaded files: %w", err)
	}
	// update task list
	ignoredFiles := make([]string, 0)
	if dbHandle != nil {
		ignoredFiles, err = dbHandle.ListFinishedVirtualPaths()
		if err != nil {
			return err
		}
	}

	taskDone := make(chan bool, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	syncCtx := rclone.InjectConfig(ctx)
	r2Creds := &rclone.CloudflareR2Credentials{
		AccessKey: config.AccessKey,
		SecretKey: config.SecretKey,
		AccountID: config.AccountID,
		Bucket:    config.Bucket,
	}
	fsrc, err := rclone.NewR2Backend(syncCtx, r2Creds)
	if err != nil {
		logger.WithError(err).Error("Failed to create R2 backend")
		return err
	}

	// Run lsf to get files and sizes
	filesAndSizes, err := operations.List()

	go func() {
		defer func() {
			taskDone <- true
		}()

		logger.Info("Starting sync tasks...")

		// 准备要上传的文件列表
		var fileList []string

		if len(fileList) == 0 {
			logger.Info("No files to upload")
			return
		}

		logger.WithField("count", len(fileList)).Info("Uploading files to R2...")

		select {
		case <-ctx.Done():
			logger.Info("Upload cancelled before creating local filesystem")
			return
		default:
		}

		fdst, err := fs.NewFs(syncCtx, cacheDir)
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

	return nil
}

var recvCmd = &cobra.Command{
	Use:   "recv",
	Short: "Receive files from S3-compatible storage",
	Long:  "Receive files from S3-compatible storage",
	Run: func(cmd *cobra.Command, args []string) {
		srcPath, _ := cmd.Flags().GetString("src")
		dstPath, _ := cmd.Flags().GetString("dst")
		configPath, _ := cmd.Flags().GetString("config")
		skipDB, _ := cmd.Flags().GetBool("skip-db")
		cacheDir, _ := cmd.Flags().GetString("cache-dir")

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

		tasksMap, err := generateTasks(srcProfile, dstProfile)
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

			// 处理已下载的文件
			if err := processDoneFiles(cacheDir, tasksMap); err != nil {
				cmd.PrintErrf("Failed to process downloaded files: %v\n", err)
				return
			}
		} else {
			logger.Info("No tasks to execute")

			// 即使没有新任务，也要处理已下载的文件
			if err := processDoneFiles(cacheDir, tasksMap); err != nil {
				cmd.PrintErrf("Failed to process downloaded files: %v\n", err)
				return
			}
		}
	},
}

func init() {
	recvCmd.Flags().StringP("src", "s", "woc.src.json", "WoC profile of the transfer source")
	recvCmd.Flags().StringP("dst", "d", "woc.dst.json", "Woc profile of the transfer destination")
	recvCmd.Flags().StringP("config", "c", "config.json", "Path to the configuration file")
	recvCmd.Flags().StringP("cache-dir", "C", "", "Path to the cache directory")
	RootCmd.AddCommand(recvCmd)
}
