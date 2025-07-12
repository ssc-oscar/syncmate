// offsetfs.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FileConfig 定义了 JSONL 配置文件中的每一行
type FileConfig struct {
	VirtualPath string `json:"virtual_path"` // 虚拟文件系统中的文件路径
	SourcePath  string `json:"source_path"`  // 源文件路径
	Offset      int64  `json:"offset"`       // 从源文件的偏移量开始
	Size        int64  `json:"size"`         // 要映射的字节数，0表示到文件末尾
	ReadOnly    bool   `json:"read_only"`    // 是否启用只读模式
}

// OffsetFileNode 代表一个带偏移量的虚拟文件
type OffsetFileNode struct {
	fs.Inode

	config *FileConfig
}

// 确保OffsetFileNode实现必要的接口
var _ = (fs.NodeGetattrer)((*OffsetFileNode)(nil))
var _ = (fs.NodeOpener)((*OffsetFileNode)(nil))
var _ = (fs.NodeReader)((*OffsetFileNode)(nil))
var _ = (fs.NodeWriter)((*OffsetFileNode)(nil))
var _ = (fs.NodeSetattrer)((*OffsetFileNode)(nil))

// Getattr 返回文件属性
func (n *OffsetFileNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// 获取源文件信息，如果文件不存在则返回默认值
	sourceInfo, err := os.Stat(n.config.SourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，但我们可能会创建它，所以返回默认属性
			if n.config.ReadOnly {
				out.Mode = 0444 // 只读权限
			} else {
				out.Mode = 0644 // 读写权限
			}
			out.Size = 0 // 新文件大小为0
			// 设置当前时间作为默认时间戳
			now := uint64(time.Now().Unix())
			out.Mtime = now
			out.Atime = now
			out.Ctime = now
			return 0
		}
		log.Printf("Error getting source file info: %v", err)
		return syscall.EIO
	}

	// 设置文件模式
	if n.config.ReadOnly {
		out.Mode = 0444 // 只读权限
	} else {
		out.Mode = 0644 // 读写权限
	}

	// 计算实际大小
	if n.config.Offset == 0 && n.config.Size == 0 {
		// 直接访问整个文件
		out.Size = uint64(sourceInfo.Size())
	} else if n.config.Size == 0 {
		// 从offset到文件末尾
		if n.config.Offset >= sourceInfo.Size() {
			out.Size = 0
		} else {
			out.Size = uint64(sourceInfo.Size() - n.config.Offset)
		}
	} else {
		// 指定大小 - 返回配置大小和实际大小中的较小值
		configSize := uint64(n.config.Size)
		if n.config.Offset == 0 {
			// 直接访问模式下，取配置大小和实际大小的最小值
			actualSize := uint64(sourceInfo.Size())
			if actualSize < configSize {
				out.Size = actualSize
			} else {
				out.Size = configSize
			}
		} else {
			// 偏移模式下，计算从偏移量开始的实际可用大小
			availableSize := uint64(0)
			if sourceInfo.Size() > n.config.Offset {
				availableSize = uint64(sourceInfo.Size() - n.config.Offset)
			}
			if availableSize < configSize {
				out.Size = availableSize
			} else {
				out.Size = configSize
			}
		}
	}

	// 设置时间戳等其他属性
	out.Mtime = uint64(sourceInfo.ModTime().Unix())
	out.Atime = uint64(sourceInfo.ModTime().Unix())
	out.Ctime = uint64(sourceInfo.ModTime().Unix())

	return 0
}

// Open 当文件被打开时调用
func (n *OffsetFileNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// 检查文件是否存在
	_, err := os.Stat(n.config.SourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在
			// 如果是只读模式，不允许创建文件
			if n.config.ReadOnly {
				log.Printf("Cannot open non-existent file in read-only mode: %v", err)
				return nil, 0, syscall.ENOENT
			}
			// 如果是写入模式，我们稍后在 Write 方法中创建文件，这里先允许打开
		} else {
			log.Printf("Error checking source file: %v", err)
			return nil, 0, syscall.EIO
		}
	}

	// 如果是只读模式，检查是否试图写入
	if n.config.ReadOnly && (flags&syscall.O_WRONLY != 0 || flags&syscall.O_RDWR != 0) {
		log.Printf("Cannot open file in read-write mode when it is read-only: %s", n.config.VirtualPath)
		return nil, 0, syscall.EACCES
	}

	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

