package rclone

import (
	"context"
	"fmt"

	"github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/filter"
)

// InjectGlobalConfig injects global configuration into the context.
func InjectGlobalConfig(ctx context.Context) context.Context {
	ctx, ci := fs.AddConfig(ctx)
	ci.Progress = true
	ci.LogLevel = fs.LogLevelInfo
	ci.Retries = 10
	ci.LowLevelRetries = 100
	return ctx
}

// InjectFileList injects a list of files into the context for filtering.
func InjectFileList(ctx context.Context, files []string) context.Context {
	f, err := filter.NewFilter(nil)
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		if err := f.AddFile(file); err != nil {
			panic(err)
		}
	}
	return filter.ReplaceConfig(ctx, f)
}

type CloudflareR2Credentials struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	AccountID string `json:"account_id"`
	Bucket    string `json:"bucket"`
}

func NewR2Backend(ctx context.Context, cred *CloudflareR2Credentials) (fs.Fs, error) {
	if cred == nil {
		return nil, fmt.Errorf("Cloudflare R2 credentials are required")
	}

	mopt := configmap.New()
	mopt.Set("access_key_id", cred.AccessKey)
	mopt.Set("secret_access_key", cred.SecretKey)
	mopt.Set("endpoint", fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cred.AccountID))
	mopt.Set("region", "auto")
	mopt.Set("no_check_bucket", "true")

	f, err := s3.NewFs(ctx, "r2:", cred.Bucket, mopt)
	if err != nil {
		return nil, err
	}
	return f, nil
}
