//go:build !fuse3
// +build !fuse3

package offsetfs

import "os/exec"

func UmountExec(mountpoint string) error {
	if err := exec.Command("fusermount", "-zu", mountpoint).Run(); err != nil {
		return err
	}
	return nil
}
