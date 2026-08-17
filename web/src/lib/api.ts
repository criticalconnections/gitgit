// Typed client for the GitGit JSON API (/api/v1).
// This file is the single source of truth for backend payload shapes.

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
  })
  let data: unknown
  try {
    data = await res.json()
  } catch {
    data = {}
  }
  if (!res.ok) {
    const msg =
      typeof data === "object" && data !== null && "error" in data
        ? String((data as { error: string }).error)
        : `request failed (${res.status})`
    throw new ApiError(res.status, msg)
  }
  return data as T
}

export const get = <T,>(url: string) => request<T>("GET", url)
export const post = <T,>(url: string, body?: unknown) => request<T>("POST", url, body ?? {})
export const patch = <T,>(url: string, body: unknown) => request<T>("PATCH", url, body)
export const put = <T,>(url: string, body: unknown) => request<T>("PUT", url, body)
export const del = <T,>(url: string) => request<T>("DELETE", url)

// ---------- shapes ----------

export interface User {
  id: number
  username: string
  email: string
  full_name: string
  is_admin: boolean
  created_at: number
}

export interface UserRef {
  id: number
  username: string
  full_name?: string
}

export interface BranchRef {
  name: string
  sha: string
}

export interface Repo {
  id: number
  owner: string
  name: string
  full_name: string
  description: string
  default_branch: string
  private: boolean
  created_at: number
  can_write: boolean
  can_admin: boolean
  stars: number
  starred: boolean
  open_pulls: number
  open_issues: number
  empty: boolean
  allow_merge: boolean
  allow_squash: boolean
  allow_rebase: boolean
  delete_branch_on_merge: boolean
  require_ci_pass: boolean
  require_approvals: number
  // present on GET /repos/{o}/{r} only:
  clone_url?: string
  branches?: BranchRef[]
}

export interface Label {
  id: number
  name: string
  color: string
}

export interface Commit {
  sha: string
  short_sha: string
  author_name: string
  author_email: string
  when: number
  subject: string
  body: string
  parents: string[] | null
  ci_status?: CIStatus
  ci_run?: number
}

export type CIStatus = "queued" | "running" | "success" | "failure" | "error" | "timeout"

export interface DiffLine {
  op: " " | "+" | "-" | "\\"
  old: number
  new: number
  text: string
}

export interface DiffHunk {
  header: string
  lines: DiffLine[]
}

export interface DiffFile {
  old_path: string
  new_path: string
  path: string
  status: "modified" | "added" | "deleted" | "renamed"
  binary: boolean
  additions: number
  deletions: number
  truncated: boolean
  hunks: DiffHunk[]
}

export interface Diff {
  files: DiffFile[]
  stat: { files: number; additions: number; deletions: number }
}

export interface TreeEntry {
  name: string
  path: string
  type: "blob" | "tree" | "commit"
  size: number
  mode: string
}

export interface TreeResponse {
  empty: boolean
  ref?: string
  sha?: string
  path?: string
  entries: TreeEntry[]
  latest?: Commit
  ci_status?: CIStatus
  ci_run?: number
  commit_count?: number
  readme?: string
  readme_path?: string
}

export interface BlobResponse {
  ref: string
  sha: string
  path: string
  size: number
  binary: boolean
  truncated: boolean
  raw_url: string
  content?: string
  rendered?: string
}

export interface CommitsResponse {
  ref: string
  path: string
  page: number
  has_next: boolean
  commits: Commit[]
}

export interface CommitResponse {
  commit: Commit
  diff: Diff
  ci_status?: CIStatus
  ci_run?: number
}

export interface BranchRow {
  name: string
  sha: string
  when: number
  subject: string
  author_name: string
  default: boolean
  ahead?: number
  behind?: number
  pull?: number
}

export interface Tag {
  name: string
  sha: string
  when: number
  subject: string
}

export interface CompareResponse {
  base: string
  head: string
  commits?: Commit[]
  diff?: Diff
  ahead?: number
  behind?: number
  existing_pull?: number
}

