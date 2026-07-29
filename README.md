# Orca

Orca is an in-development, self-hosted Postgres orchestration and control platform. An agent on infrastructure you control reconciles Docker resources from a locally cached desired-state snapshot. The control plane stores desired state and reported health, not user database data, and never initiates a connection to the agent host.

## Current Status

Implemented in the backend and agent:

- Full desired-state snapshots over an agent-initiated WebSocket, including a fresh full snapshot after reconnecting.
- Local desired-state caching and periodic reconciliation while the control plane is unavailable.
- Postgres primary lifecycle, real streaming-replica provisioning, PgBouncer, pgBackRest schedules, and a limited set of Postgres extensions.
- Persisted agent reports, reconciliation results, Prometheus-compatible metrics, and server-side alert-rule evaluation.
- Authenticated project and cluster CRUD plus host registration through the REST API.
- Email/password authentication, optional GitHub and Google OAuth through Goth, 24-hour HS256 JWTs, JWT middleware, and JWT-authenticated project event WebSockets.
- Agent tunnel authentication using the token issued during host registration.

The web application is an authenticated, read-only viewer with email/password signup and login plus optional GitHub/Google OAuth. It lists existing projects, renders desired primaries, replicas, and PgBouncer resources at fixed positions, and combines them with the latest persisted agent observations. It receives full project-status snapshots over a separate browser WebSocket. It does not provide host management, resource mutation, backup or extension controls, PITR, alert management, or topology-position persistence.

Point-in-time recovery exists as `pgbackrest.RestoreToTime`, but nothing in the REST API, tunnel protocol, CLI, development RPC, or UI invokes it.

## Architecture

1. The agent authenticates with a host token and opens an outbound WebSocket. No inbound agent port is required.
2. The server sends the complete current desired state for that host.
3. The agent saves the snapshot locally, observes Docker, computes create/update/delete actions, and applies them.
4. Independent actions continue after a failure, while actions with a known failed dependency are skipped and reported as such.
5. The agent reports post-apply observed state, health, and reconciliation results.

After a disconnect, the agent keeps reconciling from its local cache. On reconnect, the server sends current desired state rather than replaying missed changes.

Active agent sessions, frontend subscriptions, desired-state push routing, and alert debounce timers are process-local. Run one server instance; horizontal scaling is not coordination-free in the current implementation.

## Local Development

### Prerequisites

- Docker with the Compose plugin.
- Node.js and npm for the web application.
- Go 1.25 or newer when running the server or agent outside Compose.
- `psql` when running migrations outside Compose.

The development Compose stack runs Postgres, the Go API, and a Caddy same-origin proxy. Vite and the agent run from source on the host.

### Database And Environment

1. Create the local environment file and replace `ORCA_JWT_SECRET` with a random value.

```sh
cp .env.example .env
openssl rand -hex 32
```

2. Install the web dependencies.

```sh
make setup
```

3. Start Postgres, apply migrations, build the API, and run the API plus proxy.

```sh
make dev
```

Use `make dev-detach` instead to leave the backend stack running in the background. In another terminal, start Vite:

```sh
make dev-web
```

Open `http://localhost:3000`, not Vite's direct `5173` address. Use `make stop` to stop the backend stack. `make dev-migrate` can be rerun independently and is idempotent.

### Create A User And Host

Register with email/password to obtain a user JWT:

```sh
curl -sS http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"developer@example.test","password":"choose-a-local-password"}'
```

Use the returned `token` as `USER_JWT`, then register a host:

```sh
curl -sS http://localhost:8080/hosts \
  -X POST \
  -H "Authorization: Bearer $USER_JWT"
```

`POST /hosts` returns the agent token only inside `docker_run_command`; copy its `ORCA_TOKEN` value into the agent environment. A new agent connection is accepted only during the token's 24-hour lifetime; an already-established connection is not reauthenticated when that lifetime elapses.

The generated command currently names `orca/agent`, but this checkout has no agent Dockerfile or configured image source. For repository development, run the agent from source on the Docker host:

```sh
export ORCA_TOKEN='token-from-the-generated-command'
export ORCA_SERVER_URL='ws://localhost:3000/agent'
export ORCA_STATE_PATH='/var/orca/state/desired.json'
go run ./agent/cmd/agent
```

