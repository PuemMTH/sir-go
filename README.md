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
curl -fsSL https://raw.githubusercontent.com/PuemMTH/sir-go/main/install.sh | bash
```

Installs to `/usr/local/bin/sir`. Override the directory:

```bash
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/PuemMTH/sir-go/main/install.sh | bash
```

### Build from source

```bash
git clone https://github.com/PuemMTH/sir-go
cd sir-go
go build -o sir .
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

## Autobackup

Backup PostgreSQL databases from a Docker container to Cloudflare R2.

### Setup credentials

```bash
sir autobackup config set \
  --account-id <cloudflare-account-id> \
  --access-key <r2-access-key-id> \
  --secret-key <r2-secret-access-key> \
  --bucket <bucket-name>
```

Credentials are saved to `~/.sir/settings.json` (mode 0600).

```bash
sir autobackup config show   # print current config
```

### Run a backup

```bash
sir autobackup run --container <container-name> --db <database> [--user postgres]
```

Connects to the container via Docker exec, runs `pg_dump`, gzips the output, and uploads to R2 at:

```
backups/<database>/<database>-<timestamp>.sql.gz
```

### Schedule automatic backups

```bash
# Install a cron job
sir autobackup cron set \
  --schedule "0 2 * * *" \
  --container <container-name> \
  --db <database> \
  [--user postgres]

sir autobackup cron status   # show active cron job
sir autobackup cron remove   # remove cron job
```

The cron entry runs `sir autobackup run` on the given schedule using the system crontab.

### Autobackup commands

| Command | Description |
|---------|-------------|
| `sir autobackup config set` | Save R2 credentials to `~/.sir/settings.json` |
| `sir autobackup config show` | Print current R2 configuration |
| `sir autobackup run` | Run a backup immediately |
| `sir autobackup cron set` | Install a cron job for automatic backups |
| `sir autobackup cron status` | Show the active autobackup cron job |
| `sir autobackup cron remove` | Remove the autobackup cron job |

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
│       └── release.yml  # multi-arch build & GitHub release
├── main.go              # cobra CLI entrypoint + version var
├── types.go             # shared types and constants
├── styles.go            # color and lipgloss style vars
├── docker.go            # Docker SDK: container index, project matching, uptime
├── compose.go           # docker-compose YAML parsing
├── scan.go              # directory walk and data collection
├── table.go             # table rendering and row filtering
├── tui.go               # Bubble Tea TUI model
├── print.go             # one-shot terminal output
├── upgrade.go           # self-upgrade logic
├── backup.go            # autobackup: pg_dump → gzip → Cloudflare R2
└── install.sh           # curl-based installer
```

## Requirements

- Docker daemon running and accessible via socket

> **Note:** Replace `PuemMTH` in the install script, `upgrade.go`, and this README with your actual GitHub username/org before publishing.
