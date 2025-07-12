package main

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// TestSimpleFileOperations 简化的文件操作测试
func TestSimpleFileOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "offsetfs_simple_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建源文件
	sourceFile := filepath.Join(tmpDir, "source.txt")
	originalContent := "Hello, OffsetFS!"
	err = os.WriteFile(sourceFile, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// 创建配置
	config := &FileConfig{
		VirtualPath: "test.txt",
		SourcePath:  sourceFile,
		Offset:      0,
		Size:        0,
		ReadOnly:    false,
	}

	// 直接测试 OffsetFileNode 而不挂载文件系统
	node := &OffsetFileNode{
		config: config,
	}

	ctx := context.Background()

	// 测试 Getattr
	var attrOut fuse.AttrOut
	errno := node.Getattr(ctx, nil, &attrOut)
	if errno != 0 {
		t.Fatalf("Getattr failed: %v", errno)
	}
	t.Logf("File size: %d, mode: %o", attrOut.Size, attrOut.Mode)

	// 测试 Open
	handle, flags, errno := node.Open(ctx, syscall.O_RDWR)
	if errno != 0 {
		t.Fatalf("Open failed: %v", errno)
	}
	t.Logf("Open successful, handle: %v, flags: %v", handle, flags)

	// 测试 Read
	readBuf := make([]byte, 100)
	result, errno := node.Read(ctx, handle, readBuf, 0)
	if errno != 0 {
		t.Fatalf("Read failed: %v", errno)
	}

	readData, status := result.Bytes(readBuf)
	if status != fuse.OK {
		t.Fatalf("Read result error: %v", status)
	}
	t.Logf("Read data: %s", string(readData))

	if string(readData) != originalContent {
		t.Errorf("Read content = %q, want %q", string(readData), originalContent)
	}

	// 测试 Write
	newContent := "Updated content!"
	written, errno := node.Write(ctx, handle, []byte(newContent), 0)
	if errno != 0 {
		t.Fatalf("Write failed: %v", errno)
	}
	t.Logf("Wrote %d bytes", written)

	// 验证写入结果
	sourceContent, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("Failed to read source file: %v", err)
	}

	if string(sourceContent) != newContent {
		t.Errorf("Source file content = %q, want %q", string(sourceContent), newContent)
	}

	t.Log("Simple file operations test passed!")
}

// TestMountWithMinimalOptions 使用最小选项的挂载测试
func TestMountWithMinimalOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mount test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "offsetfs_minimal_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建源文件
	sourceFile := filepath.Join(tmpDir, "source.txt")
	content := "Minimal test content"
	err = os.WriteFile(sourceFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// 配置
	configs := map[string]*FileConfig{
		"minimal.txt": {
			VirtualPath: "minimal.txt",
			SourcePath:  sourceFile,
			Offset:      0,
			Size:        0,
			ReadOnly:    false,
		},
	}

	root := &OffsetFSRoot{
		configs: configs,
	}

	mountpoint := filepath.Join(tmpDir, "mount")
	err = os.Mkdir(mountpoint, 0755)
	if err != nil {
		t.Fatalf("Failed to create mountpoint: %v", err)
	}

	// 使用最小的挂载选项
	server, err := fs.Mount(mountpoint, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: false,
		},
	})
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	defer func() {
		err := server.Unmount()
		if err != nil {
			t.Logf("Unmount failed: %v", err)
		}
	}()

	// 给文件系统一些时间来初始化
	time.Sleep(100 * time.Millisecond)

	// 测试列出文件
	entries, err := os.ReadDir(mountpoint)
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "minimal.txt" {
		t.Fatalf("Expected one file 'minimal.txt', got: %v", entries)
	}

	// 测试读取
	virtualFile := filepath.Join(mountpoint, "minimal.txt")
	readContent, err := os.ReadFile(virtualFile)
	if err != nil {
		t.Fatalf("Failed to read virtual file: %v", err)
	}

	if string(readContent) != content {
		t.Errorf("Read content = %q, want %q", string(readContent), content)
	}

	t.Log("Minimal mount test passed!")
}
