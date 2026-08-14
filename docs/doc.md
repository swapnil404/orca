# Implementation Architecture

This document describes behavior confirmed in the current code. Product workflows that are not exposed by the current web application are identified explicitly.

## System Boundary

Orca's server stores control-plane users, projects, hosts, desired cluster state, agent reports, reconciliation results, and alert-rule state in its own Postgres database. It does not store user database contents. The agent runs Postgres-related Docker containers on the user's host and initiates every connection to the control plane; the server never opens a connection to that host.

| Component | Current responsibility |
|---|---|
| Agent | Cache desired state, observe Docker, reconcile resources, schedule backups, and report actual state |
| Server | Authenticate users and agents, persist metadata and reports, expose REST, and route WebSocket snapshots |
| Web UI | Read-only project topology and latest persisted status view |

## Authentication

`POST /auth/register` creates an email/password user with a bcrypt hash. `POST /auth/login` verifies that hash. Both return a 24-hour HS256 JWT whose subject is the user ID for non-browser clients. TanStack Start server functions exchange those responses for an httpOnly, same-site `orca.session` browser cookie without exposing the JWT to browser JavaScript. `ORCA_JWT_SECRET` is required at startup. Protected REST routes accept either `Authorization: Bearer <JWT>` or the browser cookie, require expiry and a nonempty subject, and check the database on every request to ensure the user is still active. Unsafe cookie-authenticated requests with a cross-origin `Origin` are rejected; explicit bearer clients remain non-ambient.

GitHub and Google OAuth use Goth. A provider is enabled only when both its client ID and secret are configured. `GET /auth/{provider}` starts the flow and `GET /auth/{provider}/callback` resolves an identity by `(provider, provider_user_id)` or creates a new OAuth-only user, then issues the same JWT shape into the browser session cookie and redirects to `/`. Provider email is metadata and never causes implicit linking. Linking a provider to an already-authenticated user is deliberately deferred pending an authenticated, CSRF-protected flow, as documented in `server/internal/api/oauth.go`.

`GET /auth/session` validates the active browser or bearer session and returns only the user ID. The pathless authenticated frontend route calls it through a TanStack Start server function before protected route loaders run, so unauthenticated document requests redirect before protected content renders.

Browser project WebSockets authenticate with the same-origin session cookie sent during the upgrade. The original exact `orca.jwt` plus `orca.jwt.token.<JWT>` subprotocol contract remains supported for existing bearer clients. The server validates the token and project ownership before upgrading.

`DELETE /account` soft-deletes the user. The active-user lookup rejects that user's JWT on future REST requests and future project-WebSocket handshakes; password and OAuth login also reject the deleted account. Soft deletion does not close an already-established project WebSocket, revoke agent tokens, delete owned infrastructure, or release the user's email/OAuth identities for reuse.

## Desired State And Reconnection

Cluster mutations are available through authenticated REST APIs, not through the current canvas. A committed cluster change appends desired state in the same metadata transaction, after which the local orchestrator attempts to send the host's complete current desired state through the active agent session.

Host snapshot revisions are `<desired-state revision>:<restore-event revision>` after the first restore event. Resource mutation responses return the desired-state component only, and frontend acknowledgement compares that component numerically while the agent treats and reports the complete snapshot revision as an opaque value.

Project restart requests increment every active cluster's durable restart generation and append complete desired-state snapshots in one transaction. Generations are latest-request-wins, matching the full-snapshot reconnection model rather than replaying every request made while an agent was offline. Agents stop PgBouncer and replicas before the primary, require the primary to start before dependants, and acknowledge the generation only after every currently managed container starts successfully. Offline agents execute the latest restart after receiving the current snapshot on reconnect. A crash after containers restart but before acknowledgement can cause an at-least-once retry.

The agent writes each received snapshot atomically to its local cache before reconciliation. It observes Docker, computes the ordinary create/update/delete diff, applies actions, observes Docker again, and reports post-apply state and individual action results. Apply continues after unrelated failures; known dependants are reported as skipped when their prerequisite failed.

