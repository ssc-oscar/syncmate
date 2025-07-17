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
	"time"

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

func onFileTransferred(task *woc.WocSyncTask, filePath string, destPath string, finishedCallback func(virtualPath string) error) error {
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
	err = finishedCallback(task.VirtualPath)
	if err != nil {
		logger.WithError(err).Errorf("Failed to call finished callback for %s", task.VirtualPath)
		return err
	}
	return nil
}

func processDoneFiles(
	ctx context.Context,
	tasksMap map[string]*woc.WocSyncTask,
	finishedCallback func(virtualPath string) error,
) error {
	downloadedFiles, err := scanDownloadedFiles(tasksMap)
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
	cancelled := false

fileLoop:
	for _, fileInfo := range downloadedFiles {
		// Check for cancellation before starting each file
		select {
		case <-ctx.Done():
			logger.Info("processDoneFiles cancelled by user interrupt")
			cancelled = true
			break fileLoop
		default:
		}

		if cancelled {
			break
		}

		wg.Add(1)
		go func(info downloadedFileInfo) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Check for cancellation before processing each file
			select {
			case <-ctx.Done():
				logger.WithField("file", info.task.VirtualPath).Debug("File processing cancelled")
				return
			default:
			}

			if err := onFileTransferred(info.task, info.filePath, info.destPath, finishedCallback); err != nil {
				logger.WithError(err).WithField("file", info.task.VirtualPath).Error("Failed to process transferred file")
				errChan <- err
			} else {
				logger.WithField("file", info.task.VirtualPath).Debug("Successfully processed transferred file")
			}
		}(fileInfo)
	}

	// Wait for all goroutines to complete or context to be cancelled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All processing completed normally
	case <-ctx.Done():
		// Context cancelled, but we still wait for current goroutines to finish
		logger.Info("Waiting for current file processing to complete before cancelling...")
		wg.Wait()
		return fmt.Errorf("processDoneFiles cancelled by user interrupt")
	}

	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		logger.WithField("errorCount", len(errs)).Error("Some files failed to process")
		return nil
	}

	if cancelled {
		return fmt.Errorf("processDoneFiles cancelled by user interrupt")
	}

	logger.Info("All downloaded files processed successfully")
	return nil
}

func scanDownloadedFiles(tasksMap map[string]*woc.WocSyncTask) ([]downloadedFileInfo, error) {
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

		var destPath string
		if task.TargetPath != "" {
			destPath = task.TargetPath
		} else {
			logger.WithField("virtualPath", virtualPath).Debug("No target path specified for task, using default destination")
			dirPath := filepath.Join(destDir, virtualPathToSubdir(virtualPath))
			// create destination directory if it doesn't exist
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				logger.WithError(err).WithField("dirPath", dirPath).Error("Failed to create destination directory")
				return err
			}
			destPath = filepath.Join(dirPath, virtualPath)
		}

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
	deleteRemote bool,
) error {
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	syncCtx := rclone.InjectConfig(ctx)
	r2Creds := &rclone.CloudflareR2Credentials{
		AccessKey: config.AccessKey,
		SecretKey: config.SecretKey,
		AccountID: config.AccountID,
		Bucket:    config.Bucket,
	}
	fdst, err := fs.NewFs(syncCtx, cacheDir)
	if err != nil {
		logger.WithError(err).Error("Failed to create local filesystem")
		return err
	}
	fsrc, err := rclone.NewR2Backend(syncCtx, r2Creds)
	if err != nil {
		logger.WithError(err).Error("Failed to create R2 backend")
		return err
	}

	logger.Info("Getting existing files from R2...")
	existingFiles, err := rclone.ListFiles(syncCtx, fsrc)
	if err != nil {
		logger.WithError(err).Error("Failed to list files from R2")
	}

	deleteFileFunc := func(virtualPath string) error {
		return nil
	}
	if deleteRemote {
		deleteFileFunc = func(virtualPath string) error {
			logger.WithField("virtualPath", virtualPath).Debug("Deleting file on R2")
			fobj, err := fsrc.NewObject(syncCtx, virtualPath)
			if err != nil {
				logger.WithError(err).WithField("virtualPath", virtualPath).Error("Failed to get object on R2")
				return err
			}
			return operations.DeleteFile(syncCtx, fobj)
		}
	}

	// run process done files
	if err := processDoneFiles(ctx, tasksMap, deleteFileFunc); err != nil {
		return err
	}

	// update task list
	ignoredFilesMap := make(map[string]bool)
	if dbHandle != nil {
		r, err := dbHandle.ListFinishedVirtualPaths()
		for _, v := range r {
			ignoredFilesMap[v] = true
		}
		if err != nil {
			return err
		}
	}

	// if some files are finished but not deleted on R2, run deleteFileFunc for them
	for _, finfo := range existingFiles {
		if _, ok := ignoredFilesMap[finfo.Name]; ok {
			// run delete
			deleteFileFunc(finfo.Name)
		}
	}

	// what files need to sync?
	fileList := make([]string, 0)
	for _, finfo := range existingFiles {
		// if it is ignored, skip it
		if _, ok := ignoredFilesMap[finfo.Name]; ok {
			logger.WithField("virtualPath", finfo.Name).Debug("File is ignored, skipping")
			continue
		}
		// get task by virtual path
		task, exists := tasksMap[finfo.Name]
		if !exists {
			logger.WithField("virtualPath", finfo.Name).Debug("No task found for existing file")
			continue
		}
		if task == nil {
			logger.WithField("virtualPath", finfo.Name).Warn("Task is nil for existing file")
			continue
		}
		if task.Size != finfo.Size {
			logger.WithFields(logger.Fields{
				"virtualPath": finfo.Name,
				"expected":    task.Size,
				"actual":      finfo.Size,
			}).Warn("File size mismatch, skipping file")
			continue // skip files with size mismatch
		}
		fileList = append(fileList, finfo.Name)
		logger.WithFields(logger.Fields{
			"virtualPath": finfo.Name,
			"size":        finfo.Size,
		}).Debug("Found existing file in R2")
	}
	// inject file list into context
	syncCtx = rclone.InjectFileList(syncCtx, fileList)

	downloadDone := make(chan error, 1)
	var copyErr error

	// Start the CopyFiles operation in background
	go func() {
		downloadDone <- rclone.Run(syncCtx, func() error {
			return rclone.CopyFiles(syncCtx, fsrc, fdst, fileList)
		})
	}()

	// Main loop: continuously process done files until CopyFiles completes
	logger.Info("Starting continuous file processing loop")
	for {
		// Run processDoneFiles (this may take a long time and cannot be interrupted)
		logger.Debug("Running processDoneFiles")
		if err := processDoneFiles(ctx, tasksMap, deleteFileFunc); err != nil {
			logger.WithError(err).Warn("processDoneFiles failed, will retry in next iteration")
		}

		// After processDoneFiles completes, check if CopyFiles has finished
		select {
		case copyErr = <-downloadDone:
			// CopyFiles completed, do one final processing and exit
			logger.Info("CopyFiles completed, doing final processDoneFiles")
			if err := processDoneFiles(ctx, tasksMap, deleteFileFunc); err != nil {
				logger.WithError(err).Error("Final processDoneFiles failed")
				if copyErr == nil {
					copyErr = err // Only override if CopyFiles succeeded
				}
			}
			if copyErr != nil {
				logger.WithError(copyErr).Error("File upload failed")
			} else {
				logger.Info("File upload completed successfully")
			}
			return copyErr
		case <-ctx.Done():
			logger.Info("Upload cancelled by user interrupt")
			return fmt.Errorf("upload cancelled by user interrupt")
		default:
			logger.Info("CopyFiles still running, continuing processing loop after 2 minutes")
			// Sleep interruptible for 2 minute
			select {
			case <-ctx.Done():
				logger.Info("Upload cancelled by user interrupt")
				return fmt.Errorf("upload cancelled by user interrupt")
			case <-time.After(2 * time.Minute):
				// Continue processing loop
			}
		}
	}
}

