package offsetfs

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

// FileConfig 定义了 JSONL 配置文件中的每一行
type FileConfig struct {
	VirtualPath string `json:"virtual_path"` // 虚拟文件系统中的文件路径
	SourcePath  string `json:"source_path"`  // 源文件路径
	Offset      int64  `json:"offset"`       // 从源文件的偏移量开始
	Size        int64  `json:"size"`         // 要映射的字节数，0表示到文件末尾
}

// OffsetFS 实现了 cgofuse.FileSystemInterface
type OffsetFS struct {
	fuse.FileSystemBase
	configs  map[string]*FileConfig
	readOnly bool
	mu       sync.RWMutex
}

// NewOffsetFS 创建一个新的 OffsetFS 实例
func NewOffsetFS(configs map[string]*FileConfig, readOnly bool) *OffsetFS {
	return &OffsetFS{
		configs:  configs,
		readOnly: readOnly,
	}
}

// getFileConfig 根据路径获取文件配置
func (fs *OffsetFS) getFileConfig(path string) (*FileConfig, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// 移除前导斜杠
	path = strings.TrimPrefix(path, "/")
	config, exists := fs.configs[path]
	return config, exists
}

// Getattr 获取文件/目录属性
func (fs *OffsetFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	if path == "/" {
		// 根目录
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
		return 0
	}

	config, exists := fs.getFileConfig(path)
	if !exists {
		return -fuse.ENOENT
	}

	// 获取源文件信息
	sourceInfo, err := os.Stat(config.SourcePath)
	if err != nil {
		if os.IsNotExist(err) { // 文件不存在，但我们可能会创建它
			stat.Mode = fuse.S_IFREG | 0644
			if fs.readOnly {
				stat.Mode = fuse.S_IFREG | 0444
			}
			stat.Size = 0
			stat.Nlink = 1
			now := time.Now()
			stat.Mtim = fuse.NewTimespec(now)
			stat.Atim = fuse.NewTimespec(now)
			stat.Ctim = fuse.NewTimespec(now)
			return 0
		}
		log.Printf("Error getting source file info: %v", err)
		return -fuse.EIO
	}

	// 设置文件模式
	if fs.readOnly {
		stat.Mode = fuse.S_IFREG | 0444
	} else {
		stat.Mode = fuse.S_IFREG | 0644
	}

	// 计算实际大小
	if config.Offset == 0 && config.Size == 0 {
		// 直接访问整个文件
		stat.Size = sourceInfo.Size()
	} else if config.Size == 0 {
		// 从offset到文件末尾
		if config.Offset >= sourceInfo.Size() {
			stat.Size = 0
		} else {
			stat.Size = sourceInfo.Size() - config.Offset
		}
	} else {
		// 指定大小
		configSize := config.Size
		if config.Offset == 0 {
			actualSize := sourceInfo.Size()
			if actualSize < configSize {
				stat.Size = actualSize
			} else {
				stat.Size = configSize
			}
		} else {
			availableSize := int64(0)
			if sourceInfo.Size() > config.Offset {
				availableSize = sourceInfo.Size() - config.Offset
			}
			if availableSize < configSize {
				stat.Size = availableSize
			} else {
				stat.Size = configSize
			}
		}
	}

	stat.Nlink = 1
	stat.Mtim = fuse.NewTimespec(sourceInfo.ModTime())
	stat.Atim = fuse.NewTimespec(sourceInfo.ModTime())
	stat.Ctim = fuse.NewTimespec(sourceInfo.ModTime())

	return 0
}

// Readdir 读取目录内容
func (fs *OffsetFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	if path != "/" {
		return -fuse.ENOENT
	}

	// 添加 . 和 .. 目录项
	fill(".", nil, 0)
	fill("..", nil, 0)

	// 添加配置的文件
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	for name := range fs.configs {
		if !fill(name, nil, 0) {
			break
		}
	}

	return 0
}

