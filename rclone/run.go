package rclone

import (
	"context"
	"runtime"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/accounting"
	"github.com/rclone/rclone/fs/cache"
	"github.com/rclone/rclone/lib/terminal"
)

// Run the function with stats and retries if required
func Run(ctx context.Context, f func() error) error {
	ci := fs.GetConfig(ctx)
	var cmdErr error
	stopStats := func() {}
	if ci.Progress {
		stopStats = startProgress()
	}
	for try := 1; try <= ci.Retries; try++ {
		cmdErr = f()
		cmdErr = fs.CountError(ctx, cmdErr)
		lastErr := accounting.GlobalStats().GetLastError()
		if cmdErr == nil {
			cmdErr = lastErr
		}
		if !accounting.GlobalStats().Errored() {
			if try > 1 {
				fs.Errorf(nil, "Attempt %d/%d succeeded", try, ci.Retries)
			}
			break
		}
		if accounting.GlobalStats().HadFatalError() {
			fs.Errorf(nil, "Fatal error received - not attempting retries")
			break
		}
		if accounting.GlobalStats().Errored() && !accounting.GlobalStats().HadRetryError() {
			fs.Errorf(nil, "Can't retry any of the errors - not attempting retries")
			break
		}
		if retryAfter := accounting.GlobalStats().RetryAfter(); !retryAfter.IsZero() {
			d := time.Until(retryAfter)
			if d > 0 {
				fs.Logf(nil, "Received retry after error - sleeping until %s (%v)", retryAfter.Format(time.RFC3339Nano), d)
				time.Sleep(d)
			}
		}
		if lastErr != nil {
			fs.Errorf(nil, "Attempt %d/%d failed with %d errors and: %v", try, ci.Retries, accounting.GlobalStats().GetErrors(), lastErr)
		} else {
			fs.Errorf(nil, "Attempt %d/%d failed with %d errors", try, ci.Retries, accounting.GlobalStats().GetErrors())
		}
		if try < ci.Retries {
			accounting.GlobalStats().ResetErrors()
		}
		if ci.RetriesInterval > 0 {
			time.Sleep(time.Duration(ci.RetriesInterval))
		}
	}
	stopStats()
	if accounting.GlobalStats().Errored() {
		accounting.GlobalStats().Log()
	}
	fs.Debugf(nil, "%d go routines active\n", runtime.NumGoroutine())

	if ci.Progress && ci.ProgressTerminalTitle {
		terminal.WriteTerminalTitle("")
	}
	cache.Clear()
	if lastErr := accounting.GlobalStats().GetLastError(); cmdErr == nil {
		cmdErr = lastErr
	}

	// Log the final error message and exit
	if cmdErr != nil {
		nerrs := accounting.GlobalStats().GetErrors()
		if nerrs <= 1 {
			fs.Logf(nil, "Error: %v", cmdErr)
		} else {
			fs.Logf(nil, "Error: %v (%d errors, showing the last)", cmdErr, nerrs)
		}
	}
	return cmdErr
}
