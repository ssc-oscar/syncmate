package woc

import (
	"context"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/machinebox/progress"
	logger "github.com/sirupsen/logrus"
)

type CopyMode int

const (
	CopyModeOverwrite CopyMode = iota
	CopyModeAppend
)

func MoveFile(
	srcPath,
	dstPath string,
	mode CopyMode,
	expectedDigestAfterTransfer string,
	expectedDstSizeBeforeTransfer int64) error {
	srcStat, err := os.Stat(srcPath)

	// 1. Check before copying
	if err != nil {
		return fmt.Errorf("unable to get source file info: %w", err)
	}
	if !srcStat.Mode().IsRegular() {
		return fmt.Errorf("source file is not a regular file: %s", srcPath)
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("unable to open source file for reading: %w", err)
	}
	defer srcFile.Close()

	// trunc: verify digest now
	if mode == CopyModeOverwrite && expectedDigestAfterTransfer != "" {
		md5Res, err := SampleMD5(srcPath, 0, 0)
		if err != nil {
			return fmt.Errorf("failed to compute source file digest: %w", err)
		}
		if md5Res.Digest != expectedDigestAfterTransfer {
			return fmt.Errorf("source file digest mismatch: expected %s, got %s", expectedDigestAfterTransfer, md5Res.Digest)
		}
	}

	lockFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("unable to open destination file for locking: %w", err)
	}
	defer lockFile.Close()

	// Apply exclusive lock to prevent other processes from accessing this file simultaneously
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("unable to lock destination file: %w", err)
	}
	// Ensure the lock is released when the function exits
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	var openFlags int
	switch mode {
	case CopyModeOverwrite:
		openFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case CopyModeAppend:
		openFlags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	default:
		return fmt.Errorf("invalid copy mode: %d", mode)
	}

	dstFile, err := os.OpenFile(dstPath, openFlags, srcStat.Mode())
	if err != nil {
		return fmt.Errorf("unable to open destination file for writing: %w", err)
	}
	defer dstFile.Close()

	dstStat, err := dstFile.Stat()
	if err != nil {
		return fmt.Errorf("unable to get destination file info: %w", err)
	}
	dstSize := dstStat.Size()
	// append: check size
	if mode == CopyModeAppend && expectedDstSizeBeforeTransfer >= 0 {
		if dstSize != expectedDstSizeBeforeTransfer {
			return fmt.Errorf("destination file size mismatch before transfer: expected %d, got %d", expectedDstSizeBeforeTransfer, dstSize)
		}
	}

	// 2. Do copy
	r := progress.NewReader(srcFile)
	// Start a goroutine printing progress
	go func() {
		ctx := context.Background()
		progressChan := progress.NewTicker(ctx, r, srcStat.Size(), 10*time.Second)
		for p := range progressChan {
			logger.Debugf("Moving file %s->%s, %.1f%% copied, remaining %v", srcPath, dstPath, p.Percent(), p.Remaining().Round(time.Second))
		}
		logger.Infof("Moved file %s successfully", srcPath)
	}()
	written, err := io.Copy(dstFile, r)
	if err != nil {
		return fmt.Errorf("file copy error occurred: %w", err)
	}
	if written != srcStat.Size() {
		return fmt.Errorf("number of bytes copied does not match source file size: expected %d, got %d", srcStat.Size(), written)
	}

	// 3. Check after copying
	// append: verify digest after transfer
	if mode == CopyModeAppend && expectedDigestAfterTransfer != "" {
		md5Res, err := SampleMD5(dstPath, 0, 0)
		if err != nil {
			return fmt.Errorf("failed to compute destination file digest: %w", err)
		}
		if md5Res.Digest != expectedDigestAfterTransfer {
			// shit, rollback!
			// resize destination file, keep the original part
			err = fmt.Errorf("destination file digest mismatch: expected %s, got %s", expectedDigestAfterTransfer, md5Res.Digest)
			logger.WithError(err).Error("File copy failed, rolling back")
			if err := os.Truncate(dstPath, dstSize); err != nil {
				return fmt.Errorf("failed to resize destination file: %w", err)
			}
			return err
		}
	}
	// 4. Delete the source file
	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("failed to delete source file: %w", err)
	}
	return nil
}
