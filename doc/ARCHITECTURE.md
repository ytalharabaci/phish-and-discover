# PHISH & DISCOVER Architecture

This document describes the current implementation of PHISH & DISCOVER's scanner subsystem — the part
of the codebase that goes beyond stock Gophish. For Gophish's own architecture (campaigns,
templates, results), see the [Gophish documentation](https://docs.getgophish.com/).

## Stack

- **Backend**: Go, built on top of Gophish's HTTP server, middleware, and GORM models.
- **Scanner engine**: `scanner/` package — wraps ProjectDiscovery CLI tools via `os/exec`.
- **Frontend**: Server-rendered `html/template` (`templates/discover_dashboard.html`) with
  Tailwind CSS and vanilla JS; CSRF-protected via Gophish's existing `gorilla/csrf` middleware.
- **Database**: SQLite (same database file as Gophish; `Scan`/`Finding` tables added via
  GORM `AutoMigrate`, foreign keys to Gophish's existing tables where relevant).
- **Deployment**: Docker Compose — `phish-core` (app + PD tools), `postfix` (SMTP relay),
  `caddy` (TLS termination, reverse proxy, admin path obfuscation).

## Scanner package design

`scanner/scanner_interface.go` defines the seams the rest of the package is built around:

- **`Tool`** — one implementation per ProjectDiscovery binary (`tools.go`). Adding a new tool
  means adding a new `Tool` implementation and registering it; the engine never special-cases
  a tool by name (Open/Closed).
- **`ToolRegistry`** — thread-safe lookup of `Tool` by name. `DefaultRegistry` registers all
  shipped tools at package init.
- **`ScanService`** — orchestration entry points (`RunScannerTool`, `RunDiscovery`, `RunTask`).
  `PHISH & DISCOVERScanService` is the concrete implementation (`engine.go`); callers depend on the
  interface, not the struct (Dependency Inversion).
- **`ToolExecutor`** — low-level process execution + result collection, separate from
  orchestration logic.
- **`ResultPersister`** — separates "how a finding is parsed/stored" from "how a scan is run".

Each scan runs in its own goroutine, started from `RunDiscovery`/`RunTask`/`RunScannerTool` and
guarded by a `defer recover()` so a single tool failure doesn't take down the admin server.
`RegisterScan`/`UnregisterScan` track per-scan `context.CancelFunc`s so scans can be stopped
from the API. WebSocket log streaming (`ScannerLogHub` in `engine.go`) fans scan output out to
connected dashboard clients.

`scanner/timeout_handler.go` enforces per-tool timeouts (e.g. nuclei: 30m) so a hung CLI process
can't leak a goroutine indefinitely. `scanner/json_streaming.go` parses tool output line-by-line
instead of buffering it all in memory, to avoid OOM on large nuclei/httpx runs.

## Known limitations

- Single SQLite writer — fine for one team's campaigns/scans, not for high-concurrency
  multi-tenant use. A Postgres backend would be the natural next step if that's ever needed.
- One scan lock per scan type/target (`ScanState.AcquireLock`) — concurrent scans on the same
  target queue rather than running in parallel.
- No automated test coverage for the `scanner/` package yet (tracked in
  [CHANGELOG.md](CHANGELOG.md)).

## Where things live

| Concern | Path |
|---|---|
| Scan/Finding DB models | `models/discover.go` |
| Scanner orchestration | `scanner/engine.go` |
| Tool adapters | `scanner/tools.go` |
| Tool/Service interfaces | `scanner/scanner_interface.go` |
| Timeout enforcement | `scanner/timeout_handler.go` |
| Streaming JSON parsing | `scanner/json_streaming.go` |
| Structured logging / agent health | `scanner/error_handler.go` |
| REST API handlers | `controllers/api/scanner.go`, `controllers/api/network.go` |
| WebSocket log hub | `scanner/engine.go` (`ScannerLogHub`), mounted in `controllers/route.go` |
| Dashboard template | `templates/discover_dashboard.html` |
| Deployment | `Dockerfile`, `docker-compose.yml`, `Caddyfile` |
