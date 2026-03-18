# Infuser

```
  ██╗███╗   ██╗███████╗██╗   ██╗███████╗███████╗██████╗
  ██║████╗  ██║██╔════╝██║   ██║██╔════╝██╔════╝██╔══██╗
  ██║██╔██╗ ██║█████╗  ██║   ██║███████╗█████╗  ██████╔╝
  ██║██║╚██╗██║██╔══╝  ██║   ██║╚════██║██╔══╝  ██╔══██╗
  ██║██║ ╚████║██║     ╚██████╔╝███████║███████╗██║  ██║
  ╚═╝╚═╝  ╚═══╝╚═╝      ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝
```

**Infrastructure as Code for Gitea/Forgejo.**

Infuser is a declarative reconciliation engine that manages users, organizations, teams, and repositories on Gitea/Forgejo servers via YAML configuration files. It supports multiple servers and provides a terminal UI for interactive use.

## Features

- **Declarative configuration** — Define your desired state in YAML, Infuser reconciles it with the server
- **Multi-server support** — Manage multiple Gitea/Forgejo instances from a single tool
- **Safe by default** — Dry-run mode shows what would change before touching anything
- **Export** — Snapshot your current server state into YAML files
- **Interactive TUI** — Arrow-key navigation, status indicators, guided setup
- **Reports** — Generate CSV/Markdown grids of repositories, owners, and access

## Quick Start

### Prerequisites

- Go 1.25+
- A Gitea/Forgejo instance with an API token (requires `read:admin` scope)

### Install

```bash
git clone https://github.com/augustose/infuser-go.git
cd infuser-go
go build -o infuser .
```

### Configure

Copy the example config and add your server(s):

```bash
cp servers.yaml.example servers.yaml
```

Edit `servers.yaml`:

```yaml
servers:
  - name: my-gitea
    url: https://gitea.example.com
    read_token_env: GITEA_READ_TOKEN    # env var name (recommended)
    write_token_env: GITEA_WRITE_TOKEN
    allow_writes: true
```

Set your tokens as environment variables (or in a `.env` file):

```bash
export GITEA_READ_TOKEN="your-token-here"
export GITEA_WRITE_TOKEN="your-token-here"
```

### Run

```bash
./infuser
```

The TUI will guide you through the setup. Typical first-time workflow:

1. **Export Gitea state** — pulls current users, orgs, teams, and repos into YAML files
2. **Reconcile (apply)** — initializes the state file from exported YAMLs
3. **Edit YAMLs** — make changes to the desired state
4. **Reconcile (dry-run)** — preview what Infuser would change
5. **Reconcile (apply)** — push changes to the server

## Configuration

### Multi-server (`servers.yaml`)

```yaml
servers:
  - name: production
    url: https://gitea.example.com
    read_token_env: PROD_READ_TOKEN   # env var containing the token
    write_token_env: PROD_WRITE_TOKEN
    allow_writes: true
    config_dir: infuser-config/production        # optional
    state_file: .infuser_state_production.json    # optional

  - name: staging
    url: https://staging.example.com
    read_token: "direct-token-value"   # or set token directly (less secure)
    write_token: "direct-token-value"
    allow_writes: true
```

Token resolution priority: direct value (`read_token`) > environment variable (`read_token_env`).

### Single-server (`.env` fallback)

If no `servers.yaml` exists, Infuser falls back to `.env` variables:

```
GITEA_URL=https://gitea.example.com
GITEA_READ_TOKEN=your-token
GITEA_WRITE_TOKEN=your-token
GITEA_ALLOW_WRITES=true
```

## CLI Usage

```bash
# Interactive TUI
./infuser

# Dry-run reconciliation
go run ./cmd/reconcile/

# Apply changes
go run ./cmd/reconcile/ --apply

# Apply without confirmation (CI/CD)
go run ./cmd/reconcile/ --apply --auto-approve

# Target specific server
go run ./cmd/reconcile/ --server production

# Export current state
go run ./cmd/export/

# Generate repo grid report
go run ./cmd/report/
```

## Safety Model

- **Dry-run is the default** — no mutations without `--apply`
- **Write operations require `allow_writes: true`** in configuration
- **Interactive confirmation** before applying changes (bypass with `--auto-approve`)
- **New users get a random temporary password** with `must_change_password: true`

## YAML Structure

After exporting, Infuser creates this directory structure:

```
infuser-config/<server-name>/
├── users/
│   └── <username>/
│       ├── user.yaml
│       └── repositories/
│           └── <repo>.yaml
└── organizations/
    └── <org-name>/
        ├── org.yaml
        ├── teams/
        │   └── <team>.yaml
        └── repositories/
            └── <repo>.yaml
```

## License

MIT
