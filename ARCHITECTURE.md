# Architecture Decisions And Limitations

The detailed implementation description is in [`docs/doc.md`](docs/doc.md). This file records cross-cutting decisions and known limitations confirmed in the current code.

## Decisions

- Agents initiate an outbound WebSocket and receive complete desired-state snapshots; the server never connects inbound and never substitutes event replay for a reconnect snapshot.
- The agent caches desired state before applying it and can reconcile without a server connection.
- Durable control-plane state and agent reports live in Postgres through sqlc; active WebSocket connections remain process-local.
- Email/password and optional GitHub/Google OAuth issue the same 24-hour HS256 JWT. Non-browser clients use bearer tokens; the browser uses an httpOnly same-origin cookie for REST, SSR guards, and project WebSockets. Every new authentication verifies that the subject is still active.
- OAuth identities are keyed by provider and provider user ID. Provider email is metadata and is not trusted for implicit account linking.
- Agent/server tunnel messages use protobuf; frontend REST and WebSocket shapes do not.
- Alert evaluation is separate from report ingestion, metrics exposition, and future notification delivery.

## Known Limitations

- **Single server instance:** agent sessions, desired-state push routing, frontend subscriptions, and alert debounce state are process-local, so multiple instances can miss pushes and notifications without cross-instance coordination.
- **Duplicate agent sessions:** registering a newer connection replaces the hub entry but does not close the older socket, so both connections can continue reporting until the older one exits.
- **OAuth provider linking:** attaching another provider to an already-authenticated user is deferred because it requires an authenticated, CSRF-protected linking flow; separate providers currently create separate users unless the exact provider identity already exists.
- **Login timing side channel:** unknown, deleted, and OAuth-only users skip bcrypt comparison while password users do not, so timing can reveal whether an active password credential exists.
- **Duplicate authentication header lines:** REST auth uses `Header.Get("Authorization")` and project WebSocket parsing ultimately uses one combined `Sec-WebSocket-Protocol` value, so duplicate field lines are not explicitly rejected and may be interpreted differently by intermediaries.
- **Soft-delete scope:** deletion revokes future user JWT authentications but does not close existing project WebSockets, revoke agent tokens, remove owned resources, or make email/OAuth identities reusable.
- **Primary major-version changes:** replacement preserves the existing `PGDATA` volume without `pg_upgrade` or dump/restore, so a PostgreSQL major-version change is not a supported safe upgrade despite the emitted replacement action.
- **Primary replacement dependency ordering:** a failed primary replacement does not block subsequent replica deletes, so replicas can be stopped or removed while their recreate actions are skipped.
- **Extension action staleness:** a primary replacement and extension change in the same diff emits an extension action containing the removed primary's container ID; a later extension-only pass may converge, but the first pass reports a stale failure.
- **Replica cleanup dependency:** removing replica data and slots executes through the primary, so cleanup can remain blocked after the replica is stopped when the primary is absent or unusable.
- **Backup schedule restart behavior:** schedule timestamps are in memory and tickers wait a full interval, so restarting the agent resets each interval and can delay the next backup by up to that interval.
- **Alert delivery and debounce:** rules are evaluated and state transitions are stored, but there is no alert API, UI, or notification sink, and restarting an evaluator resets an in-progress duration-before-firing window.
- **Backup image trust:** missing `orca-postgres:<version>` images are built automatically, but existing tags are not inspected and builds require Docker and external parent/package access.
- **PITR exposure and coordination:** recovery is only an internal package function and does not coordinate replicas, PgBouncer, or the reconciliation/backup operation gate; a production entry point is deferred until those safety boundaries are designed.
- **PITR failure cleanup:** failures after restore begins intentionally leave the primary stopped, and a process crash can leave a temporary restore container that ordinary observation does not classify for cleanup, because automatic restart could expose partially restored data.
- **Read-only web product:** beyond login and signup, the UI has no host, mutation, backup, extension, PITR, alert, or persisted-layout workflow; implementing controls before their complete persistence contracts would overstate product behavior.
- **Development packaging:** there is no checked-in agent Dockerfile, Compose deployment, or frontend/API proxy, so local development currently requires source execution and external same-origin web routing.
- **Automated coverage:** there are no committed Go or frontend tests, so reconciler resource paths, auth/store/API behavior, backup/PITR behavior, and WebSocket concurrency have no behavioral or race-test safety net.
