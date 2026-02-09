# MOE — Copilot Instructions

## Project Overview

MOE (Mobile Operations Engine) is a single-binary Go web application with an embedded SQLite database. It provides an abstraction layer over BlackBerry UEM and Microsoft Intune to run sequential device command "campaigns." No external runtime dependencies — everything embeds into one binary.

## Tech Stack

- **Go** (stdlib `net/http`, `html/template`, `database/sql`, `embed`)
- **SQLite** via `modernc.org/sqlite` (pure Go, no CGo)
- **htmx 2.0.7** + **Alpine.js 3.15.8** (embedded JS, no build step)
- Single binary, dark-themed admin UI

## Project Structure

```
cmd/moe/main.go              # Entrypoint: flags, DB init, server start, graceful shutdown
internal/
  db/db.go                    # SQLite connection (WAL, FK, single conn)
  db/migrate.go               # Embedded migration runner
  db/migrations/*.sql          # Numbered migrations (applied in order)
  models/models.go            # Domain structs (Device, ProviderConfig, etc.)
  store/*.go                  # Data access layer (one file per entity)
  server/server.go            # Server struct, mux, dependencies
  server/routes.go            # All route registrations
  server/render.go            # Template renderer (per-page clone)
  server/*.go                 # Handlers grouped by feature
  provider/provider.go        # Provider interface + types
  provider/registry.go        # Thread-safe provider registry
  provider/intune/*.go        # Intune adapter (Graph API)
  provider/uem/*.go           # UEM adapter (REST API) — future
web/
  embed.go                    # //go:embed directives for templates + static
  templates/*.html            # Go html/template files (layout + pages)
  static/css/style.css        # Dark theme design system
  static/js/app.js            # Shared JS helpers
  static/js/htmx.min.js       # htmx library (embedded)
  static/js/alpine.min.js     # Alpine.js library (embedded)
```

## Terminal & Build Rules

**CRITICAL**: Go is installed at `C:\Program Files\Go\bin` and may not be on PATH in new terminal sessions.

### Always use this pattern for ANY terminal command:

```powershell
$env:PATH = "C:\Program Files\Go\bin;" + $env:PATH; cd c:\Users\dan\source\repos\moe; <command>
```

### Standard commands:

| Action | Command |
|--------|---------|
| Build check | `go build ./...` |
| Run server | `go run ./cmd/moe` |
| Run tests | `go test ./...` |
| Add dependency | `go get <module>` then `go mod tidy` |

### Starting the web server for testing:

1. **Kill any existing server first** (check for running background terminals).
2. Use `isBackground: true` — the server blocks.
3. Wait ~3 seconds, then check output to confirm `server listening on :8080`.
4. Use `open_simple_browser` to verify pages render.
5. **Never** start multiple servers — port 8080 will conflict.

### After editing Go files:

1. Run `go build ./...` (non-background, with timeout) to check compilation.
2. Only start the server if the user asks to test or you need to verify rendering.

## Code Conventions

### Go

- Handlers: grouped by feature in `internal/server/<feature>.go`
- Each handler file has a template data struct at the top
- New routes go in `routes.go`
- ID generation: `newID()` helper (crypto/rand hex)
- Stores take `*sql.DB`, return typed results — no interface unless needed
- Errors bubble up with `fmt.Errorf("context: %w", err)`

### SQL Migrations

- Files in `internal/db/migrations/` — format: `NNN_description.sql`
- Applied in filesystem order, tracked in `_migrations` table
- **Never modify an existing migration** — always add a new one
- Test migration by deleting `moe.db` and restarting (it auto-creates)

### Templates

- Layout: `web/templates/layout.html` (defines `{{template "title" .}}` and `{{template "content" .}}`)
- Pages: each defines `{{define "title"}}` and `{{define "content"}}` blocks
- Renderer clones layout per page (avoids block name collisions)
- Template functions registered in `render.go` funcMap

### Frontend

- **htmx**: Use for server-driven interactivity (partial swaps, polling, inline forms). Attributes go on HTML elements — no JS needed.
- **Alpine.js**: Use for client-side-only reactivity (show/hide, dropdowns, form logic). Use `x-data`, `x-show`, `x-bind` directives.
- **CSS**: All styles in `style.css`. Use existing CSS variables (`--bg`, `--fg`, `--border`, `--color-primary`, etc.).
- **No npm, no build step** — everything is a static file embedded in the binary.

## Provider Architecture

- `Provider` interface: `SyncDevices(ctx, cursor)`, `SendCommand(ctx, deviceID, cmd)`, `CheckCommandStatus(ctx, cmdID)`
- Each provider type has its own package under `internal/provider/`
- Provider configs stored in DB — adapters instantiated at sync/command time via `buildProvider()`
- **Intune**: OAuth2 client credentials → Microsoft Graph API
- **UEM**: Username/password → auth header via `/util/authorization` → REST API (tenant URL includes SRP ID)

## Multi-Tenant Model

- Multiple providers of the same type (e.g., 3 Intune tenants, 2 UEM tenants)
- Each provider config has a unique `name` used as the foreign key in device records
- MOE is a **cache**, not source of truth — devices sync from providers

## Database

- SQLite in WAL mode, foreign keys ON
- Single connection (SQLite concurrency model)
- Tables: `settings`, `provider_configs`, `devices`, `_migrations`
- Future: `campaigns`, `campaign_steps`, `audit_log`
