# AGENTS.md

This describes the Orca codebase for AI coding agents (opencode, Claude, Copilot, etc). Read this before making changes anywhere in the repo.

## What Orca is

Orca is a self-hosted Postgres orchestration and control platform. Users run a single agent container on their own infrastructure. The agent connects outbound to Orca's control plane over a persistent WebSocket connection. Through a web UI, users manage Postgres clusters, replicas, connection pooling, backups, and extensions, all of which actually run on the user's own host, not on Orca's infrastructure.

Orca owns no servers running user data. All Postgres infrastructure runs on the user's own host. Orca's server only stores desired state and reported health, and pushes desired state down to agents.

## Monorepo structure

```
orca/
├── agent/          # Go, runs on the user's host, reconciles Docker state
├── server/         # Go, control plane: REST API, WebSocket hub, desired state store
├── web/            # React, canvas UI
├── pkg/            # shared Go types, imported by both agent and server
├── proto/          # message definitions for agent <-> server communication
├── deploy/         # Docker Compose for local development
└── scripts/        # migration and codegen scripts
```

## Module boundaries

| Module | Language | Responsibility |
|---|---|---|
| `agent/` | Go | Docker reconciliation, local state cache, tunnel client |
| `server/` | Go | Desired state store, WebSocket hub, REST API, auth |
| `web/` | TypeScript / React | Canvas UI, real-time topology |
| `pkg/` | Go | Shared types only, no business logic |
| `proto/` | Protobuf | Agent <-> server message contracts |

**Never import `agent/` from `server/` or vice versa.** Shared types live in `pkg/` only. Agent and server communicate exclusively over the WebSocket tunnel, they never share in-process state or call each other's internal packages directly.

## Transport

Agent to server communication is a WebSocket connection, initiated by the agent, never by the server. The server never opens a connection to a user's host and never needs an inbound port on the user's infrastructure. This is a deliberate security property, not an implementation detail, do not introduce any transport that requires the server to reach into a user's host.

Message shapes for this connection are defined in `proto/` and generated into `pkg/`. Proto is only for the agent-server tunnel. Do not add REST-style request/response shapes to proto, REST lives in `server/internal/api` and talks to the web frontend, not to agents.

## The reconnection rule

This is the single most important correctness property in the system. If an agent disconnects from the server and reconnects later, the server sends the full current desired state, not a queue of changes that happened while it was offline. The agent does not replay missed events, it reconciles against whatever the server currently says desired state is. Any code touching the WebSocket hub, the orchestrator, or the agent's tunnel client must preserve this property. Do not implement delta syncing or event replay as a substitute for this.

The agent must also be fully functional with no server connection at all. Reconciliation always reads from a local cache of the last known desired state, never blocks on server availability.

## Agent (`agent/`)

```
agent/internal/
├── docker/       # Docker SDK wrapper, container and volume lifecycle
├── reconciler/   # diff(desired, actual) and apply(actions), core correctness logic
├── state/        # local desired-state cache, survives disconnects and restarts
├── tunnel/       # WebSocket client, auth, reconnection, heartbeat
├── postgres/     # primary and replica provisioning, streaming replication config
├── pgbouncer/    # PgBouncer config generation and container lifecycle
├── pgbackrest/   # pgBackRest config, backup scheduling, PITR
└── extensions/   # per-cluster extension enable/disable
```

Correctness rules for this module:
- Diff logic must be a pure function: no I/O, no Docker calls, fully testable with in-memory structs.
- Delete handling is not optional or deferred. Anything present in actual state but absent from desired state must produce a delete action.
- A full resync, where actual state is empty and desired state has everything, must go through the same diff path as a normal partial diff. Do not special-case it.
- Apply logic must not stop on the first failed action. Each action's success or failure is reported independently.
- Replica provisioning must configure real streaming replication against the primary, not just start a second Postgres container. A replica container that isn't actually replicating is a bug, not a partial implementation.
- PgBouncer, pgBackRest, and extensions each get their own package rather than being special cases inside `reconciler` or `docker`. `reconciler` orchestrates what needs to happen, these packages know how to make it happen for their specific service.

Docker naming conventions:
- Containers: `orca-<cluster-id>-primary`, `orca-<cluster-id>-replica-<n>`, `orca-<cluster-id>-pgbouncer`, `orca-<cluster-id>-pgbackrest`.
- Data volumes: named and explicit, under `/var/orca/data/<cluster-id>/`. Never anonymous volumes.

## Server (`server/`)