// Read 处理读请求
func (n *OffsetFileNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	// 打开源文件
	sourceFile, err := os.Open(n.config.SourcePath)
	if err != nil {
		log.Printf("Error opening source file for reading: %v", err)
		return nil, syscall.EIO
	}
	defer sourceFile.Close()

	var actualOffset int64
	var maxReadSize int64

	if n.config.Offset == 0 && n.config.Size == 0 {
		// 直接访问模式
		actualOffset = off
		maxReadSize = int64(len(dest))
	} else {
		// 偏移访问模式
		actualOffset = n.config.Offset + off

		// 计算可读取的最大字节数
		if n.config.Size > 0 {
			remaining := n.config.Size - off
			if remaining <= 0 {
				return fuse.ReadResultData([]byte{}), 0
			}
			maxReadSize = min(int64(len(dest)), remaining)
		} else {
			maxReadSize = int64(len(dest))
		}
	}

	// 检查是否超出文件范围
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		log.Printf("Error getting source file stat: %v", err)
		return nil, syscall.EIO
	}

	if actualOffset >= sourceInfo.Size() {
		return fuse.ReadResultData([]byte{}), 0
	}

	// 调整读取大小，不超过文件末尾
	if actualOffset+maxReadSize > sourceInfo.Size() {
		maxReadSize = sourceInfo.Size() - actualOffset
	}

	// 读取数据
	buffer := make([]byte, maxReadSize)
	bytesRead, err := sourceFile.ReadAt(buffer, actualOffset)
	if err != nil && err != io.EOF {
		log.Printf("Error reading from source file: %v", err)
		return nil, syscall.EIO
	}

	return fuse.ReadResultData(buffer[:bytesRead]), 0
}

// Write 处理写请求
func (n *OffsetFileNode) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	// 检查是否为只读模式
	if n.config.ReadOnly {
		log.Printf("Cannot write to file in read-only mode: %s", n.config.VirtualPath)
		return 0, syscall.EACCES
	}

	// 尝试打开源文件进行写入，如果文件不存在则创建
	sourceFile, err := os.OpenFile(n.config.SourcePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Error opening/creating source file for writing: %v", err)
		return 0, syscall.EIO
	}
	defer sourceFile.Close()

	var actualOffset int64

	if n.config.Offset == 0 && n.config.Size == 0 {
		// 直接访问模式 - 允许任意位置写入，包括文件末尾追加
		actualOffset = off
	} else {
		// 偏移访问模式
		actualOffset = n.config.Offset + off

		// 只有在明确设置了大小限制时才检查边界
		// 这样允许在文件末尾追加，但仍然遵守配置的大小限制
		if n.config.Size > 0 && off+int64(len(data)) > n.config.Size {
			// 截断数据以不超过大小限制
			allowedSize := n.config.Size - off
			if allowedSize <= 0 {
				return 0, syscall.ENOSPC
			}
			data = data[:allowedSize]
		}
	}

	// 获取当前文件大小以确定是否需要扩展文件
	fileInfo, err := sourceFile.Stat()
	if err != nil {
		log.Printf("Error getting file info: %v", err)
		return 0, syscall.EIO
	}

	// 如果写入位置超出当前文件大小，需要扩展文件
	// WriteAt 在 Go 中可以自动扩展文件，但为了确保兼容性我们显式处理
	if actualOffset+int64(len(data)) > fileInfo.Size() {
		// 如果需要，扩展文件大小
		if err := sourceFile.Truncate(actualOffset + int64(len(data))); err != nil {
			log.Printf("Error extending file: %v", err)
			// 不返回错误，尝试继续写入，让 WriteAt 处理
		}
	}

	// 写入数据
	bytesWritten, err := sourceFile.WriteAt(data, actualOffset)
	if err != nil {
		log.Printf("Error writing to source file: %v", err)
		return 0, syscall.EIO
	}

	return uint32(bytesWritten), 0
}

// Setattr 处理文件属性设置
func (n *OffsetFileNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	// 如果是只读模式，不允许修改属性
	if n.config.ReadOnly {
		log.Printf("Cannot set attributes on read-only file: %s", n.config.VirtualPath)
		return syscall.EACCES
	}

	// 对于offset文件，我们通常不允许修改大小
	if in.Valid&fuse.FATTR_SIZE != 0 {
		log.Printf("Cannot change size of offset file: %s", n.config.VirtualPath)
		return syscall.EACCES
	}

	// 更新其他属性到源文件（如时间戳）
	if in.Valid&(fuse.FATTR_ATIME|fuse.FATTR_MTIME) != 0 {
		// 这里可以实现时间戳更新逻辑
		now := time.Now()
		if in.Valid&fuse.FATTR_ATIME != 0 {
			out.Atime = uint64(now.Unix())
		}
		if in.Valid&fuse.FATTR_MTIME != 0 {
			out.Mtime = uint64(now.Unix())
		}
		// 更新源文件的时间戳
		if err := os.Chtimes(n.config.SourcePath, time.Unix(int64(out.Atime), 0), time.Unix(int64(out.Mtime), 0)); err != nil {
			log.Printf("Error updating source file times: %v", err)
			return syscall.EIO
		}
	}

	// 返回当前属性
	return n.Getattr(ctx, fh, out)
}

// OffsetFSRoot 是文件系统的根节点
type OffsetFSRoot struct {
	fs.Inode
	configs map[string]*FileConfig
}

