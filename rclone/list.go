package rclone

import (
	"context"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/accounting"
	"github.com/rclone/rclone/fs/operations"
)

type RcloneFileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
}

func ListFiles(ctx context.Context, f fs.Fs) ([]RcloneFileInfo, error) {
	var fileInfos []RcloneFileInfo
	err := operations.ListFn(ctx, f, func(o fs.Object) {
		tr := accounting.Stats(ctx).NewCheckingTransfer(o, "listing")
		defer func() {
			tr.Done(ctx, nil)
		}()
		fileInfos = append(fileInfos, RcloneFileInfo{
			Name:    o.Remote(),
			Size:    o.Size(),
			ModTime: o.ModTime(ctx),
		})
	})
	if err != nil {
		return nil, err
	}
	return fileInfos, nil
}