// find which subdirectory the virtual path belongs to
func virtualPathToSubdir(virtualPath string) string {
	// if it contains .idx or .bin, goes to All.blobs
	if strings.Contains(virtualPath, ".idx") || strings.Contains(virtualPath, ".bin") {
		return "All.blobs"
	}
	// if it contains .s, goes to gz
	if strings.Contains(virtualPath, ".s") {
		return "gz"
	}
	// if it starts with sha1., goes to All.sha1o
	if strings.HasPrefix(virtualPath, "sha1.") {
		return "All.sha1o"
	}
	// if it has Full in it, goes to basemaps
	if strings.Contains(virtualPath, "Full") {
		return "basemaps"
	}
	// otherwise goes to All.sha1c
	return "All.sha1c"
}

var cacheDir string
var destDir string

var recvCmd = &cobra.Command{
	Use:   "recv",
	Short: "Receive files from S3-compatible storage",
	Long:  "Receive files from S3-compatible storage",
	Run: func(cmd *cobra.Command, args []string) {
		srcPath, _ := cmd.Flags().GetString("src")
		dstPath, _ := cmd.Flags().GetString("dst")
		configPath, _ := cmd.Flags().GetString("config")
		skipDB, _ := cmd.Flags().GetBool("skip-db")
		cacheDir, _ = cmd.Flags().GetString("cache-dir")
		destDir, _ = cmd.Flags().GetString("dest-dir")
		deleteRemote, _ := cmd.Flags().GetBool("delete-remote")

		if destDir == "" {
			destDir = cacheDir // use cacheDir as default destination directory
		}

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

		tasksMap, err := generateTasks(srcProfile, dstProfile, false)
		if err != nil {
			cmd.PrintErrf("Failed to generate tasks: %v\n", err)
			return
		}

		logger.WithField("taskCount", len(tasksMap)).Info("Generated tasks for file transfer")

		if len(tasksMap) > 0 {
			if err := runRecv(cacheDir, tasksMap, deleteRemote); err != nil {
				cmd.PrintErrf("Failed to run file transfer: %v\n", err)
				return
			}
			logger.Info("File transfer completed successfully")
		} else {
			logger.Info("No tasks to execute, skipping file transfer")
			return
		}
	},
}

func init() {
	recvCmd.Flags().StringP("src", "s", "woc.src.json", "WoC profile of the transfer source")
	recvCmd.Flags().StringP("dst", "d", "woc.dst.json", "Woc profile of the transfer destination")
	recvCmd.Flags().StringP("config", "c", "config.json", "Path to the configuration file")
	recvCmd.Flags().StringP("cache-dir", "C", "", "Path to the cache directory")
	recvCmd.Flags().StringP("dest-dir", "D", "", "Default destination directory for downloaded files. Uses cache-dir if not specified")
	recvCmd.Flags().Bool("skip-db", false, "Skip database operations (useful for testing)")
	recvCmd.Flags().Bool("delete-remote", true, "Delete files on remote after download")
	recvCmd.MarkFlagRequired("cache-dir")
	RootCmd.AddCommand(recvCmd)
}
