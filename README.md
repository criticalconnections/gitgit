<p align="center"><b>GitGit</b> — Code together. Ship further.</p>

# GitGit

GitGit is a self-hosted software forge built to compete head-on with GitHub
and GitLab: git hosting over HTTP, pull requests with code review, **stacked
PRs**, issues, labels, webhooks, tags, downloadable archives, and a built-in
CI runner — one Go binary with a React + shadcn/ui frontend embedded, SQLite
for state, bare repositories on disk. No external services.

## Features

- **Git over smart HTTP** — `git clone` / `git push` with stock git.
  Authenticate with your password or a personal access token. Zip / tar.gz
  archive downloads at `/{owner}/{repo}/archive/{ref}.zip`.
- **Pull requests** — three-dot diffs, per-line review comments, approvals /
  request-changes, shared issue/PR numbering, base retargeting.
- **Merge strategies** — merge commit, squash, or rebase, all performed
  server-side in the bare repo via `git merge-tree` (git ≥ 2.38). Rebase
  drops already-applied commits like `git rebase --empty=drop`.
- **Stacked pull requests** — open a PR whose base is another PR's head
  branch. The PR page and the *Stacks* tab visualize the dependency chain;
  when the bottom PR merges, children automatically retarget. "Update
  branch" and "Restack" keep stacks fresh server-side.
- **Built-in CI** — jobs in `.gitgit/ci.yml` run on every push in a clean
  clone with live logs, per-job status, re-runs, and commit badges. Branch
  protection can require green CI and N approvals, enforced identically in
  the UI and the API.
