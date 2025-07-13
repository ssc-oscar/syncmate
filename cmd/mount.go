package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	of "github.com/hrz6976/syncmate/offsetfs"
	"github.com/spf13/cobra"
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

		if err := of.ValidateConfig(&config, false); err != nil {
			return nil, fmt.Errorf("invalid config at line %d: %v", lineNum, err)
		}

		if _, exists := configs[config.VirtualPath]; exists {
			return nil, fmt.Errorf("duplicate virtual_path at line %d: %s", lineNum, config.VirtualPath)
		}

		configs[config.VirtualPath] = &config
		log.Printf("Loaded config: %s -> %s (offset=%d, size=%d)",
			config.VirtualPath, config.SourcePath, config.Offset, config.Size)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid configurations found in %s", configPath)
	}

	return configs, nil
}

var mountCmd = &cobra.Command{
	Use:   "mount [mountpoint]",
	Short: "Mount the OffsetFS file system",
	Long:  `Mounts the OffsetFS file system, with offsets and sizes defined in a JSONL configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		mountpoint := args[0]
		configFile := cmd.Flag("config").Value.String()
		debug := cmd.Flag("debug").Value.String() == "true"
		allowOther := cmd.Flag("allow-other").Value.String() == "true"
		readOnly := cmd.Flag("readonly").Value.String() == "true"
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
		// 加载配置
		configs, err := LoadConfigs(configFile)
		if err != nil {
			log.Fatalf("Failed to load configurations: %v", err)
		}
		log.Printf("Loaded %d file configurations", len(configs))
		err = of.MountOffsetFS(of.MountOptions{
			Mountpoint: mountpoint,
			Configs:    configs,
			Debug:      debug,
			AllowOther: allowOther,
			ReadOnly:   readOnly,
		})
		if err != nil {
			log.Fatalf("%v", err)
		}
	},
}

func init() {
	mountCmd.Args = cobra.ExactArgs(1)
	mountCmd.Flags().StringP("config", "c", "", "Path to JSONL configuration file (required)")
	mountCmd.Flags().BoolP("debug", "d", false, "Enable debug output")
	mountCmd.Flags().BoolP("allow-other", "a", false, "Allow other users to access the filesystem")
	mountCmd.Flags().BoolP("readonly", "r", false, "Mount the filesystem in read-only mode")
	RootCmd.AddCommand(mountCmd)
}
