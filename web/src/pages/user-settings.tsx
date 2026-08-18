import { useCallback, useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { Archive, Copy, KeyRound, Loader2, Plus, Trash2 } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { EmptyState, PageLoading } from "@/components/shared"
import { api, type AccessToken, type BackupFile, type SSHKey, type User } from "@/lib/api"
import { useAuth } from "@/lib/auth"
import { timeAgo } from "@/lib/format"

function ProfileCard({ user, onSaved }: { user: User; onSaved: () => Promise<void> }) {
  const [fullName, setFullName] = useState(user.full_name)
  const [email, setEmail] = useState(user.email)
  const [busy, setBusy] = useState(false)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.updateProfile(email.trim(), fullName.trim())
      toast.success("Profile saved")
      await onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to save profile")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile</CardTitle>
        <CardDescription>How you appear across this GitGit instance.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="full-name">Full name</Label>
              <Input
                id="full-name"
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                placeholder="Ada Lovelace"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save profile"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function PasswordCard() {
  const [current, setCurrent] = useState("")
  const [next, setNext] = useState("")
  const [busy, setBusy] = useState(false)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.changePassword(current, next)
      toast.success("Password updated")
      setCurrent("")
      setNext("")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to change password")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Password</CardTitle>
        <CardDescription>Use a long, unique password you don&rsquo;t use elsewhere.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="pw-current">Current password</Label>
              <Input
                id="pw-current"
                type="password"
                autoComplete="current-password"
                value={current}
                onChange={(e) => setCurrent(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pw-new">New password</Label>
              <Input
                id="pw-new"
                type="password"
                autoComplete="new-password"
                value={next}
                onChange={(e) => setNext(e.target.value)}
                required
              />
            </div>
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={busy || !current || !next}>
              {busy ? "Updating…" : "Update password"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function TokensCard() {
  const [tokens, setTokens] = useState<AccessToken[] | null>(null)
  const [name, setName] = useState("")
  const [busy, setBusy] = useState(false)
  const [fresh, setFresh] = useState<{ name: string; token: string } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<AccessToken | null>(null)

  const load = async () => {
    try {
      setTokens(await api.listTokens())
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to load tokens")
    }
  }

  useEffect(() => {
    load()
  }, [])

  const generate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || busy) return
    setBusy(true)
    try {
      const created = await api.createToken(name.trim())
      setFresh(created)
      setName("")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to create token")
    } finally {
      setBusy(false)
    }
  }

  const copyToken = async (token: string) => {
    try {
      await navigator.clipboard.writeText(token)
      toast.success("Token copied to clipboard")
    } catch {
      toast.error("Could not copy — select the token and copy it manually")
    }
  }

  const remove = async () => {
    if (!deleteTarget) return
    try {
      await api.deleteToken(deleteTarget.id)
      toast.success(`Token “${deleteTarget.name}” deleted`)
      setDeleteTarget(null)
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to delete token")
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Personal access tokens</CardTitle>
        <CardDescription>
          Tokens work as your password for git over HTTP and with the API via{" "}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
            Authorization: token …
          </code>
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {fresh && (
          <Alert className="border-primary/40 bg-primary/5">
            <KeyRound className="size-4 text-primary" />
            <AlertTitle className="text-primary">
              Token “{fresh.name}” created — copy it now
            </AlertTitle>
            <AlertDescription className="w-full">
              <div className="flex w-full items-center gap-2">
                <code className="min-w-0 flex-1 rounded-md border bg-background px-2.5 py-1.5 font-mono text-xs break-all select-all">
                  {fresh.token}
                </code>
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  onClick={() => copyToken(fresh.token)}
                  aria-label="Copy token"
                >
                  <Copy className="size-3.5" />
                </Button>
              </div>
              <p className="text-xs">You won&rsquo;t be able to see this token again.</p>
            </AlertDescription>
          </Alert>
        )}

        {tokens === null ? (
          <PageLoading />
        ) : tokens.length === 0 ? (
          <EmptyState icon={<KeyRound />} title="No tokens yet" className="py-10">
            Generate a token below to push over HTTP or call the API.
          </EmptyState>
        ) : (
          <div className="divide-y rounded-lg border">
            {tokens.map((t) => (
              <div key={t.id} className="flex items-center gap-3 px-4 py-3">
                <KeyRound className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{t.name}</div>
                  <div className="text-xs text-muted-foreground">
                    created {timeAgo(t.created_at)}
                    {" · "}
                    {t.last_used_at ? `last used ${timeAgo(t.last_used_at)}` : "never used"}
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => setDeleteTarget(t)}
                >
                  Delete
                </Button>
              </div>
            ))}
          </div>
        )}

        <form onSubmit={generate} className="flex gap-2">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Token name (e.g. laptop, CI)"
            aria-label="Token name"
          />
          <Button type="submit" variant="outline" disabled={busy || !name.trim()}>
            <Plus className="size-4" />
            Generate
          </Button>
        </form>
      </CardContent>

      <Dialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete token?</DialogTitle>
            <DialogDescription>
              Anything still authenticating with “{deleteTarget?.name}” will immediately stop
              working. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={remove}>
              Delete token
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

// SSHKeysCard manages the public keys that authenticate git over SSH. The
// key itself is the identity — the SSH username is always "git" and carries
// no authority — so a key registered here is exactly as powerful as the
// account it belongs to.
function SSHKeysCard() {
  const [keys, setKeys] = useState<SSHKey[] | null>(null)
  const [title, setTitle] = useState("")
  const [key, setKey] = useState("")
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setKeys(await api.listSSHKeys())
    } catch {
      setKeys([])
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound className="size-4 text-primary" /> SSH keys
        </CardTitle>
        <CardDescription>
          Push and pull over SSH without typing a password. Paste the contents of your public key —
          the <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">.pub</code> file,
          never the private one.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="key-title">Label</Label>
          <Input
            id="key-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Work laptop"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="key-body">Public key</Label>
          <textarea
            id="key-body"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            rows={3}
            spellCheck={false}
            placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5… you@laptop"
            className="w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs shadow-xs outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          />
        </div>
        <Button
          disabled={busy || !key.trim()}
          onClick={async () => {
            setBusy(true)
            try {
              await api.addSSHKey(title.trim(), key.trim())
              setTitle("")
              setKey("")
              toast.success("SSH key added")
              await load()
            } catch (e) {
              toast.error(e instanceof Error ? e.message : "failed")
            } finally {
              setBusy(false)
            }
          }}
        >
          <Plus className="size-4" /> Add key
        </Button>

        {keys === null ? null : keys.length === 0 ? (
          <p className="text-sm text-muted-foreground">No SSH keys yet.</p>
        ) : (
          <div className="divide-y rounded-lg border">
            {keys.map((k) => (
              <div key={k.id} className="flex items-center gap-3 px-3 py-2">
                <KeyRound className="size-3.5 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{k.title}</div>
                  <div className="truncate font-mono text-[11px] text-muted-foreground">
                    {k.fingerprint}
                  </div>
                </div>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0 text-destructive hover:text-destructive"
                  title={`Delete ${k.title}`}
                  onClick={async () => {
                    await api.deleteSSHKey(k.id)
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

// BackupsCard is site-admin only, because an archive holds every private
// repository and every user row on the instance.
function BackupsCard() {
  const [backups, setBackups] = useState<BackupFile[] | null>(null)
  const [dir, setDir] = useState("")
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await api.listBackups()
      setBackups(res.backups)
      setDir(res.directory)
    } catch {
      setBackups([])
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Archive className="size-4 text-primary" /> Backups
        </CardTitle>
        <CardDescription>
          A consistent snapshot of the database plus every repository, written to{" "}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">{dir || "…"}</code>.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded-lg border border-tangerine/40 bg-tangerine/5 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground">
          <b className="text-foreground">The secret key is not in the archive.</b> Bundling it with
          the data it decrypts would protect nothing. Copy{" "}
          <code className="font-mono">secret.key</code> somewhere separate, or your restored
          instance will list secret names it cannot read.
        </div>

        <Button
          disabled={busy}
          onClick={async () => {
            setBusy(true)
            try {
              const r = await api.createBackup()
              toast.success(`Wrote ${r.name}`)
              await load()
            } catch (e) {
              toast.error(e instanceof Error ? e.message : "failed")
            } finally {
              setBusy(false)
            }
          }}
        >
          {busy ? <Loader2 className="size-4 animate-spin" /> : <Archive className="size-4" />}
          Back up now
        </Button>

        {backups === null ? null : backups.length === 0 ? (
          <p className="text-sm text-muted-foreground">No backups yet.</p>
        ) : (
          <div className="divide-y rounded-lg border">
            {backups.map((b) => (
              <div key={b.name} className="flex items-center gap-3 px-3 py-2">
                <Archive className="size-3.5 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-xs">{b.name}</div>
                  <div className="text-[11px] text-muted-foreground">
                    {(b.size / 1048576).toFixed(1)} MB · {timeAgo(b.created_at)}
                  </div>
                </div>
                <Button asChild variant="ghost" size="sm" className="h-8 shrink-0 px-2 text-xs">
                  <a href={api.backupDownloadURL(b.name)} download>
                    Download
                  </a>
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0 text-destructive hover:text-destructive"
                  title={`Delete ${b.name}`}
                  onClick={async () => {
                    await api.deleteBackup(b.name)
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

export default function UserSettings() {
  const { user, loading, refresh } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!loading && !user) navigate("/login")
  }, [loading, user, navigate])

  if (loading || !user) return <PageLoading />

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage your account, security, SSH keys and access tokens.
        </p>
      </header>
      <ProfileCard user={user} onSaved={refresh} />
      <PasswordCard />
      <SSHKeysCard />
      <TokensCard />
      {user.is_admin && <BackupsCard />}
    </div>
  )
}
