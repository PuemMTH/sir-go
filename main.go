package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z"
var version = "dev"

func main() {
	var (
		watchMode   bool
		intervalSec int
		cfg         ScanConfig
	)

	rootCmd := &cobra.Command{
		Use:   "sir [path]",
		Short: "🐳 SIR - Service Inspector Reporter",
		Long:  "Scan directories for docker-compose files and report service status via Docker SDK.\nIf no path is given, lists ALL Docker Compose containers from the daemon.",
		Args:  cobra.MaximumNArgs(1),
		Example: `  sir                              # all compose containers from Docker
  sir .                            # scan current directory
  sir -t .                         # with image & ports
  sir -d 2 /home/user/projects
  sir -w                           # TUI mode (all containers)
  sir -w .                         # TUI mode (scan path)
  sir -w -t -f --interval 5 .`,
		Run: func(cmd *cobra.Command, args []string) {
			var targetPath string
			if len(args) == 1 {
				var err error
				targetPath, err = filepath.Abs(args[0])
				if err != nil {
					cRed.Printf("  Error: %v\n", err)
					os.Exit(1)
				}
				info, err := os.Stat(targetPath)
				if err != nil || !info.IsDir() {
					cRed.Printf("  Error: '%s' is not a directory\n", args[0])
					os.Exit(1)
				}
			}

			ctx := context.Background()
			cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				cRed.Printf("  Error: cannot connect to Docker: %v\n", err)
				os.Exit(1)
			}
			defer cli.Close()

			if watchMode {
				m := newTUI(ctx, cli, targetPath, cfg, time.Duration(intervalSec)*time.Second)
				p := tea.NewProgram(m, tea.WithAltScreen())
				if _, err := p.Run(); err != nil {
					cRed.Printf("  TUI error: %v\n", err)
					os.Exit(1)
				}
				return
			}

			var res scanResult
			if targetPath == "" {
				res = collectAllContainers(ctx, cli, cfg)
			} else {
				res = collectRows(ctx, cli, targetPath, cfg)
			}
			label := targetPath
			if label == "" {
				label = "(all Docker Compose containers)"
			}
			printOneShot(label, cfg, res)
		},
	}

	rootCmd.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "Print the current version",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				cCyan.Printf("  sir %s\n", version)
			},
		},
		&cobra.Command{
			Use:   "upgrade",
			Short: "Upgrade sir to the latest release",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				if err := runUpgrade(); err != nil {
					cRed.Printf("  Error: %v\n", err)
					os.Exit(1)
				}
			},
		},
	)

	f := rootCmd.Flags()
	f.BoolVarP(&watchMode, "watch", "w", false, "TUI monitor mode with search (auto-refresh)")
	f.IntVarP(&intervalSec, "interval", "i", 2, "refresh interval in seconds (use with -w)")
	f.IntVarP(&cfg.Depth, "depth", "d", 1, "folder scan depth")
	f.BoolVarP(&cfg.FullPath, "full-path", "f", false, "show full path of compose file")
	f.BoolVarP(&cfg.Technical, "technical", "t", false, "show extra columns: image, ports")

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		cRed.Printf("  %v\n", err)
		os.Exit(1)
	}
}
