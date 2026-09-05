# Authentication — how to get and use the API key

Kryton uses **bearer API keys** for lab and production (`KRYTON_AUTH_MODE=apikey`). The raw token is shown once (or stored in a local `lab.token` file); only a **SHA-256 digest** is kept in `keys.json`.

| Mode | When | Client sends |
|------|------|----------------|
| `disabled` | Local `make demo` only | Nothing (do not expose publicly) |
| `apikey` | Shared lab + production | `Authorization: Bearer <token>` |
| `proxy` | Behind SSO / reverse proxy | Trusted `X-Kryton-*` headers |

---

## Lab: create the key (recommended)

On the Kryton host (after [DEPLOY-REMOTE.md](DEPLOY-REMOTE.md) or a checkout with `krytonctl` on `PATH`):

```bash
cd ~/.deployments/kryton   # or your clone
./scripts/ensure-api-keys.sh
```

This writes:

| File | Contents |
|------|----------|
| `~/.kryton/lab.token` | **Raw bearer token** — paste in the UI / use as `KRYTON_TOKEN` |
| `~/.kryton/keys.json` | Hashed key file Kryton loads (`sha256`, role, projects) |

Print the key:

```bash
cat ~/.kryton/lab.token
# or from your laptop:
ssh <user>@<host> 'cat ~/.kryton/lab.token'
```

Rotate (new token + new hash):

```bash
KRYTON_ROTATE_KEYS=true ./scripts/ensure-api-keys.sh
# then point KRYTON_API_KEYS_FILE at the new keys.json and restart krytond
```

### Harden dockur / kubevirt lab units

```bash
./scripts/ensure-api-keys.sh
KRYTON_LAB_PUBLIC_HOST=<public-ip> ./scripts/harden-lab-services.sh
```

That enables **apikey** auth and usually **`KRYTON_LAB_AUTO_AUTH=true`**, so the browser can connect without pasting. CLI still needs the token from `~/.kryton/lab.token`.

---

## Lab: use the key

### Browser (Apple Store chapter login)

1. Open `http://<host>:<port>/` (demo unit often `:8080` or `:18080`; dockur lab often `:7088`).
2. If auto-auth is **off**, the login chapters appear → **Sign in** → paste the bearer token → **Sign in to Kryton**.
3. If auto-auth is **on**, the UI loads the token from `GET /api/v1/lab/bootstrap` (lab-only; requires `KRYTON_ALLOW_INSECURE=true`).
4. Settings → **Set session token** reopens the login gate to replace the token for this tab.
5. **Sign out** (top bar, or Settings → API access) clears this tab’s token and returns to the login chapters. With lab auto-auth, Sign out also skips auto-connect until you paste a key again (or clear `sessionStorage`).

Token storage: **sessionStorage for this browser tab only**.

### CLI

```bash
export KRYTON_URL=http://<host>:7088          # or :18080 / your port
export KRYTON_TOKEN=$(cat ~/.kryton/lab.token) # on the host
# from a laptop:
# export KRYTON_TOKEN=$(ssh <user>@<host> 'cat ~/.kryton/lab.token')

krytonctl doctor
krytonctl list
```

### curl

```bash
curl -sS -H "Authorization: Bearer $KRYTON_TOKEN" \
  "$KRYTON_URL/api/v1/me"
```

Public without a token: `GET /api/v1`, `/healthz`, `/readyz`, `/metrics`, `/openapi.yaml`.

---

## Manual key (no ensure script)

```bash
TOKEN=$(krytonctl generate-token)
HASH=$(krytonctl hash-token "$TOKEN")

sudo mkdir -p /etc/kryton
sudo tee /etc/kryton/keys.json >/dev/null <<EOF
{
  "keys": [
    {
      "name": "lab-admin",
      "sha256": "${HASH}",
      "role": "admin",
      "projects": ["*"]
    }
  ]
}
EOF
printf '%s\n' "$TOKEN" | sudo tee /etc/kryton/lab.token >/dev/null
sudo chmod 600 /etc/kryton/keys.json /etc/kryton/lab.token

# Point the unit at the file, then restart:
#   Environment=KRYTON_AUTH_MODE=apikey
#   Environment=KRYTON_API_KEYS_FILE=/etc/kryton/keys.json
sudo systemctl restart kryton   # or kryton-dockur / kryton-kubevirt
```

Read it later: `sudo cat /etc/kryton/lab.token`.

`keys.json` **must** be an object with a `"keys"` array (not a bare array).

---

## Production (Helm)

```bash
TOKEN=$(krytonctl generate-token)
HASH=$(krytonctl hash-token "$TOKEN")

cat > keys.json <<EOF
{
  "keys": [
    {
      "name": "ci-admin",
      "sha256": "${HASH}",
      "role": "admin",
      "projects": ["*"]
    }
  ]
}
EOF

# Save TOKEN in your secret manager — it is not stored in the cluster.
kubectl -n kryton create secret generic kryton-auth --from-file=keys.json
helm upgrade --install kryton ./deploy/helm/kryton \
  -n kryton --create-namespace \
  -f deploy/helm/kryton/values-customer.yaml
```

Use `export KRYTON_TOKEN=<saved raw token>` for `krytonctl` / CI. Chart details: [deploy/helm/kryton/README.md](../deploy/helm/kryton/README.md) · [DEPLOYMENT.md](DEPLOYMENT.md).

---

## Roles and projects

| Role | Typical use |
|------|-------------|
| `viewer` | Read-only |
| `operator` | Create / start / stop / snapshot |
| `admin` | Full control |

`projects` is a list of project names, or `["*"]` for all configured projects.

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Login page asks for a token | Auth is `apikey` and auto-auth is off — `cat ~/.kryton/lab.token` (or `/etc/kryton/lab.token`) and paste |
| `401` from API/CLI | Export `KRYTON_TOKEN` with the **raw** token, not the hash |
| `parse api keys file: cannot unmarshal array` | Use `{"keys":[…]}` wrapper, not a top-level JSON array |
| Auto-auth does nothing | Need `KRYTON_LAB_AUTO_AUTH=true`, `KRYTON_ALLOW_INSECURE=true`, `KRYTON_AUTH_MODE=apikey`, and `KRYTON_LAB_TOKEN_FILE` pointing at `lab.token` |
| Lost the raw token | Rotate with `KRYTON_ROTATE_KEYS=true ./scripts/ensure-api-keys.sh` (old token stops working after restart) |

---

## Related

- [USER-GUIDE.md](USER-GUIDE.md) — role-based walkthroughs  
- [DEPLOY-REMOTE.md](DEPLOY-REMOTE.md) — SSH lab install  
- [API.md](API.md) — REST contract  
- [DOCKUR.md](DOCKUR.md) — dockur lab + auto-auth  
- [DEPLOYMENT.md](DEPLOYMENT.md) — production Helm  