- **Preview Environments** — one click on a PR spins up a real, running
  instance of that branch on its own subdomain, with a **QR code** to open it
  on your phone. Declare how the app builds and runs in `.gitgit/preview.yml`
  and GitGit clones the branch, builds it, starts the process, and proxies
  `https://{id}.preview.example.com` to it. Environments follow the branch,
  rebuild on push, and are torn down on merge, close, idle, or TTL. Without a
  `run:` command a preview stays static — the branch's files served straight
  from the tree, no build required. See
  [Preview Environments](#preview-environments) below.
- **Import from GitHub** — mirror any repository with every branch and tag,
  and optionally its issues and labels. Works with private repos via a token,
  and with GitHub Enterprise or any other git host.
- **Issues, labels, webhooks, stars, collaborators** with read/write/admin
  roles and private repositories.
- **JSON API** under `/api/v1` — repos, branches, tags, tree/blob/commits,
  pulls (create/review/merge), issues, CI runs. Authenticate with
  `Authorization: token gg_…` or HTTP Basic.
- **Modern UI** — React 19 + Tailwind v4 + shadcn/ui (Radix primitives),
  light/dark themes, served by the binary itself.

## Architecture

```
┌────────────────────────────── one Go binary ──────────────────────────────┐
│  React SPA (embedded)   JSON API (/api/v1)   git smart HTTP   CI runner   │
│          └── shadcn/ui         └── SQLite (WAL)    └── bare repos on disk │
└───────────────────────────────────────────────────────────────────────────┘
```

- All git operations shell out to the system `git` binary — merges, rebases,
  and cherry-picks happen with plumbing commands directly in the bare repo
  (no worktrees).
- Object reads go through a persistent `git cat-file --batch-command`
  process per repository — a design adapted from **Gitea**'s git engine
  (MIT; see `NOTICE`). The vendored Gitea source used as reference lives in
  `./gitea-main`.
- Pushes are detected by diffing ref snapshots around `receive-pack`, which
  drives PR updates, CI triggering, and webhooks.
- Branch previews serve files straight from the bare repo at a commit and set
  a `Content-Security-Policy: sandbox …` (no `allow-same-origin`), so
  previewed content lives in an opaque origin: its JS cannot read GitGit's
  cookies, and its `fetch` carries `Origin: null`, which the API's
  same-origin guard rejects. QR codes are rendered server-side as PNGs.

## Quick start

Requirements: `git` ≥ 2.38 and (to build) Go ≥ 1.24 + Node ≥ 20.

```bash
cd web && npm ci && npm run build && cd ..   # build the frontend once
go build -o gitgit .
./gitgit -addr :3000 -data ./data
```

Open http://localhost:3000 — **the first account registered becomes the site
admin**. Create a repository, then:

```bash
git remote add origin http://localhost:3000/you/yourrepo.git
git push -u origin main
```

### Docker

```bash
docker build -t gitgit .
docker run -p 3000:3000 -v gitgit-data:/data gitgit
```

### Publishing it (Cloudflare Tunnel)

GitGit needs subprocesses and a filesystem, so it runs on your own hardware or
a VM — Cloudflare provides the edge rather than the runtime. A tunnel gives it
a real hostname and TLS with no open inbound ports:

```bash
cloudflared tunnel create gitgit
cloudflared tunnel route dns gitgit git.example.com
cloudflared tunnel --config deploy/cloudflared.yml run gitgit
```

See [deploy/README.md](deploy/README.md) for the full walkthrough, including
hardening before exposure and why Cloudflare Access needs path scoping to
avoid breaking `git clone`.

### Flags / environment

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `-addr` | `GITGIT_ADDR` | `:3000` | listen address |
| `-data` | `GITGIT_DATA` | `./data` | repos, SQLite DB, CI workspaces |
| `-base-url` | `GITGIT_BASE_URL` | derived | external URL for clone instructions |
| `-open-registration` | `GITGIT_OPEN_REGISTRATION` | `true` | allow anyone to sign up; set `false` when internet-facing (the first account can still bootstrap the admin) |
| `-ci-workers` | — | `2` | concurrent CI runners |

## CI configuration

Commit `.gitgit/ci.yml` (or `.ci.yml`):

```yaml
jobs:
  test:
    steps:
      - name: unit tests
        run: go test ./...
    env:
      CGO_ENABLED: "0"
    timeout_minutes: 15
```

Steps run with `bash -e -o pipefail` in a fresh clone at the pushed commit,
with `CI`, `GITGIT_SHA`, `GITGIT_REF`, `GITGIT_EVENT`, `GITGIT_REPO`, and
`GITGIT_RUN_NUMBER` set. CI executes repository code on the host — only host
repositories you trust, or run the server in a container.

## Importing from GitHub

**Import** in the top bar (or `/import`) brings a repository over. Paste
`owner/repo` or any URL — GitHub Enterprise and other git hosts work too.

Git data is mirrored with `git clone --mirror`, so **every branch and tag**
comes across in one pass, and the source's default branch is adopted. The
`origin` remote is then removed, so an imported repository can never push back
to where it came from.

Optionally tick **Import issues** to copy issues, their labels, and their
open/closed state through the GitHub API. Pull requests are imported *as
issues*: their head branches may not exist here, and a pull request that
cannot be merged or reviewed is worse than an honest record. Every imported
item keeps a header crediting the original author and linking back.

Private repositories need a GitHub token with `repo` scope, which is also
worth supplying for issue imports — unauthenticated API calls are limited to
60 per hour. The token is used for that one import and never stored.

Imports run in the background with a live log, since a large mirror takes
minutes.

```bash
curl -u you:token -X POST https://git.example.com/api/v1/import \
  -d '{"source":"octocat/Hello-World","import_issues":true}'
curl -u you:token https://git.example.com/api/v1/import/1   # progress
```

Known limits: issue import stops at 1000 issues, and because job progress is
held in memory, restarting the server during an import orphans the job (the
repository may be left partially mirrored — delete it and re-import).

## Preview Environments

A Preview Environment is an ephemeral, running instance of a branch. Add
`.gitgit/preview.yml` to the repository:

```yaml
build:                    # optional, runs once before `run`
  - npm ci
  - npm run build
run: npm start            # long-lived server; omit for a static preview
static: dist              # directory served when `run` is absent
health_path: /
ttl_minutes: 120          # hard lifetime (default 2h)
idle_minutes: 30          # stopped after this long without a request
env:
  NODE_ENV: production
```

Your process is started with **`$PORT`** set — bind to it, on any interface —
plus `GITGIT_REPO`, `GITGIT_REF`, `GITGIT_SHA`, and `GITGIT_PREVIEW_URL`.
GitGit waits for the port to accept connections before routing traffic, and
serves a holding page (which auto-refreshes) until then.

**Why subdomains.** Each environment is served from its own origin, so
absolute asset paths (`/assets/app.js`), client-side routers, cookies, and
`localStorage` behave exactly as they will in production. Serving under a path
prefix on the forge's own origin breaks all four.

Enable it by pointing GitGit at a wildcard domain:

```bash
./gitgit -preview-domain preview.example.com
```

This requires `*.preview.example.com` in DNS **and** TLS coverage for it. Note
that Cloudflare's free Universal SSL only covers `example.com` and
`*.example.com` — a *second-level* wildcard needs Advanced Certificate Manager
or Total TLS. Using a single-level pattern avoids that cost entirely.

Without `-preview-domain`, previews are served at `/p/{token}/` on the main
host instead, under a strict opaque-origin sandbox. That is fine for static
content and local development, but cannot host a running app.

### Isolation

Because environments are siblings of the forge's own domain:

- the session cookie uses the **`__Host-`** prefix over HTTPS, which browsers
  refuse to accept alongside a `Domain` attribute — so a preview cannot set or
  shadow it
- the proxy strips `Cookie` before forwarding a request into an environment
- the API's same-origin guard rejects mutations originating from a preview
- path-served previews keep the sandbox CSP; subdomain-served ones rely on
  origin separation instead, so real apps are not crippled

Environments run repository code, exactly like CI. Run GitGit in a container
or a dedicated VM if you do not fully trust everyone who can push.

## Stacked PR workflow

```bash
git checkout -b feat-parser && … && git push origin feat-parser
# open PR: feat-parser → main
git checkout -b feat-formatter && … && git push origin feat-formatter
# open PR: feat-formatter → feat-parser   ← stacked!
```

Merge the bottom PR: GitGit retargets `feat-formatter` to `main`
automatically and posts a system comment. *Restack* rebases a child onto its
new base server-side, dropping commits that already landed.

## API sketch

```bash
TOKEN=gg_…   # Settings → Personal access tokens
curl -H "Authorization: token $TOKEN" http://host/api/v1/user
curl -H "Authorization: token $TOKEN" -X POST http://host/api/v1/repos \
     -d '{"name":"demo","auto_init":true}'
curl -H "Authorization: token $TOKEN" -X POST http://host/api/v1/repos/you/demo/pulls \
     -d '{"title":"Feature","base":"main","head":"feature"}'
curl -H "Authorization: token $TOKEN" -X POST http://host/api/v1/repos/you/demo/pulls/1/merge \
     -d '{"strategy":"squash","delete_branch":true}'
```

Merge gating (required CI, approvals, conflicts) is enforced on the API
exactly as in the UI. Cookie-authenticated mutations are CSRF-protected via
`Sec-Fetch-Site`/`Origin` checks; token clients are unaffected.

## Development

```bash
./gitgit -addr :3000 -data ./data     # backend
cd web && npm run dev                  # Vite dev server on :5173, proxies /api + git
go test ./...                          # diff parser + merge strategy tests
```

## License notes

GitGit adapts specific designs from [Gitea](https://github.com/go-gitea/gitea)
(MIT) — see `NOTICE`. The vendored reference tree is in `./gitea-main`.
