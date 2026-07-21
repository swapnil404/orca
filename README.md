# Orca

Orca is a self-hosted Postgres orchestration and control platform. You run a small agent on your own server. The agent connects out to Orca's control plane over an encrypted tunnel, and from there you manage your Postgres infrastructure through a web UI: clusters, replicas, connection pooling, backups, and extensions.

Orca never runs your database. Your data stays on your own infrastructure. Orca stores what you want your infrastructure to look like, pushes that down to the agent, and the agent does the actual work through Docker.

## Why this exists

Managed Postgres works fine until you can't use it: compliance requirements, data residency, cost at scale, or wanting your database on hardware you control. Orca gives you the control plane experience, a canvas UI, one click to add a replica, live topology and health, without handing your data to a third party.

## How it works

1. You add a host from the UI. This generates a token and a `docker run` command.
2. You run that command on your server. The agent starts, authenticates with the token, and opens an outbound connection to Orca's server. Nothing on your server needs to accept inbound traffic.
3. You configure a Postgres cluster in the UI: primary, replicas, connection pooling, backup schedule, extensions. That configuration is stored as desired state and pushed down to your agent over the same connection.
4. The agent compares desired state against what's actually running in Docker and reconciles the difference: starting, updating, or removing containers as needed.
5. The agent reports back what's actually running, along with health metrics. The UI reflects it in real time.

If the connection between your agent and Orca drops and comes back, the agent doesn't try to replay everything it missed. Orca sends the full current desired state again, and the agent reconciles against that. This keeps the system correct after long disconnects, without relying on an event log staying intact.

## What you can run

- **Primary and replicas.** Streaming replication, add or remove replicas from the canvas.
- **Connection pooling.** PgBouncer, per-database or standalone pools.
- **Backups.** pgBackRest, full, differential, and WAL backups, with point-in-time recovery.
- **Extensions.** Enable common extensions (pgvector, PostGIS, TimescaleDB, pg_partman, and others) per cluster from the UI.
- **Health and metrics.** Live status per node in the canvas, backed by Prometheus-compatible metrics and alerting on your infrastructure.

## Requirements

- A server you control, running Ubuntu (or any Linux distro with Docker support)
- Docker installed and running on that server
- Root or sudo access to run the agent container, since it needs access to the Docker socket
- An Orca account

## Quickstart

1. **Create an account and a project** at the Orca web UI.
2. **Click "Add host."** You'll get back a `docker run` command with a token baked in, something like:

   ```
   docker run -d \
     -e ORCA_TOKEN=your_token_here \
     -e ORCA_SERVER_URL=wss://your-orca-server.example/agent \
     -v /var/run/docker.sock:/var/run/docker.sock \
     -v /var/orca/data:/var/orca/data \
     orca/agent
   ```

3. **Run that command on your server.** The agent starts, connects to Orca, and your host shows up as live in the UI within a few seconds. You don't need to open any inbound ports, the agent only connects out.
4. **Add a Postgres cluster** from the canvas. Set a name, version, and config, then confirm.
5. **Watch it come up.** The agent pulls the Postgres image, starts the container, and the node in the canvas goes from pending to healthy once it's up.
6. **Add replicas, pooling, backups, or extensions** the same way, configure it in the UI, the agent reconciles it on your host.

## What Orca does and doesn't do

- Orca stores your desired configuration and your infrastructure's reported health. It does not store your Postgres data, and it does not run your database.
- If your server loses connection to Orca, your database keeps running. You just won't be able to push config changes until it reconnects.
- When the agent reconnects after being offline, it re-syncs against the full current desired state, not a queue of missed changes. Nothing gets replayed twice or lost.
- Removing a cluster, replica, or pool from the UI actually removes the corresponding container and data volume on your host. There's no separate confirmation step on the server, deletion in the UI is the source of truth.

## Project structure

```
orca/
├── agent/    # runs on your host, reconciles Docker state against desired state
├── server/   # control plane: API, WebSocket hub, desired state store, metrics
├── web/      # canvas UI
├── pkg/      # shared types between agent and server
├── proto/    # agent <-> server message definitions
├── deploy/   # docker compose for local dev
└── scripts/  # dev and migration scripts
```

## FAQ

**Do I need to expose any ports on my server?**
No. The agent connects out to Orca, Orca never connects in to your server.

**What happens to my database if Orca's servers go down?**
Your Postgres containers keep running exactly as they are. The agent just won't receive new config changes until Orca is reachable again.

**Can I run multiple hosts under one account?**
Yes. Each host runs its own agent and gets its own token. You can manage them all from the same project.

**What Postgres versions are supported?**
Check the version selector in the canvas for the current supported list.

**Can I use my own Postgres image instead of the default?**
Not yet, this is on the roadmap.

**Is there a CLI?**
Not yet, this is on the roadmap.

## Local development

See `docs/` for architecture and local setup instructions.

## License

TBD.
