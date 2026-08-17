import { useCallback, useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import { AlertCircle, ArrowRight, CheckCircle2, CloudDownload, Loader2, Lock } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { ErrorNote } from "@/components/shared"
import { useAuth } from "@/lib/auth"
import { api, type ImportJob } from "@/lib/api"

// The GitHub mark, drawn inline: lucide v1 no longer ships brand icons.
function GitHubMark({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/>
    </svg>
  )
}

// Import a repository from GitHub. Git data comes over with `git clone
// --mirror`; issues are optional and use the GitHub API.
export default function ImportPage() {
  const { user, loading } = useAuth()
  const navigate = useNavigate()

  const [source, setSource] = useState("")
  const [name, setName] = useState("")
  const [isPrivate, setPrivate] = useState(false)
  const [token, setToken] = useState("")
  const [withIssues, setWithIssues] = useState(false)
  const [error, setError] = useState("")
  const [job, setJob] = useState<ImportJob | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const logRef = useRef<HTMLPreElement>(null)

  useEffect(() => {
    if (!loading && !user) navigate("/login")
  }, [loading, user, navigate])

  // derive a default repository name from whatever was pasted
  const derived = (() => {
    const s = source.trim().replace(/\.git$/, "").replace(/\/+$/, "")
    const parts = s.split("/").filter(Boolean)
    return parts.length ? parts[parts.length - 1] : ""
  })()

  const poll = useCallback(
    async (id: number) => {
      try {
        const j = await api.importStatus(id)
        setJob(j)
        if (j.status === "done") {
          toast.success("Import complete")
          if (j.repo) setTimeout(() => navigate(`/${j.repo}`), 900)
        } else if (j.status === "failed") {
          toast.error(j.message || "Import failed")
        }
        return j.status
      } catch {
        return "running"
      }
    },
    [navigate],
  )

  useEffect(() => {
    if (job && job.status === "running") {
      const t = setInterval(async () => {
        const s = await poll(job.id)
        if (s !== "running") clearInterval(t)
      }, 1500)
      return () => clearInterval(t)
    }
  }, [job, poll])

  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
  }, [job?.log])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError("")
    setSubmitting(true)
    try {
      const j = await api.startImport({
        source: source.trim(),
        name: name.trim() || undefined,
        private: isPrivate,
        token: token.trim() || undefined,
        import_issues: withIssues,
      })
      setJob(j)
    } catch (err) {
      setError(err instanceof Error ? err.message : "import failed")
    } finally {
      setSubmitting(false)
    }
  }

  const running = job?.status === "running"

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-6">
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
          <GitHubMark className="size-6" /> Import a repository
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Bring a repository over from GitHub — every branch and tag, and optionally its issues.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Source</CardTitle>
          <CardDescription>
            Paste a URL or <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">owner/repo</code>. Other
            git hosts work too.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="source">Repository</Label>
              <Input
                id="source"
                value={source}
                onChange={(e) => setSource(e.target.value)}
                placeholder="octocat/Hello-World  ·  https://github.com/octocat/Hello-World"
                required
                autoFocus
                disabled={!!job}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="name">Name on GitGit</Label>
              <div className="flex items-center gap-2">
                <span className="shrink-0 font-mono text-sm text-muted-foreground">{user?.username}/</span>
                <Input
                  id="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={derived || "repository-name"}
                  pattern="[A-Za-z0-9._-]+"
                  disabled={!!job}
                />
              </div>
            </div>

            <div className="space-y-3 rounded-lg border p-3">
              <label className="flex items-start justify-between gap-4">
                <span>
                  <span className="text-sm font-medium">Private repository</span>
                  <span className="block text-xs text-muted-foreground">Only you and collaborators can see it.</span>
                </span>
                <Switch checked={isPrivate} onCheckedChange={setPrivate} disabled={!!job} />
              </label>
              <label className="flex items-start justify-between gap-4">
                <span>
                  <span className="text-sm font-medium">Import issues</span>
                  <span className="block text-xs text-muted-foreground">
                    Copies issues, labels, and open/closed state. Pull requests come across as issues, since their
                    branches may not exist here.
                  </span>
                </span>
                <Switch checked={withIssues} onCheckedChange={setWithIssues} disabled={!!job} />
              </label>
            </div>

            <div className="space-y-2">
              <Label htmlFor="token" className="flex items-center gap-1.5">
                <Lock className="size-3.5" /> Access token
                <span className="font-normal text-muted-foreground">
                  — required for private repos{withIssues && ", and raises the issue rate limit"}
                </span>
              </Label>
              <Input
                id="token"
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="ghp_… (optional for public repositories)"
                autoComplete="off"
                disabled={!!job}
              />
              <p className="text-xs text-muted-foreground">
                Used only for this import and never stored.
              </p>
            </div>

            {error && <ErrorNote message={error} />}

            {!job && (
              <Button type="submit" disabled={submitting || !source.trim()} className="w-full">
                {submitting ? <Loader2 className="size-4 animate-spin" /> : <CloudDownload className="size-4" />}
                Import repository
              </Button>
            )}
          </form>

          {job && (
            <div className="mt-5 space-y-3">
              <div className="flex items-center gap-2 text-sm">
                {running && <Loader2 className="size-4 animate-spin text-tangerine" />}
                {job.status === "done" && <CheckCircle2 className="size-4 text-primary" />}
                {job.status === "failed" && <AlertCircle className="size-4 text-destructive" />}
                <span className="font-medium">
                  {running && `Importing ${job.source}…`}
                  {job.status === "done" && `Imported ${job.source}`}
                  {job.status === "failed" && "Import failed"}
                </span>
                {job.message && <span className="text-xs text-muted-foreground">· {job.message}</span>}
              </div>

              <pre
                ref={logRef}
                className="max-h-52 overflow-auto rounded-lg bg-zinc-900 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-zinc-200"
              >
                {job.log?.trim() || "starting…"}
              </pre>

              {job.status === "done" && job.repo && (
                <Button asChild className="w-full">
                  <a href={`/${job.repo}`}>
                    Open {job.repo} <ArrowRight className="size-4" />
                  </a>
                </Button>
              )}
              {job.status === "failed" && (
                <Button variant="outline" className="w-full" onClick={() => setJob(null)}>
                  Try again
                </Button>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
