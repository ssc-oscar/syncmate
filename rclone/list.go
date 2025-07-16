package rclone

import (
	"context"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/operations"
)

type RcloneFileInfo struct {
	Name string
	Size int64
}

func ListFiles(ctx context.Context, f fs.Fs) ([]RcloneFileInfo, error) {
	var fileInfos []RcloneFileInfo
	var opt = operations.ListJSONOpt{
		NoModTime:  true,
		NoMimeType: true,
		DirsOnly:   false,
		FilesOnly:  true,
		Recurse:    false,
	}
	err := operations.ListJSON(ctx, f, "", &opt, func(item *operations.ListJSONItem) error {
		if item.IsDir {
			return nil // Skip directories
		}
		fileInfos = append(fileInfos, RcloneFileInfo{
			Name: item.Path,
			Size: item.Size,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileInfos, nil
}
