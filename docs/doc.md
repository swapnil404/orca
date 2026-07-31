# Implementation Architecture

This document describes behavior confirmed in the current code. Product workflows that are not exposed by the current web application are identified explicitly.

## System Boundary

Orca's server stores control-plane users, projects, hosts, desired cluster state, agent reports, reconciliation results, and alert-rule state in its own Postgres database. It does not store user database contents. The agent runs Postgres-related Docker containers on the user's host and initiates every connection to the control plane; the server never opens a connection to that host.

| Component | Current responsibility |
|---|---|
| Agent | Cache desired state, observe Docker, reconcile resources, schedule backups, and report actual state |
| Server | Authenticate users and agents, persist metadata and reports, expose REST/metrics, and route WebSocket snapshots |
| Web UI | Read-only project topology and latest persisted status view |

## Authentication

`POST /auth/register` creates an email/password user with a bcrypt hash. `POST /auth/login` verifies that hash. Both return a 24-hour HS256 JWT whose subject is the user ID for non-browser clients. TanStack Start server functions exchange those responses for an httpOnly, same-site `orca.session` browser cookie without exposing the JWT to browser JavaScript. `ORCA_JWT_SECRET` is required at startup. Protected REST routes accept either `Authorization: Bearer <JWT>` or the browser cookie, require expiry and a nonempty subject, and check the database on every request to ensure the user is still active. Unsafe cookie-authenticated requests with a cross-origin `Origin` are rejected; explicit bearer clients remain non-ambient.

GitHub and Google OAuth use Goth. A provider is enabled only when both its client ID and secret are configured. `GET /auth/{provider}` starts the flow and `GET /auth/{provider}/callback` resolves an identity by `(provider, provider_user_id)` or creates a new OAuth-only user, then issues the same JWT shape into the browser session cookie and redirects to `/`. Provider email is metadata and never causes implicit linking. Linking a provider to an already-authenticated user is deliberately deferred pending an authenticated, CSRF-protected flow, as documented in `server/internal/api/oauth.go`.

`GET /auth/session` validates the active browser or bearer session and returns only the user ID. The pathless authenticated frontend route calls it through a TanStack Start server function before protected route loaders run, so unauthenticated document requests redirect before protected content renders.

Browser project WebSockets authenticate with the same-origin session cookie sent during the upgrade. The original exact `orca.jwt` plus `orca.jwt.token.<JWT>` subprotocol contract remains supported for existing bearer clients. The server validates the token and project ownership before upgrading.

`DELETE /account` soft-deletes the user. The active-user lookup rejects that user's JWT on future REST requests and future project-WebSocket handshakes; password and OAuth login also reject the deleted account. Soft deletion does not close an already-established project WebSocket, revoke agent tokens, delete owned infrastructure, or release the user's email/OAuth identities for reuse.

## Desired State And Reconnection

Cluster mutations are available through authenticated REST APIs, not through the current canvas. A committed cluster change appends desired state in the same metadata transaction, after which the local orchestrator attempts to send the host's complete current desired state through the active agent session.

Project restart requests increment every active cluster's durable restart generation and append complete desired-state snapshots in one transaction. Generations are latest-request-wins, matching the full-snapshot reconnection model rather than replaying every request made while an agent was offline. Agents stop PgBouncer and replicas before the primary, require the primary to start before dependants, and acknowledge the generation only after every currently managed container starts successfully. Offline agents execute the latest restart after receiving the current snapshot on reconnect. A crash after containers restart but before acknowledgement can cause an at-least-once retry.

The agent writes each received snapshot atomically to its local cache before reconciliation. It observes Docker, computes the ordinary create/update/delete diff, applies actions, observes Docker again, and reports post-apply state and individual action results. Apply continues after unrelated failures; known dependants are reported as skipped when their prerequisite failed.

While connected, cached reconciliation also runs every 30 seconds. During disconnection and connection backoff, the agent continues reconciling from the cache. After reconnecting, periodic reporting waits until a fresh complete server snapshot has been cached and reconciled. There is no event replay or delta-sync path.

## Resources

Containers use `orca-<cluster-id>-primary`, `orca-<cluster-id>-replica-<n>`, `orca-<cluster-id>-pgbouncer`, and temporary `orca-<cluster-id>-pgbackrest` names. Each cluster has an explicit Docker named volume mounted inside managed containers at `/var/orca/data/<cluster-id>`; this is not a host bind path. Orphan volumes remain observable and deletion is retried on later passes.

Primaries and replicas use deterministic Orca-owned PostgreSQL include files mounted at `/etc/orca/postgresql.conf`. Before changing a file, the agent rejects names absent from the running version's `pg_settings` and asks that version's `postgres` binary to parse each desired value. The live `pg_settings.context` classifies the complete change as reloadable unless at least one parameter has `postmaster` context, in which case every node is restarted. Orca-owned settings for authentication, replication, extension preload, and archiving are excluded from the arbitrary parameter map. A version change currently replaces the primary container while preserving its data volume; this is not a valid PostgreSQL major-version migration because no `pg_upgrade` or dump/restore path exists.

