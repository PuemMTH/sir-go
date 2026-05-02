package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"sir/internal/backup"
	"sir/internal/config"
	"sir/internal/docker"
	"sir/internal/styles"
	"sir/internal/tui"
	"sir/internal/types"
	"sir/internal/ui"
	"sir/internal/upgrade"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z"
var version = "v5.1.0"

func main() {
	var (
		watchMode   bool
		intervalSec int
		cfg         types.ScanConfig
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
			conf := config.Load()
			if !cmd.Flags().Changed("depth") && conf.Depth > 0 {
				cfg.Depth = conf.Depth
			}
			if !cmd.Flags().Changed("interval") && conf.Interval > 0 {
				intervalSec = conf.Interval
			}
			if !cmd.Flags().Changed("full-path") && conf.FullPath {
				cfg.FullPath = conf.FullPath
			}
			if !cmd.Flags().Changed("technical") && conf.Technical {
				cfg.Technical = conf.Technical
			}

			var targetPath string
			if len(args) == 1 {
				var err error
				targetPath, err = filepath.Abs(args[0])
				if err != nil {
					styles.CRed.Printf("  Error: %v\n", err)
					os.Exit(1)
				}
				info, err := os.Stat(targetPath)
				if err != nil || !info.IsDir() {
					styles.CRed.Printf("  Error: '%s' is not a directory\n", args[0])
					os.Exit(1)
				}
			} else if conf.DefaultPath != "" {
				expanded := os.ExpandEnv(conf.DefaultPath)
				if home, err := os.UserHomeDir(); err == nil {
					if len(expanded) >= 2 && expanded[:2] == "~/" {
						expanded = filepath.Join(home, expanded[2:])
					}
				}
				if abs, err := filepath.Abs(expanded); err == nil {
					if info, err := os.Stat(abs); err == nil && info.IsDir() {
						targetPath = abs
					}
				}
			}

			ctx := context.Background()
			cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				styles.CRed.Printf("  Error: cannot connect to Docker: %v\n", err)
				os.Exit(1)
			}
			defer cli.Close()

			if watchMode {
				m := tui.New(ctx, cli, targetPath, cfg, time.Duration(intervalSec)*time.Second)
				p := tea.NewProgram(m, tea.WithAltScreen())
				if _, err := p.Run(); err != nil {
					styles.CRed.Printf("  TUI error: %v\n", err)
					os.Exit(1)
				}
				return
			}

			var res types.ScanResult
			if targetPath == "" {
				res = docker.CollectAllContainers(ctx, cli, cfg)
			} else {
				res = docker.CollectRows(ctx, cli, targetPath, cfg)
			}
			label := targetPath
			if label == "" {
				label = "(all Docker Compose containers)"
			}
			ui.PrintOneShot(label, cfg, res)
		},
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage sir configuration",
	}
	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create a sample config file at ~/.config/sir/config.yaml",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := config.Init(); err != nil {
				styles.CRed.Printf("  Error: %v\n", err)
				os.Exit(1)
			}
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			p, err := config.Path()
			if err != nil {
				styles.CRed.Printf("  Error: %v\n", err)
				os.Exit(1)
			}
			styles.CCyan.Printf("  %s\n", p)
		},
	})

	rootCmd.AddCommand(
		backup.NewCmd(),
		&cobra.Command{
			Use:   "version",
			Short: "Print the current version",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				styles.CCyan.Printf("  sir %s\n", version)
			},
		},
		&cobra.Command{
			Use:   "upgrade",
			Short: "Upgrade sir to the latest release",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				if err := upgrade.Run(version); err != nil {
					styles.CRed.Printf("  Error: %v\n", err)
					os.Exit(1)
				}
			},
		},
		func() *cobra.Command {
			var listOnly bool
			c := &cobra.Command{
				Use:   "switch [version]",
				Short: "Switch to a specific release version",
				Long: `Switch to a specific sir release (e.g. v1.2.3).
Run without arguments or with --list to see available versions.`,
				Args: cobra.MaximumNArgs(1),
				Example: `  sir switch --list
  sir switch v1.2.0`,
				Run: func(cmd *cobra.Command, args []string) {
					if listOnly || len(args) == 0 {
						versions, err := upgrade.ListVersions(version)
						if err != nil {
							styles.CRed.Printf("  Error: %v\n", err)
							os.Exit(1)
						}
						styles.CCyan.Println("  Available versions:")
						for _, v := range versions {
							if v == version {
								styles.CGreen.Printf("  → %s (current)\n", v)
							} else {
								styles.CCyan.Printf("    %s\n", v)
							}
						}
						return
					}
					target := args[0]
					if err := upgrade.Switch(version, target); err != nil {
						styles.CRed.Printf("  Error: %v\n", err)
						os.Exit(1)
					}
				},
			}
			c.Flags().BoolVarP(&listOnly, "list", "l", false, "list available versions")
			return c
		}(),
		configCmd,
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
		styles.CRed.Printf("  %v\n", err)
		os.Exit(1)
	}
}
