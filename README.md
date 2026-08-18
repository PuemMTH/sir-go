# SIR — Service Inspector Reporter

A terminal tool for inspecting Docker Compose service status across your filesystem.

## Features

- Scan directories for `docker-compose*` files and show running/stopped state
- List **all** Docker Compose containers daemon-wide (no path required)
- Live TUI monitor with auto-refresh, scrolling, and fuzzy search
- Optional technical columns: image name, exposed ports
- Configurable scan depth and full-path display
- Self-upgrade via `sir upgrade`

## Installation

### One-liner (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/PuemMTH/sir-go/main/scripts/install.sh | bash
```

Installs to `/usr/local/bin/sir`. Override the directory:

```bash
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/PuemMTH/sir-go/main/scripts/install.sh | bash
```

### Windows (WinGet)

```powershell
winget install PuemMTH.SIR
```

### Build from source

```bash
git clone https://github.com/PuemMTH/sir-go
cd sir-go
go build -o sir ./cmd/sir/
```

## Upgrade

```bash
sir upgrade
```

Fetches the latest release from GitHub, verifies the SHA-256 checksum, and atomically replaces the running binary.

## Usage

```
sir [path] [flags]
sir <command>
```

### Commands

| Command | Description |
|---------|-------------|
| `sir` | List all Docker Compose containers from the daemon |
| `sir .` | Scan current directory |
| `sir /path/to/projects` | Scan a specific directory |
| `sir -w` | TUI watch mode (all containers) |
| `sir -w .` | TUI watch mode (scan path) |
| `sir version` | Print current version |
| `sir upgrade` | Upgrade to the latest release |

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--watch` | `-w` | false | TUI monitor mode with auto-refresh |
| `--interval` | `-i` | 2 | Refresh interval in seconds (TUI mode) |
| `--depth` | `-d` | 1 | Directory scan depth |
| `--full-path` | `-f` | false | Show full path of compose file |
| `--technical` | `-t` | false | Extra columns: Image, Ports |

### Examples

```bash
sir                          # all compose containers from Docker daemon
sir .                        # scan current directory, depth 1
sir -t .                     # with image & port columns
sir -d 2 ~/projects          # scan two levels deep
sir -w                       # TUI: monitor all containers
sir -w .                     # TUI: monitor current directory
sir -w -t -f --interval 5 . # TUI: technical view, full paths, 5s refresh
```

## TUI Keybindings

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll table |
| `/` | Enter search/filter mode |
| `esc` | Clear filter |
| `t` | Toggle technical columns |
| `q` / `Ctrl+C` | Quit |

## Releasing

Push a `v*` tag to trigger the GitHub Actions release workflow:

```bash
git tag v1.0.0
git push origin v1.0.0
```

The workflow builds binaries for all platforms, generates a `checksums.txt`, and publishes a GitHub release automatically.

### Supported platforms

| OS | Arch |
|----|------|
| Linux | amd64, arm64 |
| macOS | amd64 (Intel), arm64 (Apple Silicon) |
| Windows | amd64 |

## Project Structure

```
sir-go/
├── .github/
│   └── workflows/
│       └── release.yml       # multi-arch build & GitHub release
├── cmd/sir/
│   └── main.go               # cobra CLI entrypoint + version var
├── internal/
│   ├── types/types.go        # shared types and constants
│   ├── styles/styles.go      # color and lipgloss style vars
│   ├── docker/
│   │   ├── docker.go         # container index, project matching, uptime
│   │   ├── compose.go        # docker-compose YAML parsing
│   │   └── scan.go           # directory walk and data collection
│   ├── ui/
│   │   ├── table.go          # table rendering and row filtering
│   │   └── print.go          # one-shot terminal output
│   ├── tui/tui.go            # Bubble Tea TUI model
│   ├── config/config.go      # config load/init
│   └── upgrade/upgrade.go    # self-upgrade logic
└── scripts/
    └── install.sh            # curl-based installer
```

## Requirements

- Docker daemon running and accessible via socket

> **Note:** Replace `PuemMTH` in the install script, `upgrade.go`, and this README with your actual GitHub username/org before publishing.
