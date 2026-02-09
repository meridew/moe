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

## Build & Run

### Go Setup

Go is installed at `C:\Program Files\Go\bin` and is already on PATH (verified via `go version`). No PATH manipulation needed in new terminal sessions.

### Core Commands

| Action | Command |
|--------|---------|
| **Build** (compile check) | `go build -o moe.exe ./cmd/moe` |
| **Run** (foreground) | `.\moe.exe` |
| **Stop** | `Get-Process moe -ErrorAction SilentlyContinue \| Stop-Process -Force` |
| **Run tests** | `go test ./...` |
| **Reset DB** | Delete `moe.db` and restart — **only when migrations change** (see re-seed steps below) |
| **Add dependency** | `go get <module>` then `go mod tidy` |

### VS Code Tasks (`.vscode/tasks.json`)

These tasks have Go PATH baked in via the `options.env` block:

| Task | Shortcut | What it does |
|------|----------|--------------|
| **Build MOE** | `Ctrl+Shift+B` | Compile check only |
| **Run MOE** | Run Task menu | Build → kill existing → run foreground |
| **Stop MOE** | Run Task menu | Kill any running moe.exe |
| **Test MOE** | Run Task menu | `go test ./...` |
| **Reset DB** | Run Task menu | Stop server + delete moe.db — **must re-seed after** |

## Copilot Workflow

### After Editing Go Files

1. **Build check**: Run `go build -o moe.exe ./cmd/moe` in terminal to verify compilation. Fix errors before proceeding.
2. **If runtime testing needed**: Start the server (see below), verify, then stop.

### Starting the Server

Run `.\moe.exe` as a **background terminal** (`isBackground: true`). This gives you the terminal ID to check logs later. Always kill existing instances first:

```
Get-Process moe -ErrorAction SilentlyContinue | Stop-Process -Force; Start-Sleep -Milliseconds 300; .\moe.exe
```

### Stopping the Server

Either:
- `kill_terminal` on the background terminal running moe.exe
- `Get-Process moe -ErrorAction SilentlyContinue | Stop-Process -Force` in any terminal

### Debugging & Troubleshooting

Follow this priority order — each layer narrows the problem faster:

#### 1. API-first data verification (fastest, most reliable)
- Use `fetch_webpage` against **API endpoints** (`/api/v1/*`) — returns clean JSON.
- Example: `fetch_webpage` on `http://localhost:8080/api/v1/policies/snapshots` to check data.
- Chain calls: list → get by ID → get items.
- If API data is correct but UI is wrong → bug is in templates.
- If API data is wrong → bug is in handlers/store/provider.

#### 2. HTML verification (when checking rendered templates)
- Use `run_in_terminal` with `curl -s "http://localhost:8080/<path>" | Select-String "pattern"` to grep for specific text.
- **Never** use `fetch_webpage` for HTML pages — it strips structure and produces misleading results.

#### 3. Visual inspection (when layout/styling matters)
- Use `open_simple_browser` to open `http://localhost:8080/<path>` — renders actual page in VS Code.

#### 4. Server logs
- Use `get_terminal_output` on the background terminal running moe.exe to check request logs and errors.

#### 5. Database inspection
- `run_in_terminal` with sqlite3 commands, or query via the API layer.

### After DB Reset — Re-Seed Test Data

**Only delete `moe.db` when a schema migration changed.** For normal code changes, just restart the server — the existing DB is fine.

When you DO reset the DB, **always re-seed test data** before handing back to the user. Don't leave a blank DB.

#### Step 1 — Re-create the Intune provider (form POST)

```powershell
curl -s -X POST http://localhost:8080/providers/new `
  -d "name=intune-meridew" `
  -d "type=intune" `
  -d "tenant_id=122521bd-12ea-4515-acc6-cf8d44a8dae7" `
  -d "client_id=CLIENT_ID_HERE" `
  -d "client_secret=CLIENT_SECRET_HERE" `
  -d "sync_interval=30m" `
  -d "enabled=on" `
  -o /dev/null -w "%{http_code}"
```

**Problem**: client_id and client_secret are secrets not stored in this repo. If you need to create the provider, ask the user to either:
- Add it via the UI at http://localhost:8080/providers/new, or
- Provide the credentials.

#### Step 2 — Capture baselines

Once the provider exists, capture a baseline via API:
```powershell
# Get provider ID first
$providers = curl -s http://localhost:8080/api/v1/providers | ConvertFrom-Json
$pid = $providers.data[0].id

# Capture a baseline (this calls out to Intune — takes ~30s)
curl -s -X POST http://localhost:8080/api/v1/policies/snapshots `
  -H "Content-Type: application/json" `
  -d "{`"provider_id`":`"$pid`",`"label`":`"Baseline 1`"}"
```

For compare testing, capture a second baseline with a different label.

#### Alternative — Import test fixtures (no provider needed)

If you have a previously exported snapshot JSON file, import it:
```powershell
curl -s -X POST http://localhost:8080/api/v1/policies/snapshots/import `
  -H "Content-Type: application/json" `
  -d (Get-Content .\test-snapshot.json -Raw)
```

This skips the provider entirely — useful for pure UI testing.

#### Recommended workflow

1. **If only changing templates/CSS/JS** — don't reset DB at all, just restart the server.
2. **If migration changed** — reset DB, ask user to re-add provider via UI, then capture baselines via API.
3. **If testing UI without a provider** — use the import API with test fixture data.

### Key Principles
- **Build before test** — always compile-check after edits, before starting the server.
- **API before UI** — verify data layer first, then templates.
- **One server instance** — always kill before starting a new one.
- **Background terminals for servers** — so you can check logs with `get_terminal_output`.
- **Clean up** — stop the server when done testing.
- **Don't reset DB unnecessarily** — only when migrations change the schema. Restarting the server is enough for code-only changes.
- **Re-seed after reset** — never leave the user with an empty DB after deleting moe.db.

### File References
- Tasks config: `.vscode/tasks.json`
- DB lives at: `./moe.db` (auto-created on first run)
- Binary output: `./moe.exe`


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