export interface Pull {
  id: number
  number: number
  title: string
  body: string
  body_html: string
  state: "open" | "merged" | "closed"
  base: string
  head: string
  merge_commit: string
  created_at: number
  updated_at: number
  merged_at?: number
  merged_by?: UserRef
  author: UserRef
  ci_status?: CIStatus
  ci_run?: number
  comments: number
}

export interface MergeStateInfo {
  branches_ok: boolean
  base_sha: string
  head_sha: string
  ahead: number
  behind: number
  clean: boolean
  conflicts: string[] | null
  ci_status: string
  has_ci_config: boolean
  approvals: number
  changes_requested: number
  blockers: string[]
}

export interface StackItem {
  number: number
  title: string
  base: string
  head: string
  depth: number
  current?: boolean
  ci_status?: CIStatus
  author?: UserRef
}

export interface TimelineComment {
  type: "comment"
  id: number
  body: string
  body_html: string
  system: boolean
  created_at: number
  author: UserRef
}

export interface TimelineReview {
  type: "review"
  id: number
  state: "approved" | "changes_requested" | "commented"
  body: string
  body_html: string
  commit_sha: string
  created_at: number
  author: UserRef
}

export type TimelineItem = TimelineComment | TimelineReview

export interface PullDetail extends Pull {
  merge_state: MergeStateInfo
  stack: StackItem[]
  timeline: TimelineItem[]
  verdicts: { user: User; state: "approved" | "changes_requested" }[]
  review_comments: number
}

export interface ReviewComment {
  id: number
  file: string
  line: number
  side: "old" | "new"
  body: string
  body_html: string
  commit_sha: string
  created_at: number
  author: UserRef
}

export interface PullFilesResponse extends Diff {
  from_merge_commit: boolean
  comments: ReviewComment[]
}

export interface Comment {
  id: number
  body: string
  body_html: string
  system: boolean
  created_at: number
  author: UserRef
}

export interface Issue {
  id: number
  number: number
  title: string
  body: string
  body_html: string
  state: "open" | "closed"
  created_at: number
  updated_at: number
  author: UserRef
  labels: Label[]
  comments: number
}

export interface IssueDetail extends Issue {
  comment_list: Comment[]
  all_labels: Label[]
}

export interface CIJob {
  id: number
  name: string
  status: CIStatus
  exit_code: number
  log: string
  started_at?: number
  finished_at?: number
}

export interface CIRun {
  id: number
  number: number
  commit: string
  ref: string
  event: string
  status: CIStatus
  created_at: number
  started_at?: number
  finished_at?: number
  jobs?: CIJob[]
  commit_info?: Commit
}

export interface AccessToken {
  id: number
  name: string
  created_at: number
  last_used_at?: number
}

export interface Collaborator {
  user: User
  role: "read" | "write" | "admin"
}

export interface Webhook {
  id: number
  url: string
  events: string
  active: boolean
  has_secret: boolean
  created_at: number
}

export interface UserProfile {
  user: User
  repos: Repo[]
}

export type EnvStatus = "queued" | "building" | "running" | "failed" | "stopped" | "none"

export interface PreviewEnv {
  id: number
  status: EnvStatus
  commit: string
  ref: string
  message: string
  created_at: number
  started_at: number
  last_used_at: number
  expires_at: number
  log?: string
}

export interface Preview {
  id: number
  ref: string
  token: string
  path: string // "/p/{token}/"
  created_at: number
  expires_at: number
  hosts: string[]
  sha?: string
  /** dedicated hostname when a preview domain is configured */
  host?: string
  /** absolute URL of the Preview Environment */
  url?: string
  /** repo declares a `run:` command in .gitgit/preview.yml */
  runnable?: boolean
  env?: PreviewEnv
}

// ---------- endpoints ----------

const enc = encodeURIComponent