var _ = (fs.NodeLookuper)((*OffsetFSRoot)(nil))
var _ = (fs.NodeReaddirer)((*OffsetFSRoot)(nil))

// Lookup 在根目录中查找文件
func (r *OffsetFSRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if config, ok := r.configs[name]; ok {
		offsetFile := &OffsetFileNode{
			config: config,
		}
		return r.NewInode(ctx, offsetFile, fs.StableAttr{Mode: fuse.S_IFREG}), 0
	}
	return nil, syscall.ENOENT
}

// Readdir 列出根目录中的文件
func (r *OffsetFSRoot) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	var entries []fuse.DirEntry
	for name := range r.configs {
		entries = append(entries, fuse.DirEntry{
			Name: name,
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

// 工具函数
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// 验证配置
func validateConfig(config *FileConfig) error {
	// 检查源文件路径是否合理（不检查存在性，因为我们支持创建文件）
	if config.SourcePath == "" {
		return fmt.Errorf("source_path cannot be empty")
	}

	// 检查源文件路径是否可写（尝试创建父目录）
	if !config.ReadOnly {
		parentDir := filepath.Dir(config.SourcePath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("cannot create parent directory for %s: %v", config.SourcePath, err)
		}
	}

	// 检查偏移量是否合理
	if config.Offset < 0 {
		return fmt.Errorf("offset cannot be negative: %d", config.Offset)
	}

	// 检查大小是否合理
	if config.Size < 0 {
		return fmt.Errorf("size cannot be negative: %d", config.Size)
	}

	// 检查虚拟路径
	if config.VirtualPath == "" {
		return fmt.Errorf("virtual_path cannot be empty")
	}

	// 确保虚拟路径不包含路径分隔符（简化实现，只支持根目录下的文件）
	if strings.Contains(config.VirtualPath, "/") || strings.Contains(config.VirtualPath, "\\") {
		return fmt.Errorf("virtual_path cannot contain path separators (currently only root-level files are supported): %s", config.VirtualPath)
	}

	return nil
}

// 从JSONL文件加载配置
func loadConfigs(configPath string) (map[string]*FileConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	configs := make(map[string]*FileConfig)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		var config FileConfig
		if err := json.Unmarshal([]byte(line), &config); err != nil {
			return nil, fmt.Errorf("failed to parse line %d: %v", lineNum, err)
		}

		// 验证配置
		if err := validateConfig(&config); err != nil {
			return nil, fmt.Errorf("invalid config at line %d: %v", lineNum, err)
		}

		// 检查重复的虚拟路径
		if _, exists := configs[config.VirtualPath]; exists {
			return nil, fmt.Errorf("duplicate virtual_path at line %d: %s", lineNum, config.VirtualPath)
		}

		configs[config.VirtualPath] = &config
		log.Printf("Loaded config: %s -> %s (offset=%d, size=%d, readonly=%v)",
			config.VirtualPath, config.SourcePath, config.Offset, config.Size, config.ReadOnly)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid configurations found in %s", configPath)
	}

	return configs, nil
}

func main() {
	var (
		debug      = flag.Bool("debug", false, "enable debug output")
		configFile = flag.String("config", "", "path to JSONL configuration file")
		allowOther = flag.Bool("allow-other", false, "allow other users to access the filesystem")
	)
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage: offsetfs [options] MOUNTPOINT\n" +
			"Options:\n" +
			"  -config string    Path to JSONL configuration file (required)\n" +
			"  -debug           Enable debug output\n" +
			"  -allow-other     Allow other users to access the filesystem")
	}

	if *configFile == "" {
		log.Fatal("Configuration file is required. Use -config flag.")
	}

	mountpoint := flag.Arg(0)

	// 加载配置
	configs, err := loadConfigs(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configurations: %v", err)
	}

	log.Printf("Loaded %d file configurations", len(configs))

	// 创建根文件系统
	root := &OffsetFSRoot{
		configs: configs,
	}

	// 设置挂载选项
	mountOptions := fuse.MountOptions{
		Debug: *debug,
		// 在 macOS 上添加必要的权限选项
		Options: []string{
			"default_permissions", // 使用默认权限检查
			"allow_other",         // 允许其他用户访问
		},
	}

	if *allowOther {
		mountOptions.AllowOther = true
	}

	// 在macOS上添加推荐选项
	mountOptions.FsName = "offsetfs"
	mountOptions.Name = "offsetfs"

	// 启动FUSE服务
	server, err := fs.Mount(mountpoint, root, &fs.Options{
		MountOptions: mountOptions,
	})
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}

	fmt.Printf("OffsetFS mounted on %s\n", mountpoint)
	fmt.Printf("Available files:\n")
	for virtualPath, config := range configs {
		fmt.Printf("  %s -> %s (offset=%d, size=%d, readonly=%v)\n",
			virtualPath, config.SourcePath, config.Offset, config.Size, config.ReadOnly)
	}
	fmt.Printf("Use Ctrl-C to unmount.\n")

	server.Wait()
}
