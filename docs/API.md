# API reference

Kryton exposes a versioned REST API under `/api/v1`. All machine operations are scoped to a **project**. Authentication uses bearer tokens (`KRYTON_TOKEN`) or trusted proxy headers when configured.

OpenAPI spec: [`openapi.yaml`](../openapi.yaml).

---

## Endpoints

```text
GET    /api/v1/projects
GET    /api/v1/capabilities
GET    /api/v1/doctor
GET    /api/v1/images
GET    /api/v1/golden
POST   /api/v1/golden
GET    /api/v1/golden/{id}
POST   /api/v1/golden/{id}/bootstrap
GET    /api/v1/jobs
GET    /api/v1/jobs/{id}
GET    /api/v1/summary?project=<project>
GET    /api/v1/machines?project=<project>
POST   /api/v1/machines
GET    /api/v1/machines/{id}?project=<project>
POST   /api/v1/machines/{id}/start?project=<project>
POST   /api/v1/machines/{id}/stop?project=<project>
POST   /api/v1/machines/{id}/snapshot?project=<project>
DELETE /api/v1/machines/{id}?project=<project>
GET    /api/v1/events
GET    /api/v1/events/stream
```

Health probes (unauthenticated): `GET /healthz` · `GET /readyz` · `GET /metrics`.

---

## Projects

`GET /api/v1/projects` returns the configured project list and default project.

---

## Capabilities

`GET /api/v1/capabilities` reports what the active provider supports (start/stop, snapshots, console, etc.). Use this to build provider-aware UIs without hard-coding behavior.

---

## Doctor

`GET /api/v1/doctor` runs environment diagnostics and returns a structured report:

```json
{
  "healthy": true,
  "provider": "dockur",
  "findings": [
    {"check": "auth", "status": "pass", "message": "Auth mode apikey"},
    {"check": "kvm", "status": "pass", "message": "/dev/kvm is available"}
  ]
}
```

Finding `status` values: `pass` · `warn` · `fail`. CLI equivalent: `krytonctl doctor`.

---

## Images

`GET /api/v1/images` returns the catalog. Each entry includes minimum CPU/memory, disk size, and (for dockur) a `dockurVersion` mapping.

---

## Machines

### List and get

`GET /api/v1/machines?project=default` returns all machines in the project.

`GET /api/v1/machines/{id}?project=default` returns a single machine.

### Machine object

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Stable Kryton UUID |
| `project` | string | Project scope |
| `provider` | string | `demo` · `dockur` · `kubevirt` |
| `state` | string | Lifecycle state |
| `spec` | object | Desired configuration |
| `providerRef` | object | Provider-specific reference |
| `ipAddresses` | string[] | Guest IPs when known |
| `consoleUrl` | string | Web console URL (dockur/demo) |
| `progressPercent` | int | Install progress 0–100 |
| `message` | string | Human-readable status |
| `conditions` | array | Structured conditions |
| `createdAt` / `updatedAt` | timestamp | Audit timestamps |
| `expiresAt` | timestamp | TTL expiry when set |

### Create

`POST /api/v1/machines`

```json
{
  "project": "default",
  "name": "finance-win01",
  "image": "windows-server-2025",
  "compute": {"cpu": 4, "memoryMiB": 8192},
  "disk": {"sizeGiB": 80},
  "network": {"networkId": ""},
  "ttlMinutes": 0
}
```

Validate CPU and memory against the image catalog minimums. Names must be DNS-style labels (≤ 63 characters).

### Lifecycle actions

| Action | Method | Path |
|--------|--------|------|
| Start | POST | `/api/v1/machines/{id}/start?project=…` |
| Stop | POST | `/api/v1/machines/{id}/stop?project=…` |
| Snapshot | POST | `/api/v1/machines/{id}/snapshot?project=…` |
| Delete | DELETE | `/api/v1/machines/{id}?project=…` |

---

## Events

`GET /api/v1/events` returns recent CloudEvents history.

`GET /api/v1/events/stream` opens an SSE stream for live lifecycle updates.

Common event types:

| Type | When |
|------|------|
| `io.kryton.machine.created` | Machine record created |
| `io.kryton.machine.install.started` | Dockur install begins |
| `io.kryton.machine.started` | Machine running |
| `io.kryton.machine.stopped` | Machine stopped |
| `io.kryton.machine.deleted` | Machine removed |

---

## Errors

All errors use a consistent envelope:

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "name must be a DNS-style label up to 63 characters",
    "requestId": "..."
  }
}
```

Common codes: `INVALID_REQUEST` · `NOT_FOUND` · `FORBIDDEN` · `CONFLICT` · `INTERNAL_ERROR`.

---

## Authentication

**API key mode** — send `Authorization: Bearer <token>`.

**Proxy mode** — the reverse proxy sets:

- `X-Kryton-User`
- `X-Kryton-Role` (`viewer` · `operator` · `admin`)
- `X-Kryton-Projects` (comma-separated)
- `X-Kryton-Proxy-Secret` (shared secret)

Roles are always intersected with project scope.
