import { useCallback, useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { KeyRound, Plus, ShieldCheck, Tag, Trash2, Webhook as WebhookIcon, X } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { EmptyState, ErrorNote, LabelPill, PageLoading, UserAvatar, UserLink } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"
import { timeAgo } from "@/lib/format"
import {
  api,
  type Collaborator,
  type Label as RepoLabel,
  type Repo,
  type RepoSecret,
  type Webhook,
} from "@/lib/api"

const errMsg = (e: unknown, fallback: string) => (e instanceof Error ? e.message : fallback)

// ---------- General ----------

function GeneralCard({ repo, refresh }: { repo: Repo; refresh: () => Promise<void> }) {
  const [description, setDescription] = useState(repo.description)
  const [defaultBranch, setDefaultBranch] = useState(repo.default_branch)
  const [isPrivate, setIsPrivate] = useState(repo.private)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    setDescription(repo.description)
    setDefaultBranch(repo.default_branch)
    setIsPrivate(repo.private)
  }, [repo])

  const branches = repo.branches?.length
    ? repo.branches
    : [{ name: repo.default_branch, sha: "" }]

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.updateRepo(repo.owner, repo.name, {
        description,
        default_branch: defaultBranch,
        private: isPrivate,
      })
      toast.success("Repository settings saved")
      await refresh()
    } catch (err) {
      toast.error(errMsg(err, "failed to save settings"))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>General</CardTitle>
        <CardDescription>Name, visibility and defaults for {repo.full_name}.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="repo-description">Description</Label>
            <Input
              id="repo-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="A short sentence about this project"
            />
          </div>
          <div className="space-y-2">
            <Label>Default branch</Label>
            <Select value={defaultBranch} onValueChange={setDefaultBranch}>
              <SelectTrigger className="w-full font-mono text-xs">
                <SelectValue placeholder="Select a branch" />
              </SelectTrigger>
              <SelectContent>
                {branches.map((b) => (
                  <SelectItem key={b.name} value={b.name} className="font-mono text-xs">
                    {b.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
            <div>
              <Label htmlFor="repo-private">Private repository</Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                Only you and collaborators can see this repository.
              </p>
            </div>
            <Switch id="repo-private" checked={isPrivate} onCheckedChange={setIsPrivate} />
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save changes"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

// ---------- Merges & protection ----------

function ToggleRow({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-2">
      <div>
        <div className="text-sm font-medium">{label}</div>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onChange} />
    </div>
  )
}

function MergeCard({ repo, refresh }: { repo: Repo; refresh: () => Promise<void> }) {
  const [approvals, setApprovals] = useState(String(repo.require_approvals))

  useEffect(() => {
    setApprovals(String(repo.require_approvals))
  }, [repo.require_approvals])

  const patch = async (body: Record<string, unknown>) => {
    try {
      await api.updateRepo(repo.owner, repo.name, body)
      await refresh()
    } catch (err) {
      toast.error(errMsg(err, "failed to update settings"))
    }
  }

  const commitApprovals = () => {
    const n = Math.max(0, Math.min(10, Math.floor(Number(approvals)) || 0))
    setApprovals(String(n))
    if (n !== repo.require_approvals) patch({ require_approvals: n })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Merges &amp; protection</CardTitle>
        <CardDescription>How pull requests land on {repo.default_branch}.</CardDescription>
      </CardHeader>
      <CardContent>
        <ToggleRow
          label="Allow merge commits"
          hint="Merge with a merge commit, keeping full history."
          checked={repo.allow_merge}
          onChange={(v) => patch({ allow_merge: v })}
        />
        <ToggleRow
          label="Allow squash merging"
          hint="Combine all commits into one before merging."
          checked={repo.allow_squash}
          onChange={(v) => patch({ allow_squash: v })}
        />
        <ToggleRow
          label="Allow rebase merging"
          hint="Replay commits onto the base branch without a merge commit."
          checked={repo.allow_rebase}
          onChange={(v) => patch({ allow_rebase: v })}
        />
        <ToggleRow
          label="Delete branch on merge"
          hint="Automatically clean up head branches after merging."
          checked={repo.delete_branch_on_merge}
          onChange={(v) => patch({ delete_branch_on_merge: v })}
        />
        <Separator className="my-3" />
        <ToggleRow
          label="Require CI to pass"
          hint="Block merging until the latest CI run succeeds."
          checked={repo.require_ci_pass}
          onChange={(v) => patch({ require_ci_pass: v })}
        />
        <div className="flex items-center justify-between gap-4 py-2">
          <div>
            <div className="text-sm font-medium">Required approvals</div>
            <p className="text-xs text-muted-foreground">
              Approving reviews needed before merging (0–10).
            </p>
          </div>
          <Input
            type="number"
            min={0}
            max={10}
            value={approvals}
            onChange={(e) => setApprovals(e.target.value)}
            onBlur={commitApprovals}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                commitApprovals()
              }
            }}
            className="w-20 text-center"
            aria-label="Required approvals"
          />
        </div>
      </CardContent>
    </Card>
  )
}

// ---------- Collaborators ----------

function CollaboratorsCard({ repo }: { repo: Repo }) {
  const [collabs, setCollabs] = useState<Collaborator[] | null>(null)
  const [username, setUsername] = useState("")
  const [role, setRole] = useState("write")
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      setCollabs(await api.listCollaborators(repo.owner, repo.name))
    } catch (err) {
      toast.error(errMsg(err, "failed to load collaborators"))
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repo.owner, repo.name])

  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username.trim() || busy) return
    setBusy(true)
    try {
      await api.addCollaborator(repo.owner, repo.name, username.trim(), role)
      toast.success(`Added ${username.trim()} as ${role}`)
      setUsername("")
      await load()
    } catch (err) {
      toast.error(errMsg(err, "failed to add collaborator"))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (c: Collaborator) => {
    try {
      await api.removeCollaborator(repo.owner, repo.name, c.user.id)
      toast.success(`Removed ${c.user.username}`)
      await load()
    } catch (err) {
      toast.error(errMsg(err, "failed to remove collaborator"))
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Collaborators</CardTitle>
        <CardDescription>People with access to this repository.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {collabs === null ? (
          <PageLoading />
        ) : collabs.length === 0 ? (
          <EmptyState icon={<ShieldCheck />} title="Just you so far" className="py-8">
            Add someone by username to give them read, write or admin access.
          </EmptyState>
        ) : (
          <div className="divide-y rounded-lg border">
            {collabs.map((c) => (
              <div key={c.user.id} className="flex items-center gap-3 px-4 py-2.5">
                <UserAvatar user={c.user} className="size-7" />
                <UserLink user={c.user} className="min-w-0 flex-1 truncate text-sm" />
                <Badge variant="secondary" className="rounded-full capitalize">
                  {c.role}
                </Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => remove(c)}
                >
                  Remove
                </Button>
              </div>
            ))}
          </div>
        )}

        <form onSubmit={add} className="flex gap-2">
          <Input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Username"
            aria-label="Collaborator username"
          />
          <Select value={role} onValueChange={setRole}>
            <SelectTrigger className="w-28 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="read">Read</SelectItem>
              <SelectItem value="write">Write</SelectItem>
              <SelectItem value="admin">Admin</SelectItem>
            </SelectContent>
          </Select>
          <Button type="submit" variant="outline" disabled={busy || !username.trim()}>
            <Plus className="size-4" />
            Add
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

// ---------- Labels ----------

function LabelsCard({ repo }: { repo: Repo }) {
  const [labels, setLabels] = useState<RepoLabel[] | null>(null)
  const [name, setName] = useState("")
  const [color, setColor] = useState("#34c98e")
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      setLabels(await api.listLabels(repo.owner, repo.name))
    } catch (err) {
      toast.error(errMsg(err, "failed to load labels"))
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repo.owner, repo.name])

  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || busy) return
    setBusy(true)
    try {
      await api.createLabel(repo.owner, repo.name, name.trim(), color)
      setName("")
      await load()
    } catch (err) {
      toast.error(errMsg(err, "failed to create label"))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (label: RepoLabel) => {
    try {
      await api.deleteLabel(repo.owner, repo.name, label.id)
      await load()
    } catch (err) {
      toast.error(errMsg(err, "failed to delete label"))
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Labels</CardTitle>
        <CardDescription>Used to triage issues across this repository.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {labels === null ? (
          <PageLoading />
        ) : labels.length === 0 ? (
          <EmptyState icon={<Tag />} title="No labels yet" className="py-8">
            Create labels like <span className="font-medium">bug</span> or{" "}
            <span className="font-medium">enhancement</span> to organize issues.
          </EmptyState>
        ) : (
          <div className="flex flex-wrap gap-2">
            {labels.map((l) => (
              <span key={l.id} className="inline-flex items-center gap-1">
                <LabelPill label={l} />
                <button
                  type="button"
                  onClick={() => remove(l)}
                  className="rounded-full p-0.5 text-muted-foreground transition-colors hover:text-destructive"
                  aria-label={`Delete label ${l.name}`}
                >
                  <X className="size-3.5" />
                </button>
              </span>
            ))}
          </div>
        )}

        <form onSubmit={create} className="flex gap-2">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Label name"
            aria-label="Label name"
          />
          <input
            type="color"
            value={color}
            onChange={(e) => setColor(e.target.value)}
            className="h-9 w-12 shrink-0 cursor-pointer rounded border bg-transparent p-1"
            aria-label="Label color"
          />
          <Button type="submit" variant="outline" disabled={busy || !name.trim()}>
            <Plus className="size-4" />
            Create
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

// ---------- Webhooks ----------

function WebhooksCard({ repo }: { repo: Repo }) {
  const [hooks, setHooks] = useState<Webhook[] | null>(null)
  const [url, setUrl] = useState("")
  const [secret, setSecret] = useState("")
  const [events, setEvents] = useState("push,pull_request,ci_run")
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      setHooks(await api.listWebhooks(repo.owner, repo.name))
    } catch (err) {
      toast.error(errMsg(err, "failed to load webhooks"))
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repo.owner, repo.name])

  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!url.trim() || busy) return
    setBusy(true)
    try {
      await api.createWebhook(repo.owner, repo.name, url.trim(), secret, events.trim())
      toast.success("Webhook added")
      setUrl("")
      setSecret("")
      await load()
    } catch (err) {
      toast.error(errMsg(err, "failed to create webhook"))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (hook: Webhook) => {
    try {
      await api.deleteWebhook(repo.owner, repo.name, hook.id)
      await load()
    } catch (err) {
      toast.error(errMsg(err, "failed to delete webhook"))
    }
  }

  return (
    <Card className="lg:col-span-2">
      <CardHeader>
        <CardTitle>Webhooks</CardTitle>
        <CardDescription>
          POST deliveries for repository events. When a secret is set, payloads are signed with
          HMAC-SHA256 in the{" "}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">X-GitGit-Signature</code>{" "}
          header.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {hooks === null ? (
          <PageLoading />
        ) : hooks.length === 0 ? (
          <EmptyState icon={<WebhookIcon />} title="No webhooks yet" className="py-8">
            Add an endpoint below to be notified about pushes, pull requests and CI runs.
          </EmptyState>
        ) : (
          <div className="divide-y rounded-lg border">
            {hooks.map((h) => (
              <div key={h.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5">
                <span className="min-w-0 flex-1 truncate font-mono text-sm">{h.url}</span>
                <span className="text-xs text-muted-foreground">{h.events}</span>
                {h.has_secret && (
                  <Badge variant="outline" className="gap-1 rounded-full text-muted-foreground">
                    <ShieldCheck className="size-3" /> signed
                  </Badge>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => remove(h)}
                >
                  Delete
                </Button>
              </div>
            ))}
          </div>
        )}

        <form onSubmit={add} className="grid gap-2 sm:grid-cols-[2fr_1fr_1fr_auto]">
          <Input
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://example.com/hook"
            type="url"
            aria-label="Webhook URL"
            className="font-mono text-xs"
          />
          <Input
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
            placeholder="Secret (optional)"
            aria-label="Webhook secret"
          />
          <Input
            value={events}
            onChange={(e) => setEvents(e.target.value)}
            placeholder="push,pull_request,ci_run"
            aria-label="Webhook events"
            className="font-mono text-xs"
          />
          <Button type="submit" variant="outline" disabled={busy || !url.trim()}>
            <Plus className="size-4" />
            Add
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

// ---------- Danger zone ----------

function DangerCard({ repo }: { repo: Repo }) {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [confirm, setConfirm] = useState("")
  const [busy, setBusy] = useState(false)

  const destroy = async () => {
    if (confirm !== repo.full_name || busy) return
    setBusy(true)
    try {
      await api.deleteRepo(repo.owner, repo.name)
      toast.success(`Deleted ${repo.full_name}`)
      navigate("/dashboard")
    } catch (err) {
      toast.error(errMsg(err, "failed to delete repository"))
      setBusy(false)
    }
  }

  return (
    <Card className="border-destructive/40 lg:col-span-2">
      <CardHeader>
        <CardTitle className="text-destructive">Danger zone</CardTitle>
        <CardDescription>Irreversible actions. Tread carefully.</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-destructive/30 p-4">
          <div>
            <div className="text-sm font-medium">Delete this repository</div>
            <p className="text-xs text-muted-foreground">
              Permanently removes {repo.full_name} — code, pull requests, issues and CI history.
            </p>
          </div>
          <Button
            variant="outline"
            className="border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
            onClick={() => {
              setConfirm("")
              setOpen(true)
            }}
          >
            <Trash2 className="size-4" />
            Delete repository
          </Button>
        </div>
      </CardContent>

      <Dialog
        open={open}
        onOpenChange={(o) => {
          setOpen(o)
          if (!o) setConfirm("")
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {repo.full_name}?</DialogTitle>
            <DialogDescription>
              This permanently deletes the repository, its git history, pull requests, issues and
              CI runs. There is no undo.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="confirm-delete">
              Type <span className="font-mono font-semibold">{repo.full_name}</span> to confirm
            </Label>
            <Input
              id="confirm-delete"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              placeholder={repo.full_name}
              autoComplete="off"
              className="font-mono"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={confirm !== repo.full_name || busy}
              onClick={destroy}
            >
              {busy ? "Deleting…" : "I understand — delete this repository"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

// ---------- secrets ----------

// SecretsCard manages the values injected into Preview Environments. Values
// are write-only by design: this component can create and delete them, and has
// no way to read one back, because the API has no endpoint that returns one.
function SecretsCard({ repo }: { repo: Repo }) {
  const [secrets, setSecrets] = useState<RepoSecret[] | null>(null)
  const [dotenv, setDotenv] = useState("")
  const [name, setName] = useState("")
  const [value, setValue] = useState("")
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setSecrets(await api.listSecrets(repo.owner, repo.name))
    } catch {
      setSecrets([])
    }
  }, [repo.owner, repo.name])

  useEffect(() => {
    load()
  }, [load])

  async function importEnv() {
    setBusy(true)
    try {
      const res = await api.importDotenv(repo.owner, repo.name, dotenv)
      setDotenv("")
      toast.success(`Stored ${res.imported} secret${res.imported === 1 ? "" : "s"}`)
      if (res.ignored?.length) toast.warning(`Ignored ${res.ignored.join(", ")}`)
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed")
    } finally {
      setBusy(false)
    }
  }

  async function addOne() {
    setBusy(true)
    try {
      await api.setSecret(repo.owner, repo.name, name.trim(), value)
      setName("")
      setValue("")
      toast.success("Secret stored")
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card className="lg:col-span-2">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound className="size-4 text-tangerine" /> Preview secrets
        </CardTitle>
        <CardDescription>
          Environment variables given to Preview Environments of {repo.full_name}, encrypted at rest and kept out of
          git. Values are never shown again after you save them.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="rounded-lg border border-tangerine/40 bg-tangerine/5 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground">
          <b className="text-foreground">A preview runs the branch's own code.</b> Anyone who can push can read these
          from inside a build, so use credentials scoped to a preview database — never production keys.
        </div>

        <div className="space-y-2">
          <Label htmlFor="dotenv">Paste a .env file</Label>
          <textarea
            id="dotenv"
            value={dotenv}
            onChange={(e) => setDotenv(e.target.value)}
            spellCheck={false}
            rows={5}
            placeholder={"DATABASE_URL=postgres://…\nSUPABASE_ANON_KEY=eyJhbGciOi…"}
            className="w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs shadow-xs outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          />
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">
              Comments, <code className="font-mono">export</code> and quoted values are understood.
            </p>
            <Button size="sm" disabled={busy || !dotenv.trim()} onClick={importEnv}>
              <Plus className="size-4" /> Store secrets
            </Button>
          </div>
        </div>

        <Separator />

        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-0 flex-1 space-y-2">
            <Label htmlFor="secret-name">Name</Label>
            <Input
              id="secret-name"
              value={name}
              onChange={(e) => setName(e.target.value.toUpperCase())}
              placeholder="STRIPE_KEY"
              className="font-mono"
            />
          </div>
          <div className="min-w-0 flex-1 space-y-2">
            <Label htmlFor="secret-value">Value</Label>
            <Input
              id="secret-value"
              type="password"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              autoComplete="new-password"
              placeholder="•••••••••"
              className="font-mono"
            />
          </div>
          <Button variant="outline" disabled={busy || !name.trim()} onClick={addOne}>
            Add
          </Button>
        </div>

        {secrets === null ? null : secrets.length === 0 ? (
          <EmptyState icon={<KeyRound />} title="No secrets yet" className="py-8">
            Previews of this repository build with no extra environment.
          </EmptyState>
        ) : (
          <div className="divide-y rounded-lg border">
            {secrets.map((s) => (
              <div key={s.name} className="flex items-center gap-3 px-3 py-2">
                <KeyRound className="size-3.5 shrink-0 text-muted-foreground" />
                <code className="min-w-0 flex-1 truncate font-mono text-sm">{s.name}</code>
                <span className="shrink-0 text-xs text-muted-foreground">{timeAgo(s.updated_at)}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0 text-destructive hover:text-destructive"
                  title={`Delete ${s.name}`}
                  onClick={async () => {
                    await api.deleteSecret(repo.owner, repo.name, s.name)
                    await load()
                  }}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ---------- page ----------

export default function RepoSettings() {
  const { repo, refresh } = useRepo()

  if (!repo.can_admin) {
    return <ErrorNote message="You need admin access to view repository settings." />
  }

  return (
    <div className="grid items-start gap-6 lg:grid-cols-2">
      <GeneralCard repo={repo} refresh={refresh} />
      <MergeCard repo={repo} refresh={refresh} />
      <CollaboratorsCard repo={repo} />
      <LabelsCard repo={repo} />
      <SecretsCard repo={repo} />
      <WebhooksCard repo={repo} />
      <DangerCard repo={repo} />
    </div>
  )
}