export const api = {
  // auth/session
  register: (username: string, email: string, password: string) =>
    post<User>("/api/v1/auth/register", { username, email, password }),
  login: (username: string, password: string) =>
    post<User>("/api/v1/auth/login", { username, password }),
  logout: () => post<{ ok: boolean }>("/api/v1/auth/logout"),
  me: () => get<User>("/api/v1/user"),
  updateProfile: (email: string, fullName: string) =>
    patch<User>("/api/v1/user", { email, fullName }),
  changePassword: (current: string, password: string) =>
    post<{ ok: boolean }>("/api/v1/user/password", { current, password }),
  listTokens: () => get<AccessToken[]>("/api/v1/user/tokens"),
  createToken: (name: string) => post<{ token: string; name: string }>("/api/v1/user/tokens", { name }),
  deleteToken: (id: number) => del<{ ok: boolean }>(`/api/v1/user/tokens/${id}`),

  // users + repos
  userProfile: (username: string) => get<UserProfile>(`/api/v1/users/${enc(username)}`),
  listRepos: (q?: string) => get<Repo[]>(`/api/v1/repos${q ? `?q=${enc(q)}` : ""}`),
  createRepo: (body: { name: string; description?: string; private?: boolean; auto_init?: boolean }) =>
    post<Repo>("/api/v1/repos", body),

  repo: (o: string, r: string) => get<Repo>(`/api/v1/repos/${enc(o)}/${enc(r)}`),
  updateRepo: (o: string, r: string, body: Record<string, unknown>) =>
    patch<Repo>(`/api/v1/repos/${enc(o)}/${enc(r)}`, body),
  deleteRepo: (o: string, r: string) => del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}`),
  star: (o: string, r: string, on: boolean) =>
    post<{ starred: boolean; stars: number }>(`/api/v1/repos/${enc(o)}/${enc(r)}/star`, { on }),

  // code
  tree: (o: string, r: string, ref?: string, path?: string) =>
    get<TreeResponse>(`/api/v1/repos/${enc(o)}/${enc(r)}/tree?ref=${enc(ref ?? "")}&path=${enc(path ?? "")}`),
  blob: (o: string, r: string, ref: string, path: string) =>
    get<BlobResponse>(`/api/v1/repos/${enc(o)}/${enc(r)}/blob?ref=${enc(ref)}&path=${enc(path)}`),
  commits: (o: string, r: string, ref?: string, path?: string, page?: number) =>
    get<CommitsResponse>(
      `/api/v1/repos/${enc(o)}/${enc(r)}/commits?ref=${enc(ref ?? "")}&path=${enc(path ?? "")}&page=${page ?? 1}`,
    ),
  commit: (o: string, r: string, sha: string) =>
    get<CommitResponse>(`/api/v1/repos/${enc(o)}/${enc(r)}/commit/${enc(sha)}`),
  branches: (o: string, r: string) => get<BranchRow[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/branches`),
  tags: (o: string, r: string) => get<Tag[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/tags`),
  deleteBranch: (o: string, r: string, name: string) =>
    del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/branches/${name}`),
  compare: (o: string, r: string, base: string, head: string) =>
    get<CompareResponse>(`/api/v1/repos/${enc(o)}/${enc(r)}/compare?base=${enc(base)}&head=${enc(head)}`),

  // pulls
  listPulls: (o: string, r: string, state: string) =>
    get<Pull[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls?state=${enc(state)}`),
  createPull: (o: string, r: string, body: { title: string; body: string; base: string; head: string }) =>
    post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls`, body),
  pull: (o: string, r: string, n: number | string) =>
    get<PullDetail>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}`),
  updatePull: (o: string, r: string, n: number, body: { title?: string; body?: string }) =>
    patch<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}`, body),
  pullFiles: (o: string, r: string, n: number | string) =>
    get<PullFilesResponse>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/files`),
  pullCommits: (o: string, r: string, n: number | string) =>
    get<Commit[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/commits`),
  pullComment: (o: string, r: string, n: number, body: string) =>
    post<Comment>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/comment`, { body }),
  pullReview: (o: string, r: string, n: number, verdict: string, body: string) =>
    post<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/review`, { verdict, body }),
  pullReviewComment: (o: string, r: string, n: number, file: string, line: number, side: string, body: string) =>
    post<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/review-comment`, { file, line, side, body }),
  mergePull: (o: string, r: string, n: number, body: { strategy: string; message?: string; delete_branch?: boolean }) =>
    post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/merge`, body),
  closePull: (o: string, r: string, n: number) => post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/close`),
  reopenPull: (o: string, r: string, n: number) => post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/reopen`),
  retargetPull: (o: string, r: string, n: number, base: string) =>
    post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/retarget`, { base }),
  updateBranch: (o: string, r: string, n: number) =>
    post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/update-branch`),
  rebaseBranch: (o: string, r: string, n: number) =>
    post<Pull>(`/api/v1/repos/${enc(o)}/${enc(r)}/pulls/${n}/rebase-branch`),
  stacks: (o: string, r: string) => get<StackItem[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/stacks`),

  // issues
  listIssues: (o: string, r: string, state: string) =>
    get<Issue[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues?state=${enc(state)}`),
  createIssue: (o: string, r: string, title: string, body: string) =>
    post<Issue>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues`, { title, body }),
  issue: (o: string, r: string, n: number | string) =>
    get<IssueDetail>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues/${n}`),
  updateIssue: (o: string, r: string, n: number, body: { title?: string; body?: string }) =>
    patch<Issue>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues/${n}`, body),
  issueComment: (o: string, r: string, n: number, body: string) =>
    post<Comment>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues/${n}/comment`, { body }),
  closeIssue: (o: string, r: string, n: number) => post<Issue>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues/${n}/close`),
  reopenIssue: (o: string, r: string, n: number) => post<Issue>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues/${n}/reopen`),
  setIssueLabels: (o: string, r: string, n: number, labels: number[]) =>
    put<Issue>(`/api/v1/repos/${enc(o)}/${enc(r)}/issues/${n}/labels`, { labels }),

  // labels / collaborators / webhooks
  listLabels: (o: string, r: string) => get<Label[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/labels`),
  createLabel: (o: string, r: string, name: string, color: string) =>
    post<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/labels`, { name, color }),
  deleteLabel: (o: string, r: string, id: number) =>
    del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/labels/${id}`),
  listCollaborators: (o: string, r: string) =>
    get<Collaborator[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/collaborators`),
  addCollaborator: (o: string, r: string, username: string, role: string) =>
    post<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/collaborators`, { username, role }),
  removeCollaborator: (o: string, r: string, userId: number) =>
    del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/collaborators/${userId}`),
  listWebhooks: (o: string, r: string) => get<Webhook[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/webhooks`),
  createWebhook: (o: string, r: string, url: string, secret: string, events: string) =>
    post<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/webhooks`, { url, secret, events }),
  deleteWebhook: (o: string, r: string, id: number) =>
    del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/webhooks/${id}`),

  // previews
  listPreviews: (o: string, r: string) => get<Preview[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/previews`),
  createPreview: (o: string, r: string, ref: string) =>
    post<Preview>(`/api/v1/repos/${enc(o)}/${enc(r)}/previews`, { ref }),
  deletePreview: (o: string, r: string, id: number) =>
    del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/previews/${id}`),
  previewEnv: (o: string, r: string, id: number) =>
    get<PreviewEnv>(`/api/v1/repos/${enc(o)}/${enc(r)}/previews/${id}/env`),
  stopPreviewEnv: (o: string, r: string, id: number) =>
    del<{ ok: boolean }>(`/api/v1/repos/${enc(o)}/${enc(r)}/previews/${id}/env`),
  restartPreviewEnv: (o: string, r: string, id: number) =>
    post<PreviewEnv>(`/api/v1/repos/${enc(o)}/${enc(r)}/previews/${id}/env/restart`),

  // CI
  listRuns: (o: string, r: string) => get<CIRun[]>(`/api/v1/repos/${enc(o)}/${enc(r)}/ci/runs`),
  run: (o: string, r: string, n: number | string) => get<CIRun>(`/api/v1/repos/${enc(o)}/${enc(r)}/ci/runs/${n}`),
  rerun: (o: string, r: string, n: number) => post<CIRun>(`/api/v1/repos/${enc(o)}/${enc(r)}/ci/runs/${n}/rerun`),
}

// qrURL builds the server-rendered QR code image URL for a given text.
export const qrURL = (text: string) => `/api/v1/qr?text=${enc(text)}`
