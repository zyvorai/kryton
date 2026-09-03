# API reference

Kryton exposes a versioned REST API under `/api/v1`. All machine operations are scoped to a **project**. Authentication uses bearer tokens (`KRYTON_TOKEN`) or trusted proxy headers when configured.

OpenAPI spec: [`openapi.yaml`](../openapi.yaml).

---

## Endpoints

```text
GET    /api/v1                          # discovery catalog (public)
GET    /api/v1/me
GET    /api/v1/projects
GET    /api/v1/capabilities
GET    /api/v1/doctor
GET    /api/v1/storage
GET    /api/v1/storage/config
PUT    /api/v1/storage/config
GET    /api/v1/storage/setup
POST   /api/v1/storage/setup
GET    /api/v1/settings
PUT    /api/v1/settings
POST   /api/v1/settings/test
POST   /api/v1/integrations/atlas/test
GET    /api/v1/images
GET    /api/v1/golden
POST   /api/v1/golden
GET    /api/v1/golden/{id}
GET    /api/v1/golden/{id}/passport      # guestkit Cutover Passport JSON, 404 if none recorded
POST   /api/v1/golden/{id}/bootstrap
GET    /api/v1/jobs
GET    /api/v1/jobs/{id}
GET    /api/v1/summary?project=<project>
GET    /api/v1/machines?project=<project>&limit=<n>&cursor=<opaque>
POST   /api/v1/machines
GET    /api/v1/machines/{id}?project=<project>
GET    /api/v1/machines/{id}/console?project=<project>
GET    /api/v1/machines/{id}/vnc?project=<project>
POST   /api/v1/machines/{id}/start?project=<project>
POST   /api/v1/machines/{id}/stop?project=<project>
POST   /api/v1/machines/{id}/snapshot?project=<project>
GET    /api/v1/machines/{id}/snapshots?project=<project>
POST   /api/v1/machines/{id}/snapshots/{sid}/restore?project=<project>
DELETE /api/v1/machines/{id}/snapshots/{sid}?project=<project>
DELETE /api/v1/machines/{id}?project=<project>
GET    /api/v1/events
GET    /api/v1/events/stream
```

Public (no auth): `GET /api/v1` · `GET /openapi.yaml` · `GET /healthz` · `GET /readyz` · `GET /metrics`.

OpenAPI 3.1: [`openapi.yaml`](../openapi.yaml) — also served live at `/openapi.yaml`.

---

## Integrating from other projects

Kryton is designed for Zyvor products (Axiom, Haven, automation) to drive Windows VMs over HTTP.

```bash
# 1. Discover the surface (no token required)
curl -sS "$KRYTON_URL/api/v1" | jq .

# 2. Fetch OpenAPI for codegen
curl -sS "$KRYTON_URL/openapi.yaml" -o kryton.openapi.yaml

# 3. Call authenticated APIs
curl -sS -H "Authorization: Bearer $KRYTON_TOKEN" \
  "$KRYTON_URL/api/v1/machines?project=default"
```

**CORS** — set `KRYTON_CORS_ORIGINS` to the calling app origins (comma-separated), or `*` in lab:

```bash
Environment=KRYTON_CORS_ORIGINS=https://axiom.example,https://haven.example
```

**Auth for service-to-service:**

| Mode | How |
|------|-----|
| `apikey` | `Authorization: Bearer <token>` (hash with `krytonctl hash-token`) |
| `proxy` | Edge sets `X-Kryton-User`, `X-Kryton-Role`, `X-Kryton-Projects`, `X-Kryton-Proxy-Secret` |
| `disabled` | Lab only (`KRYTON_ALLOW_INSECURE=true`) |

**Minimal create flow:**

```bash
curl -sS -X POST "$KRYTON_URL/api/v1/machines" \
  -H "Authorization: Bearer $KRYTON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "project": "default",
    "name": "win-app01",
    "image": "windows-11-enterprise",
    "compute": {"cpu": 4, "memoryMiB": 8192},
    "disk": {"sizeGiB": 80, "storageClass": "rook-ceph-block"}
  }'
```

Watch progress via `GET /api/v1/jobs` or `GET /api/v1/events/stream` (SSE).

CLI equivalent: `KRYTON_URL=… KRYTON_TOKEN=… krytonctl …`.

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

`GET /api/v1/machines?project=default` returns machines in the project, paginated:

| Param | Default | Notes |
|-------|---------|-------|
| `limit` | 50 | Max items per page, capped at 500 |
| `cursor` | *(none)* | Opaque value from the previous response's `nextCursor`; omit for the first page |

```json
{
  "items": [ { "id": "…", "…": "…" } ],
  "nextCursor": "MzY5Y2E3MzctOGJmYy00YTY0LWEyNGUtYzJhMzIwMTgyNzFh"
}
```

`nextCursor` is absent on the last page. Machines are ordered by ID for a stable sort across pages — the provider itself gives no ordering guarantee.

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
| List snapshots | GET | `/api/v1/machines/{id}/snapshots?project=…` |
| Restore snapshot | POST | `/api/v1/machines/{id}/snapshots/{sid}/restore?project=…` |
| Delete snapshot | DELETE | `/api/v1/machines/{id}/snapshots/{sid}?project=…` |
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
| `io.kryton.snapshot.created` | Snapshot requested |
| `io.kryton.snapshot.restored` | Restore requested |
| `io.kryton.snapshot.deleted` | Snapshot deleted |

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

Common codes: `INVALID_REQUEST` · `NOT_FOUND` · `FORBIDDEN` · `CONFLICT` · `INTERNAL_ERROR` · `RATE_LIMITED`.

---

## Rate limiting

Set `KRYTON_RATE_LIMIT_RPS` (and optionally `KRYTON_RATE_LIMIT_BURST`) to cap requests per caller — a token bucket keyed by API-key name (or remote address when auth is disabled). Disabled by default (`KRYTON_RATE_LIMIT_RPS=0`). Exceeding the limit returns `429` with `error.code: "RATE_LIMITED"`; back off and retry.

---

## Authentication

**API key mode** — send `Authorization: Bearer <token>`.

**Proxy mode** — the reverse proxy sets:

- `X-Kryton-User`
- `X-Kryton-Role` (`viewer` · `operator` · `admin`)
- `X-Kryton-Projects` (comma-separated)
- `X-Kryton-Proxy-Secret` (shared secret)

Roles are always intersected with project scope.
