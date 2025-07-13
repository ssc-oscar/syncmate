package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	of "github.com/hrz6976/syncmate/offsetfs"
	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
)

// LoadConfigs 从JSONL文件加载配置
func LoadConfigs(configPath string) (map[string]*of.FileConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	configs := make(map[string]*of.FileConfig)

	// 读取整个文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	lineNum := 0

	for _, line := range lines {
		lineNum++
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		var config of.FileConfig
		if err := json.Unmarshal([]byte(line), &config); err != nil {
			return nil, fmt.Errorf("failed to parse line %d: %v", lineNum, err)
		}

		if err := of.ValidateConfig(&config); err != nil {
			return nil, fmt.Errorf("invalid config at line %d: %v", lineNum, err)
		}

		if _, exists := configs[config.VirtualPath]; exists {
			return nil, fmt.Errorf("duplicate virtual_path at line %d: %s", lineNum, config.VirtualPath)
		}

		configs[config.VirtualPath] = &config
		log.Printf("Loaded config: %s -> %s (offset=%d, size=%d, readonly=%v)",
			config.VirtualPath, config.SourcePath, config.Offset, config.Size, config.ReadOnly)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid configurations found in %s", configPath)
	}

	return configs, nil
}

func MountCGO(mountpoint string, configFile *string, debug bool, allowOther bool) error {
	// 加载配置
	configs, err := LoadConfigs(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configurations: %v", err)
	}

	log.Printf("Loaded %d file configurations", len(configs))

	// 创建文件系统实例
	filesystem := of.NewOffsetFS(configs)

	// 设置挂载选项
	options := []string{
		"-o", "fsname=offsetfs",
		"-o", "volname=OffsetFS",
	}

	if allowOther {
		options = append(options, "-o", "allow_other")
	}

	if debug {
		options = append(options, "-d")
	}

	// 添加挂载点
	fmt.Printf("OffsetFS (CGO) mounted on %s\n", mountpoint)
	fmt.Printf("Available files:\n")
	for virtualPath, config := range configs {
		fmt.Printf("  %s -> %s (offset=%d, size=%d, readonly=%v)\n",
			virtualPath, config.SourcePath, config.Offset, config.Size, config.ReadOnly)
	}
	fmt.Printf("Use Ctrl-C to unmount.\n")

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
	if host.Mount(mountpoint, options) != true {
		return fmt.Errorf("failed to mount filesystem at %s", mountpoint)
	}
	return nil
}

var mountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mount the OffsetFS file system",
	Long:  `Mounts the OffsetFS file system, with offsets and sizes defined in a JSONL configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			log.Fatal("Usage: syncmate mount [options] MOUNTPOINT\n" +
				"Options:\n" +
				"  -config string    Path to JSONL configuration file (required)\n" +
				"  -debug           Enable debug output\n" +
				"  -allow-other     Allow other users to access the filesystem")
		}
		mountpoint := args[0]
		configFile := cmd.Flag("config").Value.String()
		debug := cmd.Flag("debug").Value.String() == "true"
		allowOther := cmd.Flag("allow-other").Value.String() == "true"
		if configFile == "" {
			log.Fatal("Configuration file is required. Use -config flag.")
		}
		if mountpoint == "" {
			log.Fatal("Mount point is required.")
		}
		if debug {
			log.SetFlags(log.LstdFlags | log.Lshortfile)
		} else {
			log.SetFlags(log.LstdFlags)
		}
		err := MountCGO(mountpoint, &configFile, debug, allowOther)
		if err != nil {
			log.Fatalf("%v", err)
		}
	},
}

func init() {
	mountCmd.Flags().StringP("config", "c", "", "Path to JSONL configuration file (required)")
	mountCmd.Flags().BoolP("debug", "d", false, "Enable debug output")
	mountCmd.Flags().BoolP("allow-other", "a", false, "Allow other users to access the filesystem")
	RootCmd.AddCommand(mountCmd)
}
