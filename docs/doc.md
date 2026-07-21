# Architecture

## Overview

Orca is split into two halves that never trust each other more than necessary.

**Control plane (Orca's servers):** stores desired state, exposes the API and web UI, tracks which hosts are online, and pushes configuration changes down to agents. It holds no Postgres data.

**Self-hosted (the user's infrastructure):** runs the agent, which runs the user's actual Postgres containers, replicas, connection pooling, and backups via Docker.

This split exists so that a user's data never has to leave their own infrastructure, and so Orca's control plane never needs a way into a user's host. That second property shapes the transport layer directly: connections are always initiated by the agent, never by the server.

## Components

| Component | Runs where | Responsibility |
|---|---|---|
| Agent | User's host | Reconciles Docker state to match desired state |
| Server | Orca's infrastructure | Stores desired state, tracks actual state, exposes API |
| Web UI | Browser | Canvas for configuring infrastructure, real-time topology view |

## Registration flow

1. A user adds a host from the web UI. The server issues a token tied to that user and generates a `docker run` command containing it.
2. The user runs that command on their own infrastructure. The agent container starts with the token as an environment variable and Docker socket access mounted in.
3. The agent opens an outbound WebSocket connection to the server, authenticates with the token, and reports its capabilities.
4. The server validates the token, associates the resulting session with the correct host, and the host becomes visible in the UI.

No inbound port is ever required on the user's host for this to work.

## Desired state flow

1. A user makes a change in the canvas UI, for example adding a replica.
2. The frontend calls the REST API.
3. The API validates the request and writes the new desired state to the store.
4. The orchestrator looks up the WebSocket session belonging to the correct host and pushes the updated desired state down that connection.
5. The agent receives the desired state, diffs it against what's actually running, and applies the difference through the Docker SDK.
6. The agent reports the resulting actual state and health back over the same connection.
7. The server updates its record of actual state. The frontend, which holds its own WebSocket connection to the server, reflects the update in the canvas without a full reload.

## The agent's reconciliation loop

The agent does not act on individual commands from the server. It receives a full desired state, computes a diff against the current actual state (queried from Docker), and applies whatever changes that diff requires. This makes the agent idempotent by construction, pushing the same desired state twice produces no further changes the second time.

The loop:
1. Read desired state from the local cache.
2. Query Docker for actual running containers and their status.
3. Diff desired against actual, producing a list of actions: create, update, delete.
4. Apply those actions through the Docker SDK.
5. Report the new actual state and health back to the server.

This runs on a timer and also triggers immediately whenever a new desired state message arrives.

`agent/internal/reconciler.Runner` owns this path for both the production tunnel and the development RPC harness. A server snapshot is written atomically to the local desired-state cache before Docker is queried. The runner observes Docker again after applying every action and builds the `AgentReportMessage` from that post-apply observation; individual apply failures remain independent and appear in the development result while the report reflects what is actually running.

`agent/internal/tunnel.Client` connects to `ORCA_SERVER_URL`, sends `ORCA_TOKEN` as the first JSON WebSocket frame, then exchanges binary protobuf messages. Each `DesiredStateMessage` is a complete snapshot and triggers the shared runner immediately. The resulting `AgentReportMessage` is sent on the same connection. While connected, the client also runs the cached reconciliation path every 30 seconds and reports each result.

Currently the primary is the only service this loop provisions end to end. Replica, PgBouncer, pgBackRest, and extension provisioning are designed but not yet implemented, see "Provisioning scope" below for the target shape of that work.

## The reconnection rule and split-brain avoidance

This is the correctness property the rest of the system depends on. If the agent's connection to the server drops, the agent continues operating from its local cache of the last known desired state. It does not require the server to be reachable to keep running Postgres correctly, it simply stops receiving new configuration changes until the connection is restored.

When the connection is restored, the server does not attempt to replay a log of what changed while the agent was offline. It sends the full current desired state, as it exists right now, in one message. The agent reconciles against that as it would any other desired state update.

The tunnel retries connection and authentication failures with exponential backoff from one second to 30 seconds. Between attempts it reconciles from the local cache, without waiting on server availability. On a new connection, periodic reporting remains paused until the server's fresh full desired-state snapshot has been cached and reconciled, so stale pre-disconnect state cannot be reported ahead of the reconnect snapshot.

This avoids split-brain scenarios where the agent and server disagree about history. There is no history to disagree about, only a current desired state and a current actual state, reconciled on every pass. An agent that has been offline for five minutes and an agent that has been offline for five days go through the exact same reconnection path.

## Data model

**Desired state** describes what the user wants to exist: which clusters, their Postgres version and configuration, their replicas, and whether connection pooling or backups are configured. It is owned by the server and pushed to agents.

**Actual state** describes what is really running, as reported by the agent after querying Docker directly. It is owned by the agent and reported to the server.

The server never assumes actual state without a report from the agent confirming it. An endpoint that would otherwise need to guess or fake a status must instead reflect only what has actually been confirmed.

The server considers an agent report current for two minutes. Status reads compute staleness from the report's `reported_at` timestamp; after that window host reports are marked stale and per-cluster health is returned as `unknown`. This is deliberately a read-time check, so no background expiry job is required and a last known report remains available for diagnostics without being presented as current health.

## Container and volume conventions

Postgres-related containers run as siblings to the agent container, not nested inside it, communicating through the Docker socket mounted from the host.

- Containers are namespaced by cluster ID: `orca-<cluster-id>-primary`, `orca-<cluster-id>-replica-<n>`, `orca-<cluster-id>-pgbouncer`, `orca-<cluster-id>-pgbackrest`.
- Data volumes are named and explicit, mounted at `/var/orca/data/<cluster-id>/` on the host. Anonymous volumes are not used, since named volumes are what allow data to survive container restarts and agent upgrades.

## Provisioning scope

The reconciler's diff and apply logic, and the tunnel and reconnection behavior described above, are service-agnostic: they operate on whatever actions a spec produces, not specifically on primaries. Extending provisioning beyond the primary is additive work within `agent/internal`, not a change to the reconciliation model itself.

**Replicas** (`agent/internal/postgres`): a replica action must configure real streaming replication against the cluster's primary, not just start a second Postgres container with the same image. Actual state reporting for a replica includes replication status (streaming, lagging, disconnected), not just container running/stopped, so the canvas can distinguish a replica that's up from a replica that's healthy.

**PgBouncer** (`agent/internal/pgbouncer`): generates a pool configuration from the cluster's `PgBouncerSpec` (pool mode, per-database or standalone) and manages the PgBouncer container's lifecycle alongside the primary and its replicas.

**pgBackRest** (`agent/internal/pgbackrest`): generates backup configuration and manages scheduled full, differential, and WAL backups, with support for point-in-time recovery. This is the one piece of provisioning that has an ongoing responsibility beyond reconciling to a snapshot, backup scheduling runs independently of the diff/apply cycle even though its container lifecycle is managed the same way as other services.

**Extensions** (`agent/internal/extensions`): enables or disables extensions per cluster by reconciling against the cluster's desired extension list, applied against the running primary rather than through a separate container.

Each of these follows the same naming and volume conventions already established for the primary. None of them changes the core reconciliation loop, hub, or reconnection behavior described elsewhere in this document, they extend what a `ClusterSpec` can describe and what the apply step knows how to execute.

## Server internals

The server is stateless with respect to its own process, all durable state lives in its own Postgres metadata database, accessed through a `sqlc`-generated typed query layer. Multiple server instances can run behind a load balancer without coordination between them, since coordination state (which host is connected to which instance) lives in the WebSocket hub of whichever instance holds that connection, not in a shared in-memory structure.

The WebSocket hub maintains a map from host ID to the active session for that host. It must be safe under concurrent reads and writes, since sessions connect, disconnect, and receive pushes from different goroutines simultaneously.

### Metadata database

The server metadata schema is defined by ordered, plain SQL migrations in `server/migrations/`. Run them with:

```sh
DATABASE_URL=postgres://... ./scripts/migrate.sh
```

The script records applied filenames in `schema_migrations` and applies each new migration in a transaction. sqlc does not apply or otherwise manage migrations.

sqlc is configured in `server/sqlc.yaml`. Its source queries are grouped by resource:

- `server/internal/store/queries/hosts.sql`
- `server/internal/store/queries/projects.sql`
- `server/internal/store/queries/clusters.sql`
- `server/internal/store/queries/desired_states.sql`
- `server/internal/store/queries/reports.sql`

Run generation from the server module:

```sh
cd server
sqlc generate
```

Generated typed functions and models are written to `server/internal/store/sqlcdb/`. The handwritten store in `server/internal/store/hosts.go`, `projects.go`, and `clusters.go` only calls those generated functions. Multi-write operations use the generated `WithTx` support; the store does not contain handwritten row scanning.

Projects are owned by a user. Clusters belong to a project and one of that same user's hosts. Ownership is included in the SQL predicates for reads and mutations so an ID from another user behaves as not found.

Projects and clusters use soft deletion. Every cluster create or update appends an `upsert` row to `desired_states` in the same transaction. Every cluster delete appends a `delete` row containing an explicit `exists: false` tombstone. Deleting a project performs the same operation for each active cluster before soft-deleting the project. The latest record per cluster determines the current full desired state for a host; clusters whose latest record is a tombstone are omitted. Historical rows are retained for diagnostics, but agents receive current state rather than event replay.

### Resource API

`server/internal/api/resources.go` registers these authenticated routes:

| Method | Path | Operation |
|---|---|---|
| `POST` | `/projects` | Create a project |
| `GET` | `/projects` | List owned projects |
| `GET` | `/projects/{projectID}` | Get an owned project |
| `PUT` | `/projects/{projectID}` | Update an owned project |
| `DELETE` | `/projects/{projectID}` | Delete an owned project and tombstone its clusters |
| `POST` | `/projects/{projectID}/clusters` | Create a cluster on an owned host |
| `GET` | `/projects/{projectID}/clusters` | List clusters in an owned project |
| `GET` | `/clusters/{clusterID}` | Get an owned cluster |
| `PUT` | `/clusters/{clusterID}` | Update an owned cluster and desired state |
| `DELETE` | `/clusters/{clusterID}` | Delete an owned cluster and write a tombstone |
| `GET` | `/projects/{projectID}/events` | Upgrade to a project-scoped frontend WebSocket |

Authentication middleware supplies the verified identity with `api.WithUserID`; handlers never accept a user ID from JSON or a path. Host registration uses the same context identity so host ownership can be checked when a cluster is created.

### Frontend project events

`server/internal/api/project_events.go` owns the frontend WebSocket endpoint. It is separate from the protobuf agent tunnel: browser clients connect to `GET /projects/{projectID}/events`, and the server verifies that the authenticated user owns that project before upgrading the connection. A connection subscribes to exactly the project in its URL.

Every message is a full JSON snapshot rather than a delta:

```json
{
  "type": "project_state",
  "project_id": "project-id",
  "clusters": [
    {
      "cluster_id": "cluster-id",
      "host_id": "host-id",
      "actual_state": {},
      "health": "healthy",
      "last_seen": "2026-07-21T12:00:00Z",
      "stale": false
    }
  ]
}
```

The handler sends a fresh snapshot as part of registering every connection. Snapshot loading and subscription registration are serialized with report publication, so a reconnect either includes a concurrently committed report in its initial snapshot or receives a subsequent snapshot for it. No missed-event queue or replay is used.

After `StoreAgentReport` commits, the agent WebSocket handler calls the configured report notifier. `ProjectEventHandler.NotifyHostReport` resolves the active projects assigned to that host and publishes only to clients subscribed to those project IDs. Server construction wires the two handlers explicitly:

```go
projectEvents := api.NewProjectEventHandler(metadataStore)
projectEvents.RegisterRoutes(mux)
agentHandler.SetReportNotifier(projectEvents)
```

### Metrics and alerting

Health data ingested from agent reports is planned to be exposed in a Prometheus-compatible format from the server, in addition to what's shown live in the canvas, so users can scrape their own infrastructure's metrics independently. Alert rule evaluation runs against ingested health data server side. This is not yet implemented; when it is, ingestion (already built, see "Data model" above), exposition, and rule evaluation should remain separable, each testable without the others, consistent with how `metrics/` is scoped in `AGENTS.md`.

### Database tests

Store integration tests use an isolated schema in a real Postgres database and apply the migration files before exercising the generated queries. They are opt-in so ordinary unit tests do not require Docker or a database:

```sh
cd server
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' go test ./internal/store -v
```

The configured database user must be able to create and drop schemas. API tests use a fake store and cover authentication, user scoping, successful mutation, and not-found behavior.

## Frontend

The web UI holds its own WebSocket connection to the server, separate from the agent-server tunnel, used to receive real-time topology and health updates and reflect them in the canvas. All outbound requests from the frontend, configuration changes, host registration, and so on, go through the REST API, not through this connection.

## What proto is for, and isn't

`proto/` defines the message contract exclusively between agent and server, sent over the WebSocket tunnel. It is not used for REST request or response shapes, those are defined and validated within the server's own API layer. Keeping this boundary clean avoids the tunnel contract growing to accommodate concerns that only the frontend needs.

## Non-goals for the current architecture

- The server does not, and should not, ever initiate a connection to a user's host.
- The agent does not persist a log of past desired states or replay history on reconnect.
- `pkg/` contains shared types only. Business logic belongs in the module that owns the relevant data, not in the shared package.
