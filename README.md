# Infuser Go

Infrastructure as Code reconciliation engine for Gitea/Forgejo servers. Manages users, organizations, teams, and repositories declaratively via YAML files.

Built-in multi-server support — manage multiple Gitea instances from a single configuration.

## Install

```bash
go build -o infuser-go .
```

## Usage

### Interactive Launcher

```bash
./infuser-go
# or: go run .
```

### CLI Commands

```bash
# Dry-run reconciliation (default safe mode)
go run ./cmd/reconcile/

# Apply changes (interactive confirmation)
go run ./cmd/reconcile/ --apply

# Apply without confirmation (CI/CD)
go run ./cmd/reconcile/ --apply --auto-approve

# Target a specific server
go run ./cmd/reconcile/ --server production

# Export current Gitea state to YAML
go run ./cmd/export/

# Generate repo grid report
go run ./cmd/report/
```

## Configuration

### Single Server (`.env`)

Copy `.env.example` to `.env` and fill in your values:

```
GITEA_URL="https://gitea.example.com"
GITEA_READ_TOKEN="your_read_token"
GITEA_WRITE_TOKEN="your_write_token"
GITEA_ALLOW_WRITES="false"
```

### Multi-Server (`servers.yaml`)

Copy `servers.yaml.example` to `servers.yaml`. Token values are read from environment variables referenced in the YAML (no secrets in the file).

Set the env vars in your `.env` or shell:

```
PROD_GITEA_READ_TOKEN="..."
PROD_GITEA_WRITE_TOKEN="..."
STAGING_GITEA_READ_TOKEN="..."
STAGING_GITEA_WRITE_TOKEN="..."
```

## YAML Config Structure

The `infuser-config/` directory contains:

- `users/{name}/user.yaml` — User definitions
- `users/{name}/repositories/{repo}.yaml` — Personal repositories
- `organizations/{name}/org.yaml` — Organization definitions
- `organizations/{name}/teams/{team}.yaml` — Team definitions with members
- `organizations/{name}/repositories/{repo}.yaml` — Org repositories

## Safety Model

- Dry-run is the default — no mutations without `--apply`
- Write operations require `allow_writes: true` (or `GITEA_ALLOW_WRITES=true`)
- Interactive confirmation before applying (bypass with `--auto-approve`)
