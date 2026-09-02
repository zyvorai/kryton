# Kryton ↔ Atlas integration

[Atlas](../../../atlas) (`../../atlas` from this repo) is Zyvor’s **storage control plane**.
Kryton is the **Windows virtualization** control plane. Together:

| Product | Owns |
|---------|------|
| **Atlas** | Ceph / NFS / ZFS inventory, StorageClasses, PVC lifecycle, ownership |
| **Kryton** | Windows VMs on KubeVirt, golden images, snapshots of VM disks |

## Configure from Kryton UI

**Settings → Integrations → Atlas storage control plane**

1. Enable Atlas integration  
2. Set **Atlas base URL** (e.g. `http://127.0.0.1:5110` or lab NodePort)  
3. Paste an Atlas **bearer JWT**  
4. **Save Atlas**, then **Test Atlas**

Owner product id defaults to `kryton` (recorded on Atlas volume bindings).

## Mint an Atlas service token

As Atlas admin:

```bash
curl -sS -X POST "$ATLAS_URL/api/atlas/v1/auth/tokens" \
  -H "Authorization: Bearer $ATLAS_ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"subject":"product.service.kryton","role":"operator","ttl_secs":2592000}'
```

Use the returned JWT as Kryton’s Atlas token.

## APIs

| Kryton | Purpose |
|--------|---------|
| `PUT /api/v1/settings` `{ "atlas": { ... } }` | Persist integration |
| `POST /api/v1/integrations/atlas/test` | Probe `/readyz`, version, `/storage-classes` |
| `GET /api/v1` | Discovery (includes Atlas integration route) |

Atlas product conventions: see Atlas [`docs/PRODUCTS.md`](../../../atlas/docs/PRODUCTS.md) — product id **`kryton`**.

## Ownership when creating storage for Windows VMs

When Kryton later provisions disks via Atlas (or Atlas tracks Kryton PVCs), use:

```json
{
  "owner": {
    "product": "kryton",
    "resource_type": "vm",
    "resource_id": "<machine-uuid>",
    "role": "data_disk"
  }
}
```

Today Kryton stamps `disk.storageClass` from Settings (and can prefer classes discovered through Atlas). Full create-volume-via-Atlas is a follow-up once Atlas owns the lab’s Rook pool end-to-end.

## CORS

If Atlas console or another origin calls Kryton, set:

```bash
KRYTON_CORS_ORIGINS=https://atlas.example,http://127.0.0.1:5110
```

## Related repos

```
kryton/   ← this repo
atlas/    ← ../../atlas (sibling checkout)
```
