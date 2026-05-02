# Changelog

All notable changes to sir-go are documented here.

---

## [v0.2.0] — 2026-05-02

### Added
- **Full Docker SDK Integration** — migrated all container interactions (logs, exec, list, status) to the official Docker SDK.
- **Redesigned Log & Exec View** — new 70/30 layout with a dedicated log viewport (top) and an interactive execution pane (bottom).
- **Directory Tracking in Exec** — the execution pane now tracks and persists the container's working directory across commands (e.g., `cd` works as expected).
- **Lazy Loading Logs** — scroll up in the log view to automatically fetch and prepend older logs from the container history.

### Removed
- Dependency on the `docker` CLI binary for core operations.
- The external 's' (shell) command in favor of the integrated Exec pane.

---

## [v0.1.0] — 2026-04-28

### Added
- **PostgreSQL autobackup to Cloudflare R2** — new `backup` command performs pg_dump and streams the output directly to an R2 bucket.
- Interactive **backup TUI** (`backup_tui.go`) for configuring and monitoring backup jobs in real time.
- Backup configuration persisted alongside the existing service-inspection config.

---

## [v0.0.4] — 2026-04-28

### Added
- Toggle to show/hide full file-system paths in the service table (`p` key).
- **Configuration management** — settings (theme, column visibility, etc.) are now read from and written to a config file so preferences survive restarts.

### Changed
- Enhanced container info handling to surface more detail in the TUI.

---

## [v0.0.3] — 2026-04-27

### Fixed
- Separated JSON and binary HTTP helpers (`httpGetJSON` / `httpGet`) so release metadata and checksum files are fetched with the correct `Accept` headers, resolving upgrade failures on some platforms.
- Improved checksum verification logic in the upgrade flow.

---

## [v0.0.2] — 2026-04-27

### Fixed
- Hardened `install.sh` checksum verification: the script now correctly extracts the expected hash for the target platform and exits early on mismatch, preventing corrupted installs.

---

## [v0.0.1] — 2026-04-27

### Added
- Initial release.
- Scans Docker Compose files and running containers to report service statuses.
- Table and TUI views for real-time service monitoring.
- Coloured terminal output via style utilities.
- `upgrade` command — fetches the latest GitHub release, verifies the checksum, and replaces the local binary.
- `install.sh` — one-liner installer for Linux/macOS.
- GitHub Actions release workflow for cross-platform binary builds.