While connected, cached reconciliation also runs every 30 seconds. During disconnection and connection backoff, the agent continues reconciling from the cache. After reconnecting, periodic reporting waits until a fresh complete server snapshot has been cached and reconciled. There is no event replay or delta-sync path.

## Resources

Containers use `orca-<cluster-id>-primary`, `orca-<cluster-id>-replica-<n>`, `orca-<cluster-id>-pgbouncer`, and temporary `orca-<cluster-id>-pgbackrest` names. Every container for one cluster attaches to its isolated `orca-<cluster-id>-network` user-defined bridge, which provides the container-name DNS used by PgBouncer and limits the trusted replication subnet to that cluster. Each cluster has an explicit Docker named volume mounted inside managed containers at `/var/orca/data/<cluster-id>`; this is not a host bind path. Orphan volumes remain observable and deletion is retried on later passes.

Primaries and replicas use deterministic Orca-owned PostgreSQL include files mounted at `/etc/orca/postgresql.conf`. Before changing a file, the agent rejects names absent from the running version's `pg_settings` and asks that version's `postgres` binary to parse each desired value. The live `pg_settings.context` classifies the complete change as reloadable unless at least one parameter has `postmaster` context, in which case every node is restarted. Orca-owned settings for authentication, replication, extension preload, and archiving are excluded from the arbitrary parameter map. A version change currently replaces the primary container while preserving its data volume; this is not a valid PostgreSQL major-version migration because no `pg_upgrade` or dump/restore path exists.

Replicas are provisioned with a base backup and streaming-replication configuration. Reported replica state includes connectivity, streaming state, lag bytes, lag classification, and the last successfully applied parameter map. PgBouncer configuration is generated from desired pool settings and includes read aliases for replica meshes. PgBouncer is the only client-facing endpoint: its desired publish address and port are bound on the agent host, while PostgreSQL remains private to the cluster network. The default bind is `127.0.0.1:6432`; use an externally reachable host address such as `0.0.0.0` only when host firewall policy permits it.

The agent generates one stable PostgreSQL password per cluster at `${ORCA_DATA_DIR}/<cluster-id>/secrets/postgres-password` with host mode `0600`. It synchronizes the `postgres` role to that password, gives PgBouncer the resulting SCRAM verifier rather than plaintext, requires SCRAM for application connections, and permits the `pgbouncer` admin user without a password only over the container-local Unix socket. Operators on the agent host use the generated password for client connections. Same-host PITR clones start with PgBouncer disabled because the source's published port cannot be reused safely; a distinct endpoint can be enabled after activation.

The extensions package manages `pgvector`, `powa`, `timescaledb`, `pg_partman`, and `postgis` when the selected primary image provides their files and libraries. It preserves unmanaged extensions and preload libraries.

## Backups And PITR

pgBackRest is reconciled as create/update/delete state. Its repository path is an absolute directory on the agent host, bind-mounted into backup-enabled primaries so backups survive primary container replacement; it must not overlap the cluster's managed primary, replica, configuration, or restore-work directories. The backup model and interval bounds are defined by `PgBackRestSpec` and `BackupSchedule` in `proto/orca.proto`; the server uses those generated Go types directly and the web resource types mirror their nested shape. Applying it installs configuration, enables WAL archiving, initializes the stanza, records applied state, and starts interval workers for desired full, differential, and incremental schedules. Scheduled operations share an exclusive gate with reconciliation and restore execution, and their results are queued for a later agent report.

Backup-enabled primaries use `orca-postgres:<version>`, built from `agent/images/postgres/Dockerfile`. If that image is absent, container creation automatically builds it through the Docker API using the corresponding official Postgres parent image; manual `make postgres-image POSTGRES_VERSION=17` is optional. The automatic build requires Docker build support and access to the parent image and Debian package repositories. Existing images are trusted by tag and are not inspected for pgBackRest. Disabling backups does not currently replace an existing Orca image with the official image.

Restore operations are durable server records with append-only events, idempotency keys, explicit intents, and monotonic agent reports. Active operations are included in every complete host snapshot. The agent stores a separately fsynced operation journal beside its desired-state cache and blocks ordinary reconciliation for clusters whose data may be in transition.

