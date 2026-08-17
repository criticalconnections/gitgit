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
- **Branch previews** — one click on a PR or the code view spins up a live,
  sandboxed preview of the branch's tree at a capability URL (`/p/{token}/`)
  and shows a **QR code** to open it on your phone (auto-detecting your LAN
  address). The preview follows the branch — push again and the same link
  serves the new tip. Ideal for eyeballing a static site or built frontend
  before merging. Previews are opaque-origin sandboxed (previewed scripts run
  but cannot read GitGit's session or call its API), expire in 24h, and are
  revocable.
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

### Flags / environment

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `-addr` | `GITGIT_ADDR` | `:3000` | listen address |
| `-data` | `GITGIT_DATA` | `./data` | repos, SQLite DB, CI workspaces |
| `-base-url` | `GITGIT_BASE_URL` | derived | external URL for clone instructions |
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