// Open 打开文件
func (fs *OffsetFS) Open(path string, flags int) (int, uint64) {
	config, exists := fs.getFileConfig(path)
	if !exists {
		return -fuse.ENOENT, ^uint64(0)
	}

	// 检查文件是否存在
	_, err := os.Stat(config.SourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在
			if fs.readOnly {
				return -fuse.ENOENT, ^uint64(0)
			}
			// 在写入模式下，我们稍后创建文件
		} else {
			return -fuse.EIO, ^uint64(0)
		}
	}

	// 检查访问权限
	if fs.readOnly && (flags&fuse.O_WRONLY != 0 || flags&fuse.O_RDWR != 0) {
		return -fuse.EACCES, ^uint64(0)
	}

	return 0, 0 // 返回成功，文件句柄为0
}

// Read 读取文件内容
func (fs *OffsetFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	config, exists := fs.getFileConfig(path)
	if !exists {
		return -fuse.ENOENT
	}

	// 打开源文件
	sourceFile, err := os.Open(config.SourcePath)
	if err != nil {
		log.Printf("Error opening source file for reading: %v", err)
		return -fuse.EIO
	}
	defer sourceFile.Close()

	var actualOffset int64
	var maxReadSize int64

	if config.Offset == 0 && config.Size == 0 {
		// 直接访问模式
		actualOffset = ofst
		maxReadSize = int64(len(buff))
	} else {
		// 偏移访问模式
		actualOffset = config.Offset + ofst

		// 计算可读取的最大字节数
		if config.Size > 0 {
			remaining := config.Size - ofst
			if remaining <= 0 {
				return 0
			}
			maxReadSize = min(int64(len(buff)), remaining)
		} else {
			maxReadSize = int64(len(buff))
		}
	}

	// 检查是否超出文件范围
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		log.Printf("Error getting source file stat: %v", err)
		return -fuse.EIO
	}

	if actualOffset >= sourceInfo.Size() {
		return 0
	}

	// 调整读取大小，不超过文件末尾
	if actualOffset+maxReadSize > sourceInfo.Size() {
		maxReadSize = sourceInfo.Size() - actualOffset
	}

	// 读取数据
	bytesRead, err := sourceFile.ReadAt(buff[:maxReadSize], actualOffset)
	if err != nil && err != io.EOF {
		log.Printf("Error reading from source file: %v", err)
		return -fuse.EIO
	}

	return bytesRead
}

// Write 写入文件内容
func (fs *OffsetFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	config, exists := fs.getFileConfig(path)
	if !exists {
		return -fuse.ENOENT
	}

	// 检查是否为只读模式
	if fs.readOnly {
		log.Printf("Cannot write to file in read-only mode: %s", path)
		return -fuse.EACCES
	}

	// 尝试打开源文件进行写入，如果文件不存在则创建
	sourceFile, err := os.OpenFile(config.SourcePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Error opening/creating source file for writing: %v", err)
		return -fuse.EIO
	}
	defer sourceFile.Close()

	var actualOffset int64
	data := buff

	if config.Offset == 0 && config.Size == 0 {
		// 直接访问模式
		actualOffset = ofst
	} else {
		// 偏移访问模式
		actualOffset = config.Offset + ofst

		// 检查大小限制
		if config.Size > 0 && ofst+int64(len(data)) > config.Size {
			allowedSize := config.Size - ofst
			if allowedSize <= 0 {
				return -fuse.ENOSPC
			}
			data = data[:allowedSize]
		}
	}

	// 获取当前文件大小以确定是否需要扩展文件
	fileInfo, err := sourceFile.Stat()
	if err != nil {
		log.Printf("Error getting file info: %v", err)
		return -fuse.EIO
	}

	// 如果写入位置超出当前文件大小，扩展文件
	if actualOffset+int64(len(data)) > fileInfo.Size() {
		if err := sourceFile.Truncate(actualOffset + int64(len(data))); err != nil {
			log.Printf("Error extending file: %v", err)
		}
	}

	// 写入数据
	bytesWritten, err := sourceFile.WriteAt(data, actualOffset)
	if err != nil {
		log.Printf("Error writing to source file: %v", err)
		return -fuse.EIO
	}

	return bytesWritten
}

