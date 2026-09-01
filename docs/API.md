# API quick reference

All machine operations are scoped to a project.

```text
GET    /api/v1/projects
GET    /api/v1/capabilities
GET    /api/v1/images
GET    /api/v1/summary?project=default
GET    /api/v1/machines?project=default
POST   /api/v1/machines
GET    /api/v1/machines/{id}?project=default
POST   /api/v1/machines/{id}/start?project=default
POST   /api/v1/machines/{id}/stop?project=default
POST   /api/v1/machines/{id}/snapshot?project=default
DELETE /api/v1/machines/{id}?project=default
GET    /api/v1/events
GET    /api/v1/events/stream
```

Create request:

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

Error response:

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "name must be a DNS-style label up to 63 characters",
    "requestId": "..."
  }
}
```
