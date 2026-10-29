# Changelog

This tracks notable changes to the PHISH & DISCOVER-specific subsystem (scanner engine, dashboard,
deployment) on top of upstream Gophish. It replaces the earlier internal audit/implementation
reports, which had gone stale and contradicted each other.

## Unreleased

### Fixed

- `scanner/` now has unit test coverage: `ToolRegistry`, each `Tool.BuildArgs`/`ExtractTarget`,
  `ScanState` lock lifecycle, and orchestration error/panic paths in `RunScannerTool`/`RunTask`
  (`scanner/scanner_test.go`).
- The per-tool parallel goroutine in `RunTask` (`scanner/engine.go`) now has its own
  `recover()`, matching the other scan goroutines — a panic in one parallel tool run no longer
  crashes the whole process.
- `POST /api/scanner/start` now dispatches the scan synchronously instead of from inside an
  extra goroutine, so lock-contention errors (e.g. "scan already in progress") are returned to
  the caller as `409 Conflict` instead of being silently logged while the client still gets a
  202 "scan queued and starting".
- Module path rebranded to `github.com/ytalharabaci/phish-and-discover`; fixed a `.gitignore` rule that was
  silently excluding `vendor/github.com/gophish/gomail` from version control, and a stale
  `logger` test left over from the slog migration.

### Known open items

- Single concurrent scan per target (lock-based) — no scan queue yet.
- SQLite is single-writer; fine for typical team usage, not for high-concurrency multi-tenant
  deployments.
- GitHub Actions has not run on this fork yet — forks have workflows disabled by default and the
  repository owner needs to enable them once from the Actions tab.

### Resolved since the original April 2026 internal audit

- Panic recovery: scan goroutines (`RunDiscovery`, `RunTask` top-level, `ScannerLogHub`) now
  wrap their work in `defer recover()`.
- Dashboard now renders through Go's `html/template` (`discover_dashboard.html` via `base`
  layout) with a CSRF token, instead of being served as static HTML.
- Interactsh and Cloudlist now have dedicated tool adapters (`scanner/tools.go`).
- Network interface selection has a dedicated `UserNetworkConfig` model and API
  (`models/discover.go`, `controllers/api/network.go`).