Preflight reads pgBackRest JSON metadata, chooses an explicit backup set at or before the requested timestamp, verifies that set, checks PostgreSQL major-version compatibility and storage capacity, and reports candidate recovery bounds. WAL continuity through the requested wall-clock target is confirmed by PostgreSQL replay during execution rather than inferred from archive filename bounds.

In-place recovery removes PgBouncer and replicas, preserves the original primary directory, restores into a clean directory, verifies paused recovery, promotes to read-write, and allows ordinary reconciliation to rebuild dependants. The retained original can be rolled back until the operation is finalized. A destructive failure remains journaled and blocks ordinary primary recovery.

Clone recovery reserves a new cluster specification without publishing it as normal desired cluster state. A temporary container mounts the source host repository read-only and a new target volume read-write. After recovery succeeds, the server atomically activates the target metadata and desired state; fresh replicas and PgBouncer then converge normally. Clone backups start disabled so the target cannot archive into the source repository. Local repositories restrict clone execution to the source host, and physical recovery requires matching PostgreSQL major versions.

Authenticated REST endpoints create, inspect, confirm, cancel, roll back, and finalize operations. Mutations require an organization owner or administrator, creation requires an idempotency key, and execution requires mode-specific typed confirmation. Project WebSocket snapshots expose durable operation progress to the backup UI.

## Actual State And Web UI

Agent reports are transactionally persisted before frontend subscribers are notified. Report reads mark data older than two minutes stale and return cluster health as `unknown`; no expiry worker is required.

The web UI lists existing projects and renders desired primary, replica, and PgBouncer nodes at deterministic, non-persistent positions. Nodes cannot be dragged or connected. Selecting a node opens a resource panel. Status badges are derived from persisted agent observations, server health, and report staleness rather than hardcoded values. Project settings expose PostgreSQL parameter rows with agent-reported reload/restart classification, effective values, and desired-versus-applied state across the primary and replicas. Backup operations expose schedule management plus durable in-place and same-host clone PITR workflows.

The project page opens a JSON WebSocket separate from the protobuf agent tunnel. It receives full snapshots after reports or desired-state mutations are committed and replaces the latest project snapshot in the frontend store. Snapshots include desired topology, actual state, and restore operations.

The frontend provides email/password registration and login plus GitHub/Google OAuth initiation. Its API URLs and browser session cookie are same-origin. The development Compose stack includes Caddy routing for frontend documents, JSON API requests, OAuth, agent traffic, and project WebSockets; production deployments need equivalent same-origin routing.

## Server Architecture

Durable data lives in Postgres through sqlc-generated queries and plain SQL migrations. Run `./scripts/migrate.sh` before server startup; neither the server nor sqlc applies migrations automatically.

The active agent hub uses an `RWMutex`, sessions serialize desired-state writes, the orchestrator serializes pushes per host, and frontend project subscriptions use their own mutex. These make one process safe for concurrent access, but there are no automated race tests.

The hub, frontend subscription map, desired-state push routing, and alert debounce windows are process-local. If an API request reaches one server while its target agent is connected to another, the first process persists the mutation but cannot push it through the second process's hub. Frontend report notifications have the same limitation. The current deployment model is therefore one server instance; horizontal scaling requires cross-instance session routing/pub-sub and coordinated alert evaluation.

## Tests

The current repository contains no committed Go `*_test.go` files and no frontend test framework or test files. No package has behavioral test coverage. `go test ./...` compiles all Go packages but does not verify behavior.

There is no reconciler suite covering create, update, delete, or empty-actual full resync for primaries, replicas, PgBouncer, pgBackRest, or extensions. There are also no tests for primary replacement failure ordering, replica cleanup, the extension second pass, automatic image building, backup scheduling/PITR, API/store behavior, JWT/OAuth behavior, project events, or hub/session concurrency.

## Protocol Boundary

`proto/` defines binary messages only for the agent-server tunnel. REST request/response and browser event shapes remain in `server/internal/api`. Agent and server share transport types and pure cross-binary policy through `pkg/` and do not import each other's internal packages.
