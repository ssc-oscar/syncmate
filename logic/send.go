package logic

import (
	"context"
	"testing"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/rclone/rclone/fs/sync"
	"github.com/rclone/rclone/fstest"
	"github.com/stretchr/testify/require"
)

func testCopyWithFilesFrom(t *testing.T, noTraverse bool) {
	ctx := context.Background()
	ctx, ci := fs.AddConfig(ctx)
	r := fstest.NewRun(t)
	file1 := r.WriteFile("potato2", "hello world", time.Now())
	file2 := r.WriteFile("hello world2", "hello world2", time.Now())

	// Set the --files-from equivalent
	f, err := filter.NewFilter(nil)
	require.NoError(t, err)
	require.NoError(t, f.AddFile("potato2"))
	require.NoError(t, f.AddFile("notfound"))

	// Change the active filter
	ctx = filter.ReplaceConfig(ctx, f)
	ci.NoTraverse = noTraverse

	err = sync.CopyDir(ctx, r.Fremote, r.Flocal, false)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1, file2)
	r.CheckRemoteItems(t, file1)
}