Replicas are provisioned with a base backup and streaming-replication configuration. Reported replica state includes connectivity, streaming state, lag bytes, lag classification, and the last successfully applied parameter map. PgBouncer configuration is generated from desired pool settings and includes read aliases for replica meshes.

The extensions package manages `pgvector`, `powa`, `timescaledb`, `pg_partman`, and `postgis` when the selected primary image provides their files and libraries. It preserves unmanaged extensions and preload libraries.

## Backups And PITR

pgBackRest is reconciled as create/update/delete state. Applying it installs configuration, enables WAL archiving, initializes the stanza, records applied state, and starts interval workers for desired full, differential, and incremental schedules. Scheduled operations share an exclusive gate with reconciliation and their results are queued for a later agent report.

Backup-enabled primaries use `orca-postgres:<version>`, built from `agent/images/postgres/Dockerfile`. If that image is absent, container creation automatically builds it through the Docker API using the corresponding official Postgres parent image; manual `make postgres-image POSTGRES_VERSION=17` is optional. The automatic build requires Docker build support and access to the parent image and Debian package repositories. Existing images are trusted by tag and are not inspected for pgBackRest. Disabling backups does not currently replace an existing Orca image with the official image.

`pgbackrest.RestoreToTime` implements local-repository PITR. It validates that the repository is inside the shared cluster volume, stops the primary, restores in a temporary pgBackRest container with `--delta --type=time --target-action=pause`, restarts Postgres, waits for paused recovery, resumes replay, and waits for read-write state. A failure before restore starts restarts the untouched primary; a failure after restore starts leaves the primary stopped because `PGDATA` may be partially rewritten.

PITR has no API, tunnel message, CLI, development RPC, or UI caller. It also does not coordinate replicas or PgBouncer and does not acquire the scheduler/reconciler operation gate, so any future caller must provide that coordination.

## Actual State And Web UI

Agent reports are transactionally persisted before frontend subscribers are notified. Report reads mark data older than two minutes stale and return cluster health as `unknown`; no expiry worker is required.

The web UI lists existing projects and renders desired primary, replica, and PgBouncer nodes at deterministic, non-persistent positions. Nodes cannot be dragged or connected. Selecting a node opens a read-only panel. Status badges are derived from persisted agent observations, server health, and report staleness rather than hardcoded values. Project settings expose PostgreSQL parameter rows with agent-reported reload/restart classification, effective values, and desired-versus-applied state across the primary and replicas.

The project page opens a JSON WebSocket separate from the protobuf agent tunnel. It receives full snapshots after reports are committed and replaces the latest actual-state snapshot in the frontend store. Desired topology is loaded by REST; resource mutations do not themselves publish a refreshed desired topology to an already-open browser page.

The frontend provides email/password registration and login plus GitHub/Google OAuth initiation. Its API URLs and browser session cookie are same-origin. The development Compose stack includes Caddy routing for frontend documents, JSON API requests, OAuth, agent traffic, and project WebSockets; production deployments need equivalent same-origin routing.

## Server Architecture

Durable data lives in Postgres through sqlc-generated queries and plain SQL migrations. Run `./scripts/migrate.sh` before server startup; neither the server nor sqlc applies migrations automatically.

The active agent hub uses an `RWMutex`, sessions serialize desired-state writes, the orchestrator serializes pushes per host, and frontend project subscriptions use their own mutex. These make one process safe for concurrent access, but there are no automated race tests.

The hub, frontend subscription map, desired-state push routing, and alert debounce windows are process-local. If an API request reaches one server while its target agent is connected to another, the first process persists the mutation but cannot push it through the second process's hub. Frontend report notifications have the same limitation. The current deployment model is therefore one server instance; horizontal scaling requires cross-instance session routing/pub-sub and coordinated alert evaluation.

## Tests

The current repository contains no committed Go `*_test.go` files and no frontend test framework or test files. No package has behavioral test coverage. `go test ./...` compiles all Go packages but does not verify behavior.

There is no reconciler suite covering create, update, delete, or empty-actual full resync for primaries, replicas, PgBouncer, pgBackRest, or extensions. There are also no tests for primary replacement failure ordering, replica cleanup, the extension second pass, automatic image building, backup scheduling/PITR, API/store behavior, JWT/OAuth behavior, project events, or hub/session concurrency.

## Protocol Boundary

`proto/` defines binary messages only for the agent-server tunnel. REST request/response and browser event shapes remain in `server/internal/api`. Agent and server share types through `pkg/` and do not import each other's internal packages.