// Truncate 截断文件
func (fs *OffsetFS) Truncate(path string, size int64, fh uint64) int {
	_, exists := fs.getFileConfig(path)
	if !exists {
		return -fuse.ENOENT
	}

	if fs.readOnly {
		return -fuse.EACCES
	}

	// 对于offset文件，暂不支持截断操作
	return -fuse.EACCES
}

// Utimens 更新文件时间戳
func (fs *OffsetFS) Utimens(path string, tmsp []fuse.Timespec) int {
	config, exists := fs.getFileConfig(path)
	if !exists {
		return -fuse.ENOENT
	}

	if fs.readOnly {
		return -fuse.EACCES
	}

	if len(tmsp) >= 2 {
		atime := time.Unix(tmsp[0].Sec, tmsp[0].Nsec)
		mtime := time.Unix(tmsp[1].Sec, tmsp[1].Nsec)

		if err := os.Chtimes(config.SourcePath, atime, mtime); err != nil {
			log.Printf("Error updating file times: %v", err)
			return -fuse.EIO
		}
	}

	return 0
}

// 工具函数
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// ValidateConfig 验证配置
func ValidateConfig(config *FileConfig, readOnly bool) error {
	if config.SourcePath == "" {
		return fmt.Errorf("source_path cannot be empty")
	}

	if !readOnly {
		parentDir := filepath.Dir(config.SourcePath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("cannot create parent directory for %s: %v", config.SourcePath, err)
		}
	}

	if config.Offset < 0 {
		return fmt.Errorf("offset cannot be negative: %d", config.Offset)
	}

	if config.Size < 0 {
		return fmt.Errorf("size cannot be negative: %d", config.Size)
	}

	if config.VirtualPath == "" {
		return fmt.Errorf("virtual_path cannot be empty")
	}

	if strings.Contains(config.VirtualPath, "/") || strings.Contains(config.VirtualPath, "\\") {
		return fmt.Errorf("virtual_path cannot contain path separators: %s", config.VirtualPath)
	}

	return nil
}

type MountOptions struct {
	Mountpoint string
	Configs    map[string]*FileConfig
	Debug      bool
	AllowOther bool
	ReadOnly   bool
}

func MountOffsetFS(opt MountOptions) error {
	// 创建文件系统实例
	filesystem := NewOffsetFS(opt.Configs, opt.ReadOnly)

	// 设置挂载选项
	options := []string{
		"-o", "fsname=offsetfs",
	}

	if opt.AllowOther {
		options = append(options, "-o", "allow_other")
	}

	if opt.Debug {
		options = append(options, "-d")
	}

	// 添加挂载点
	fmt.Printf("OffsetFS (CGO) mounted on %s\n", opt.Mountpoint)
	fmt.Printf("Available files:\n")
	for virtualPath, config := range opt.Configs {
		fmt.Printf("  %s -> %s (offset=%d, size=%d)\n",
			virtualPath, config.SourcePath, config.Offset, config.Size)
	}
	fmt.Printf("Use Ctrl-C to unmount.\n")
	if opt.ReadOnly {
		fmt.Printf("Filesystem is mounted in READ-ONLY mode.\n")
	} else {
		fmt.Printf("Filesystem is mounted in READ-WRITE mode.\n")
	}

	// 设置信号处理
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nUnmounting...")
		os.Exit(0)
	}()

	// 挂载文件系统
	host := fuse.NewFileSystemHost(filesystem)
	if !host.Mount(opt.Mountpoint, options) {
		return fmt.Errorf("failed to mount filesystem at %s", opt.Mountpoint)
	}
	return nil
}
