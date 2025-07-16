package cmd

import (
	"fmt"
	"os"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "syncmate",
	Short: "Src -[S3]-> Dst",
	Long: `SyncMate is a tool for incrementally synchronizing files between a source and destination, 
using S3 as the transfer medium.`,
	Version: "<unknown>",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
	// Disable the default help command
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetCount("verbose")
		if verbose > 0 {
			switch verbose {
			case 1:
				logger.SetLevel(logger.InfoLevel)
			case 2:
				logger.SetLevel(logger.DebugLevel)
			default: // 3 or more
				logger.SetLevel(logger.TraceLevel)
			}
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	// RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cobra-example.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	RootCmd.PersistentFlags().CountP("verbose", "v", "Verbose output (use -v, -vv, or --verbose=N)")
}
