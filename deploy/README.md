# Deploying GitGit behind Cloudflare

GitGit is a single Go binary that shells out to `git` and runs CI steps with
`bash`. It needs a real OS with a filesystem and subprocesses, so it runs on a
VM, a container, or your own hardware — **not** on Cloudflare Workers, which
execute JavaScript/WASM in an isolate with no subprocesses or persistent disk.

The Cloudflare piece is therefore the **edge**: a Cloudflare Tunnel publishes
the origin at a real hostname with TLS, no open inbound ports, and no public
IP. Cloudflare's WAF, caching, and (optionally) Access sit in front.

```
   phone / laptop / git CLI
            │  https://git.example.com
            ▼
   ┌──────────────────────┐
   │  Cloudflare edge     │  TLS, WAF, DDoS, caching
   └──────────┬───────────┘
              │  outbound-only QUIC tunnel
   ┌──────────▼───────────┐
   │  cloudflared         │  same host (or sidecar)
   ├──────────────────────┤
   │  gitgit  :3000       │  binds 127.0.0.1 only
   │   ├─ bare git repos  │
   │   ├─ SQLite (WAL)    │
   │   └─ CI runner       │
   └──────────────────────┘
```

## 1. Harden before you expose

Public exposure means anyone who can reach the hostname can register — and CI
**executes repository code on the host**. Two things are strongly recommended
before the tunnel goes live:

```bash
# close signups (the first account can still be created to bootstrap the admin)
export GITGIT_OPEN_REGISTRATION=false

# bind to loopback so the box is only reachable through the tunnel
./gitgit -addr 127.0.0.1:3000 -data /var/lib/gitgit
```

Run the whole thing in a container (see the repo `Dockerfile`) if you want CI
jobs isolated from the host filesystem.

## 2. Create the tunnel

```bash
cloudflared tunnel login                 # once per machine, per zone
cloudflared tunnel create gitgit         # prints a UUID and writes <UUID>.json
deploy/route-dns.sh git.example.com      # guarded; see the warning below
```

> **Do not use `cloudflared tunnel route dns` directly on a machine that has
> other tunnels.** It has two footguns that combine badly:
>
> 1. `cloudflared tunnel login` prints an error but **exits 0** when a
>    `cert.pem` already exists, so `login && route dns` runs anyway with the
>    old credentials.
> 2. `route dns` **ignores the tunnel-name argument**, taking the tunnel from
>    `~/.cloudflared/config.yml`, and resolves the hostname relative to
>    whichever zone `cert.pem` is scoped to. Asking for `git.example.com`
>    while the cert is scoped to `other.com` does not fail — it creates
>    `git.example.com.other.com` on the wrong zone, pointing at the wrong
>    tunnel.
>
> `deploy/route-dns.sh` checks the cert's zone first, refuses on a mismatch
> with instructions, and writes the record through the API so the global
> config cannot hijack the target.

Fill the UUID, credentials path, and hostname into
[`deploy/cloudflared.yml`](cloudflared.yml).

> **Note:** if this machine already runs other tunnels, keep using the
> `--config deploy/cloudflared.yml` flag shown below. Editing the global
> `~/.cloudflared/config.yml` would disturb them.

## 3. Run

```bash
GITGIT_BASE_URL=https://git.example.com \
  ./gitgit -addr 127.0.0.1:3000 -data /var/lib/gitgit &

cloudflared tunnel --config deploy/cloudflared.yml run gitgit
```

`GITGIT_BASE_URL` matters: it's what clone instructions show and what branch
preview QR codes encode. Without it GitGit falls back to the request host,
which is usually right but can be wrong behind proxies.

Install as services with `cloudflared service install` (Linux/macOS) plus a
systemd unit or launchd plist for the binary itself.

## 4. Verify

```bash
curl -sS https://git.example.com/api/v1                        # {"name":"gitgit",...}
git clone https://git.example.com/you/yourrepo.git             # smart HTTP works
curl -sSI https://git.example.com/ | grep -i x-frame-options   # DENY
```

## Preview Environments (wildcard subdomains)

Preview Environments each get their own hostname, so the tunnel needs a
wildcard route and the zone needs matching TLS.

**1. Wildcard DNS** — point `*.preview.example.com` at the tunnel:

```bash
deploy/route-dns.sh '*.preview.example.com'
```

**2. TLS — the part that surprises people.** Cloudflare's free Universal SSL
certificate covers exactly two names: `example.com` and `*.example.com`. A
wildcard one level deeper (`*.preview.example.com`) is **not** covered, and
requests to it fail the TLS handshake with `no peer certificate available`
before they ever reach your origin. To use a depth-2 wildcard you need
**Advanced Certificate Manager** (SSL/TLS → Edge Certificates → Order Advanced
Certificate, hostnames `example.com`, `*.example.com`,
`*.preview.example.com`) or **Total TLS**.

Wildcards can only be validated over DNS, never HTTP. On a zone using
Cloudflare nameservers the `_acme-challenge` TXT records are published
automatically; issuance usually completes in 10–15 minutes. Verify with:

```bash
echo | openssl s_client -connect example.com:443 \
  -servername probe.preview.example.com 2>/dev/null | openssl x509 -noout -subject
```

**Free alternative:** keep previews at a single level — `-preview-domain
example.com` serves them as `{id}.example.com`, which the Universal
certificate already covers. Be aware that every one-label hostname then looks
like a preview token, so reserve names like `www` accordingly.

**3. Tunnel ingress** — add the wildcard above the main hostname:

```yaml
ingress:
  - hostname: "*.preview.example.com"
    service: http://localhost:3000
  - hostname: example.com
    service: http://localhost:3000
  - service: http_status:404
```

**4. Run GitGit with the domain configured:**

```bash
GITGIT_BASE_URL=https://example.com \
GITGIT_PREVIEW_DOMAIN=preview.example.com \
  ./gitgit -addr 127.0.0.1:3000 -open-registration=false
```

Environments execute repository code on the host. Prefer a container or an
isolated VM, and note the concurrency cap (4 live environments) plus the TTL
and idle timeouts in `.gitgit/preview.yml`.

## Cloudflare Access — read this first

Putting Zero Trust Access in front of the whole hostname will **break
`git clone`/`git push` and the JSON API**, because Access expects an
interactive browser SSO redirect and git speaks plain HTTP Basic.

If you want Access, scope it to the web UI and leave the machine paths open to
their own authentication (GitGit already requires a password or a personal
access token on those routes):

- Protect: `git.example.com/settings*`, `git.example.com/dashboard*`
- Bypass: `git.example.com/*.git/*`, `git.example.com/api/*`

Alternatively issue Access **service tokens** and have automation send
`CF-Access-Client-Id` / `CF-Access-Client-Secret` headers.

Branch previews (`/p/{token}/…`) are unauthenticated capability URLs by
design — that is what makes the QR hand-off to a phone work. If previews must
stay internal, leave that path behind Access too, but expect phones off the
network to be prompted to log in.

## Other Cloudflare services

| Service | Fit |
|---|---|
| **Tunnel** | ✅ the integration above — publishes the origin |
| **WAF / rate limiting** | ✅ front the login and register endpoints |
| **R2** | ✅ good target for `data/` backups (repos + SQLite) |
| **Workers / Pages** | ❌ cannot host the backend: no subprocesses, no `git`, no persistent FS |
| **D1** | ❌ GitGit uses local SQLite in WAL mode next to the repos, not over HTTP |

A Worker *can* usefully sit alongside GitGit — for example receiving its
webhooks (`X-GitGit-Signature`, HMAC-SHA256) and fanning out to Slack — but it
cannot be the forge itself.
