package logic

import (
	"context"
	"testing"

	"github.com/rclone/rclone/fs"
)

func CreateContextWithConfig(t *testing.T) context.Context {
	ctx := context.Background()
	ctx, ci := fs.AddConfig(ctx)
	ci.NoTraverse = false // Set to false to allow traversal in tests
	return ctx
}
