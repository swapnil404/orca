# Orca

Orca is an in-development, self-hosted Postgres orchestration and control platform. Its agent runs on infrastructure you control, keeps the last desired state locally, and reconciles Docker resources without requiring an inbound connection from the control plane. Orca stores desired state and reported health, not user database data.

The backend and agent contain most of the current functionality. The web application is a read-only topology and status view; it does not yet provide the management workflows described below.

## Current status

Implemented in the backend and agent:

- Full desired-state snapshots over an agent-initiated WebSocket connection, including full resync after reconnecting.
- Local desired-state caching and periodic reconciliation while the control plane is unavailable.
- Postgres primary lifecycle and streaming-replica provisioning, observation, and deletion.
- PgBouncer lifecycle with a fixed backend-generated configuration.
- pgBackRest configuration and scheduled full, differential, and incremental backups.
- Reconciliation of a limited set of Postgres extensions when the selected image contains them.
- Agent health reports, persisted reconciliation results, and Prometheus-compatible server metrics.
- Backend CRUD for projects and clusters, plus a backend endpoint that registers a host and returns an agent token and `docker run` command.
- Agent token authentication for the control-plane tunnel.
- Email/password account registration and login, GitHub/Google OAuth, JWT-protected REST routes, and authenticated project event WebSockets.

Incomplete or not yet exposed as a usable product workflow:

- The web UI has no account registration or login screens yet. The authentication API is available, but the current UI only consumes a JWT already stored by a client.
- The web UI displays projects and topology but is read-only. It cannot create or edit projects, hosts, clusters, replicas, pools, backups, or extensions.
- Host registration exists as a backend endpoint, not as a UI flow, and host listing and management are not implemented.
- Point-in-time recovery code exists in the agent but has no API, tunnel operation, CLI, or UI entry point.
- Extension controls are not connected to the UI, and the supplied Postgres images do not bundle every supported extension package.
- Alert rule storage and server-side evaluation exist, but there is no alert API, UI, or notification delivery. This is not yet operational alerting.

## Architecture

1. The agent starts on a user-controlled Linux host and authenticates to the control plane with an agent token.
2. The agent opens the outbound WebSocket connection. The server never initiates a connection to the user's host.
3. The server sends a complete desired-state snapshot for that host.
4. The agent compares desired state with Docker state and independently applies each required create, update, or delete action.
5. The agent reports observed topology, health, and reconciliation outcomes to the server.

After a disconnect, the server sends the full current desired state rather than replaying missed changes. While disconnected, the agent continues reconciling from its local cache.

## Running the components

This repository is currently suitable for development and direct API integration, not the earlier advertised account-and-click quickstart. The server requires `DATABASE_URL` and a nonempty `ORCA_JWT_SECRET`; the agent can run in local development mode or connect with `ORCA_SERVER_URL` and `ORCA_TOKEN`. See `.env.example` for every environment variable read by the agent and server.

### OAuth apps for local development

GitHub and Google OAuth are optional. For either provider, set both its client ID and client secret; the server rejects a half-configured pair. The callback origin is derived from `ORCA_SERVER_URL` by converting `ws` to `http` or `wss` to `https` and removing the `/agent` path.

Create OAuth applications with these callback URLs when the server runs at the default local URL:

- GitHub: `http://localhost:8080/auth/github/callback`
- Google: `http://localhost:8080/auth/google/callback`

Both providers require the callback URL to be registered even when it points to localhost. Set `ORCA_GITHUB_CLIENT_ID` and `ORCA_GITHUB_CLIENT_SECRET` for GitHub, and `ORCA_GOOGLE_CLIENT_ID` and `ORCA_GOOGLE_CLIENT_SECRET` for Google. Begin a login at `GET /auth/github` or `GET /auth/google`; a successful callback returns the same JWT response shape as email/password authentication.

A registered agent is run with the host's generated token and control-plane URL, for example:

```sh
docker run -d \
  -e ORCA_TOKEN=replace-with-agent-token \
  -e ORCA_SERVER_URL=wss://your-orca-server.example/agent \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /proc:/host/proc:ro \
  -v /var/orca/data:/var/orca/data \
  orca/agent
```

No inbound agent port is required. The exact registration command currently comes from `POST /hosts`; there is no registration screen in the web application yet.

## Project structure

```text
orca/
├── agent/    # Docker reconciliation and outbound tunnel client
├── server/   # REST API, WebSocket hub, desired-state store, metrics
├── web/      # read-only canvas and status UI
├── pkg/      # shared Go types
├── proto/    # agent/server tunnel message definitions
├── deploy/   # local deployment assets
└── scripts/  # migrations and development scripts
```

## Development

Go changes are verified from the repository root with:

```sh
go build ./...
go vet ./...
go test ./...
```

Architecture and implementation notes are in `ARCHITECTURE.md` and `docs/`.

## License

TBD.