Ensure the parent directory of `ORCA_STATE_PATH` is writable and persistent if desired state must survive agent process or host restarts. If `ORCA_SERVER_URL` is unset, the agent instead starts its standalone development endpoint at `ORCA_DEV_ADDRESS` (default `127.0.0.1:8080`); choose another port if the server is also using 8080.

### OAuth Apps

GitHub and Google OAuth are optional; email/password auth works without them. A provider is enabled only when both its client ID and secret are set, and startup rejects a half-configured pair.

For GitHub, create an OAuth App, set its application URL to `http://localhost:3000`, and register `http://localhost:3000/auth/github/callback` as the authorization callback URL.

For Google, configure the consent screen, create a Web application OAuth client, and register `http://localhost:3000/auth/google/callback` as an authorized redirect URI.

Set the matching `ORCA_GITHUB_*` or `ORCA_GOOGLE_*` variables before starting the server. The login page begins authentication at `GET /auth/github` or `GET /auth/google`. A successful callback establishes the browser session cookie and redirects to the project list. Linking a provider to an already-authenticated account is still deferred.

OAuth callback origins use the scheme and authority of `ORCA_SERVER_URL`, translating `ws` to `http` and `wss` to `https` and discarding the entire path, query, and fragment. The browser-facing API and agent endpoint must therefore share an externally reachable origin in the current configuration.

### Web Application

```sh
make dev-web
```

The checked-in Caddy development proxy routes `/auth/*`, JSON API requests, agent traffic, and project WebSockets to the Go server while routing document requests to Vite. Browser authentication uses an httpOnly `orca.session` cookie; the API's bearer-token response remains available for non-browser clients. Production deployments need equivalent same-origin routing.

## Environment Variables

| Variable | Used by | Current behavior |
|---|---|---|
| `DATABASE_URL` | server | Required metadata Postgres connection string |
| `ORCA_JWT_SECRET` | server | Required JWT signing and OAuth cookie-store secret |
| `ORCA_PORT` | server | HTTP port; defaults to `8080` |
| `ORCA_LOG_LEVEL` | server | `debug`, `info`, `warn`, or `error`; defaults to `info` |
| `ORCA_SERVER_URL` | agent, server | Agent tunnel URL; enables agent tunnel mode, appears in host commands, and supplies the server's OAuth callback origin |
| `ORCA_GITHUB_CLIENT_ID` / `ORCA_GITHUB_CLIENT_SECRET` | server | Optional GitHub OAuth pair |
| `ORCA_GOOGLE_CLIENT_ID` / `ORCA_GOOGLE_CLIENT_SECRET` | server | Optional Google OAuth pair |
| `ORCA_API_URL` | web server | Go API origin used by TanStack Start server functions; defaults to `http://127.0.0.1:8080` |
| `ORCA_TOKEN` | agent | Required in tunnel mode |
| `ORCA_DATA_DIR` | agent | Disk-metrics path; defaults to `/var/orca/data` and does not relocate Docker/config storage |
| `ORCA_STATE_PATH` | agent | Desired-state cache path; defaults to `/var/orca/state/desired.json` |
| `ORCA_DEV_ADDRESS` | agent | Standalone development endpoint address when `ORCA_SERVER_URL` is unset; defaults to `127.0.0.1:8080` |

The Docker SDK also honors its standard `DOCKER_*` environment variables through `client.FromEnv`; those are Docker configuration rather than Orca-specific settings.

## Project Structure

```text
orca/
├── agent/    # Docker reconciliation, backup scheduling, and outbound tunnel
├── server/   # REST API, WebSockets, desired-state store, auth, and metrics
├── web/      # read-only canvas and status UI
├── pkg/      # shared Go types
├── proto/    # agent/server tunnel message definitions
├── docs/     # implementation architecture
└── scripts/  # metadata migration runner
```

## Verification And Tests

Run Go verification from the repository root:

```sh
go build ./...
go vet ./...
go test ./...
```

This checkout currently contains no committed `*_test.go` files, frontend test files, or test framework. `go test ./...` therefore compiles packages but does not provide behavioral coverage. In particular, there is no automated reconciler create/update/delete/full-resync suite for any resource type and no WebSocket hub concurrency or race suite.

Architecture details and current limitations are in [`docs/doc.md`](docs/doc.md) and [`ARCHITECTURE.md`](ARCHITECTURE.md).

## License

TBD.
