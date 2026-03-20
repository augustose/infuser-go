# CLAUDE.md

## Project Overview

**Infuser Go** is a Go-based IaC reconciliation engine for Gitea/Forgejo servers. It manages users, organizations, teams, and repositories declaratively via YAML files. Supports multiple servers via `servers.yaml`.

## Commands

```bash
# Build
go build -o infuser-go .

# Interactive launcher
go run .

# Dry-run reconciliation
go run ./cmd/reconcile/

# Apply changes
go run ./cmd/reconcile/ --apply

# Apply without confirmation (CI/CD)
go run ./cmd/reconcile/ --apply --auto-approve

# Target specific server
go run ./cmd/reconcile/ --server senetca

# Export current Gitea state to YAML
go run ./cmd/export/

# Generate repo grid report
go run ./cmd/report/
```

No test framework configured yet.

## Architecture

### Reconciliation Flow

1. **Config** — `internal/config/` loads `servers.yaml` (multi-server) or `.env` (single-server fallback)
2. **Parse** — `internal/parser/` reads YAML from `infuser-config/` into a desired state
3. **Memory** — `internal/memory/` loads `.infuser_state*.json` (last-known state)
4. **Diff** — `internal/engine/` computes set differences and builds an action plan
5. **Execute** — `internal/api/` calls the Gitea API via struct-based `GiteaClient` (one per server)
6. **Save** — Updated desired state persisted to state file

### Key Modules

| Module | Role |
|--------|------|
| `internal/config/` | ServerConfig struct, LoadServers(), smart token resolution, .env fallback |
| `internal/api/` | GiteaClient with all CRUD methods + pagination |
| `internal/parser/` | YAML parser for infuser-config/ directory tree |
| `internal/memory/` | JSON state file management |
| `internal/engine/` | Diff logic + action plan + execution |
| `internal/export/` | Export live Gitea state to YAML files |
| `internal/report/` | Repo grid report (CSV + Markdown) |
| `internal/setup/` | Setup wizard + interactive add-server wizard |
| `cmd/reconcile/` | CLI entry point for reconciliation |
| `cmd/export/` | CLI entry point for export |
| `cmd/report/` | CLI entry point for reports |
| `main.go` | Interactive TUI launcher (includes "Add new server" option) |

### Safety Model

- Dry-run is the default — no mutations without `--apply`
- Write operations require `allow_writes: true` in servers.yaml (or `GITEA_ALLOW_WRITES=true`)
- Interactive confirmation before applying (bypass with `--auto-approve`)

## Configuration

### Multi-server (`servers.yaml`)
- `read_token` / `write_token` — direct token values
- `read_token_env` / `write_token_env` — env var names (secure, no secrets in file)
- **Smart resolution:** if `read_token`/`write_token` is an `UPPER_SNAKE_CASE` string (e.g. `MY_TOKEN`), it auto-checks the environment first — real hex tokens are never mistaken for env var names
- Direct values take precedence over env var references

### Single-server (`.env` fallback)
- `GITEA_URL`, `GITEA_READ_TOKEN`, `GITEA_WRITE_TOKEN`, `GITEA_ALLOW_WRITES`

## Dependencies

- `gopkg.in/yaml.v3` — YAML parsing
- `github.com/joho/godotenv` — .env file loading
- Standard library for everything else