```
server/internal/
├── api/           # REST API consumed by the web frontend
├── ws/            # WebSocket hub, host_id -> session map
├── orchestrator/  # desired state diffing and pushing to the correct agent session
├── store/         # database layer, built with sqlc
├── auth/          # token issuance and validation, JWT
└── metrics/       # health report ingestion, Prometheus-compatible exposition, alert rules
```

The WebSocket hub must be safe for concurrent access, sessions are added and removed from multiple goroutines. Use a mutex or equivalent, and any change to the hub needs a concurrency test, not just a happy-path test.

The orchestrator looks up the correct session by host ID before pushing desired state. A change intended for one host must never be sent to another host's session.

### Database access

The server uses `sqlc` for all query access to its own Postgres metadata database. Write raw SQL in `.sql` query files, run `sqlc generate` to produce typed Go functions, call those from the store layer. Do not hand-roll `Scan()` calls or introduce an ORM. Migrations are plain SQL files applied via the project's migration script, sqlc does not manage migrations itself.

### Auth

Email and password with JWTs is the baseline. Do not add OAuth or other providers without an explicit decision to do so, keep the auth surface minimal and correct rather than broad.

### Metrics and alerts

Health reports ingested from agents are exposed in a Prometheus-compatible format so users can scrape their own infrastructure's metrics if they choose to, in addition to what's shown in the canvas. Alert rules are evaluated server side against ingested health data. Do not couple alerting logic to the WebSocket ingestion handler directly, keep ingestion, storage, and rule evaluation as separate concerns so one can be tested without the others.

## Frontend (`web/`)

```
web/src/
├── canvas/      # React Flow canvas, nodes and edges
├── panels/      # config panels, opened when a node is clicked
├── api/         # typed API client, all fetching goes through here
├── store/       # Zustand global state
├── hooks/       # useWebSocket, useCluster, useTopology
└── types/       # TypeScript types mirroring pkg/types Go structs
```

Conventions:
- Functional components only.
- No inline API calls inside components or stores, all fetching goes through `api/`.
- Config UI belongs in `panels/`, not inside node components.
- Node status (healthy, degraded, down, pending) must reflect real actual state from the server. Do not hardcode or fake a status value anywhere.
- Any endpoint the frontend relies on for persistence (for example, topology node positions) must actually persist. Do not return a success response from an endpoint that has not written the change.

## Environment variables

Use `ORCA_` prefixed names consistently across agent, server, and documentation. Do not introduce alternate names for the same concept.

| Variable | Used by | Description |
|---|---|---|
| `ORCA_TOKEN` | agent | Auth token for the agent-server tunnel |
| `ORCA_SERVER_URL` | agent | WebSocket URL of the orchestration server |
| `ORCA_DATA_DIR` | agent | Host path for data volumes |
| `ORCA_JWT_SECRET` | server | Secret for signing tokens |
| `DATABASE_URL` | server | Postgres connection string for the server's own metadata DB |
| `ORCA_PORT` | server | HTTP port |
| `ORCA_LOG_LEVEL` | agent, server | Log level: debug, info, warn, error |

If a new environment variable is introduced, add it to this table and to `.env.example` in the same change.

## Testing

- Go: table-driven tests using `t.Run()`.
- Reconciler diff and apply logic require tests covering create, update, delete, and full resync at minimum. This code is not considered done without them.
- WebSocket hub requires a concurrency test, not just single-session coverage.
- Docker and database access should be tested against fakes or mocks where reasonable, not require a real Docker daemon or live database to run in CI.
- New features need at least one test covering the happy path and one covering a failure case.

## Coding conventions

### Go
- Errors are returned, never panicked, except in `main()` during startup.
- Use `context.Context` for all operations touching the network, Docker, or the database.
- No global state outside of `main()`, pass dependencies explicitly.
- Exported types and functions require godoc comments.

### TypeScript / React
- Functional components only, no class components.
- Props interfaces defined above the component they belong to.
- No `any`, use proper types or `unknown` with narrowing.
- Prefer `const` over `let`.

### General
- Commit messages follow Conventional Commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`.
- No secrets, tokens, or real-looking credentials committed anywhere, including in `deploy/` compose files. Use `.env.example` with placeholders.
- No built binaries tracked in the repository.
- Do not mark work as complete in a PR description or issue comment unless it is backed by a passing test and, where applicable, a manual verification step.

## Documentation

Architecture-level decisions are documented in `docs/`. Before making a change that affects module boundaries, the transport layer, the reconciliation model, or the data model, check whether a relevant doc already exists. If a change contradicts an existing doc, update the doc in the same change. If a significant new decision is made, document it rather than leaving it implicit in code.
