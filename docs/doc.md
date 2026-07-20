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

## The reconnection rule and split-brain avoidance

This is the correctness property the rest of the system depends on. If the agent's connection to the server drops, the agent continues operating from its local cache of the last known desired state. It does not require the server to be reachable to keep running Postgres correctly, it simply stops receiving new configuration changes until the connection is restored.

When the connection is restored, the server does not attempt to replay a log of what changed while the agent was offline. It sends the full current desired state, as it exists right now, in one message. The agent reconciles against that as it would any other desired state update.

This avoids split-brain scenarios where the agent and server disagree about history. There is no history to disagree about, only a current desired state and a current actual state, reconciled on every pass. An agent that has been offline for five minutes and an agent that has been offline for five days go through the exact same reconnection path.

## Data model

**Desired state** describes what the user wants to exist: which clusters, their Postgres version and configuration, their replicas, and whether connection pooling or backups are configured. It is owned by the server and pushed to agents.

**Actual state** describes what is really running, as reported by the agent after querying Docker directly. It is owned by the agent and reported to the server.

The server never assumes actual state without a report from the agent confirming it. An endpoint that would otherwise need to guess or fake a status must instead reflect only what has actually been confirmed.

## Container and volume conventions

Postgres-related containers run as siblings to the agent container, not nested inside it, communicating through the Docker socket mounted from the host.

- Containers are namespaced by cluster ID: `orca-<cluster-id>-primary`, `orca-<cluster-id>-replica-<n>`, `orca-<cluster-id>-pgbouncer`, `orca-<cluster-id>-pgbackrest`.
- Data volumes are named and explicit, mounted at `/var/orca/data/<cluster-id>/` on the host. Anonymous volumes are not used, since named volumes are what allow data to survive container restarts and agent upgrades.

## Server internals

The server is stateless with respect to its own process, all durable state lives in its own Postgres metadata database, accessed through a `sqlc`-generated typed query layer. Multiple server instances can run behind a load balancer without coordination between them, since coordination state (which host is connected to which instance) lives in the WebSocket hub of whichever instance holds that connection, not in a shared in-memory structure.

The WebSocket hub maintains a map from host ID to the active session for that host. It must be safe under concurrent reads and writes, since sessions connect, disconnect, and receive pushes from different goroutines simultaneously.

## Frontend

The web UI holds its own WebSocket connection to the server, separate from the agent-server tunnel, used to receive real-time topology and health updates and reflect them in the canvas. All outbound requests from the frontend, configuration changes, host registration, and so on, go through the REST API, not through this connection.

## What proto is for, and isn't

`proto/` defines the message contract exclusively between agent and server, sent over the WebSocket tunnel. It is not used for REST request or response shapes, those are defined and validated within the server's own API layer. Keeping this boundary clean avoids the tunnel contract growing to accommodate concerns that only the frontend needs.

## Non-goals for the current architecture

- The server does not, and should not, ever initiate a connection to a user's host.
- The agent does not persist a log of past desired states or replay history on reconnect.
- `pkg/` contains shared types only. Business logic belongs in the module that owns the relevant data, not in the shared package.
