# SIR v5.1.0 Release Notes

## 🎉 What's New

### Features
- Docker Compose scanner with TUI support
- Live monitoring with auto-refresh
- Fuzzy search in TUI mode
- Technical columns: image names and exposed ports
- Configurable scan depth

### Installation
- **Windows (WinGet):** `winget install PuemMTH.SIR`
- **Linux/macOS:** `curl -fsSL https://raw.githubusercontent.com/PuemMTH/sir-go/main/scripts/install.sh | bash`
- **Build from source:** `go build -o sir ./cmd/sir/`

### Commands
- `sir` - List all Docker Compose containers
- `sir .` - Scan current directory
- `sir -w` - TUI monitor mode
- `sir -t` - Show technical details (image, ports)
- `sir upgrade` - Self-upgrade to latest

## 📋 System Requirements
- Go 1.25.0+
- Docker daemon running
- Windows 10+, macOS, or Linux

## 🐛 Bug Fixes & Improvements
- Enhanced TUI responsiveness
- Improved error handling
- Better Docker SDK integration

## 📝 Changelog
See [CHANGELOG.md](https://github.com/PuemMTH/sir-go/blob/main/CHANGELOG.md) for detailed changes.

---

**Download:** [sir.exe](https://github.com/PuemMTH/sir-go/releases/download/v5.1.0/sir.exe)
**SHA256:** `2c634bdc1e358d04c8401edd874df52fe4dba0a7c3b2a92b37d3db2a82ad7a75`
