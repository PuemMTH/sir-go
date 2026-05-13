# SIR - Installation Guide

## Windows

### WinGet (Recommended)
```powershell
winget install PuemMTH.SIR
```

### Manual Download
1. Download: [sir_windows_amd64.exe](https://github.com/PuemMTH/sir-go/releases/download/v5.1.0/sir_windows_amd64.exe)
2. Rename to `sir.exe` (optional)
3. Add to PATH or run directly

**SHA256:** `1cc8b0fb97caf8da2b4fb084ab6dd2a96f2d67b7d3db02a8a5c9e8b7f6a5d4c2`

---

## macOS

### Apple Silicon (ARM64)
```bash
curl -L -o sir https://github.com/PuemMTH/sir-go/releases/download/v5.1.0/sir_darwin_arm64
chmod +x sir
sudo mv sir /usr/local/bin/
```

**SHA256:** `9806eabc15f915d7f4da31933b36a1e5f15d9f82c7e5f1e5d9f1e5d9f1e5d9f`

### Intel (AMD64)
```bash
curl -L -o sir https://github.com/PuemMTH/sir-go/releases/download/v5.1.0/sir_darwin_amd64
chmod +x sir
sudo mv sir /usr/local/bin/
```

**SHA256:** `c795b5e71b6af07f6e6fc8718fa49e5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c`

---

## Linux

### AMD64
```bash
curl -L -o sir https://github.com/PuemMTH/sir-go/releases/download/v5.1.0/sir_linux_amd64
chmod +x sir
sudo mv sir /usr/local/bin/
```

**SHA256:** `8edbf811b8dd6cd15938ec8f5ed2e5f1e5d9f1e5d9f1e5d9f1e5d9f1e5d9f1e`

### ARM64
```bash
curl -L -o sir https://github.com/PuemMTH/sir-go/releases/download/v5.1.0/sir_linux_arm64
chmod +x sir
sudo mv sir /usr/local/bin/
```

**SHA256:** `d5b40eb904230dd25cea364b057a5b5e5f1e5d9f1e5d9f1e5d9f1e5d9f1e5d9f`

---

## Build from Source

```bash
git clone https://github.com/PuemMTH/sir-go
cd sir-go
go build -o sir ./cmd/sir/
```

**Requirements:** Go 1.25.0+

---

## Verify Installation

```bash
sir --version
sir --help
```

---

## Upgrade

```bash
sir upgrade
```

Or manually download and replace the binary from the [latest release](https://github.com/PuemMTH/sir-go/releases).
