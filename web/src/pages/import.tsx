import { useCallback, useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import { AlertCircle, ArrowRight, CheckCircle2, CloudDownload, FileArchive, Loader2, Lock, Upload } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
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
  const [zipFile, setZipFile] = useState<File | null>(null)
  const [zipName, setZipName] = useState("")
  const [dragging, setDragging] = useState(false)
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

  const submitZip = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!zipFile) return
    setError("")
    setSubmitting(true)
    try {
      const j = await api.uploadZip(zipFile, { name: zipName.trim() || undefined, private: isPrivate })
      setJob(j)
    } catch (err) {
      setError(err instanceof Error ? err.message : "upload failed")
    } finally {
      setSubmitting(false)
    }
  }

  const pickZip = (f: File | null | undefined) => {
    if (!f) return
    if (!f.name.toLowerCase().endsWith(".zip")) {
      setError("Only .zip archives are supported.")
      return
    }
    setError("")
    setZipFile(f)
    if (!zipName) setZipName(f.name.replace(/\.zip$/i, ""))
  }

  const running = job?.status === "running"

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-6">
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
          <GitHubMark className="size-6" /> Import a repository
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Bring code in from GitHub with its full history and issues, or upload a .zip from your machine.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Source</CardTitle>
          <CardDescription>
            Pull it from a git host, or upload an archive from your machine.
          </CardDescription>
        </CardHeader>
        <CardContent>
        <Tabs defaultValue="remote">
          <TabsList className="mb-4 w-full">
            <TabsTrigger value="remote" disabled={!!job}>
              <CloudDownload className="size-4" /> From GitHub
            </TabsTrigger>
            <TabsTrigger value="zip" disabled={!!job}>
              <FileArchive className="size-4" /> Upload .zip
            </TabsTrigger>
          </TabsList>

          <TabsContent value="remote">
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
          </TabsContent>

          <TabsContent value="zip">
            <form onSubmit={submitZip} className="space-y-5">
              {/* drop zone */}
              <label
                onDragOver={(e) => {
                  e.preventDefault()
                  setDragging(true)
                }}
                onDragLeave={() => setDragging(false)}
                onDrop={(e) => {
                  e.preventDefault()
                  setDragging(false)
                  pickZip(e.dataTransfer.files?.[0])
                }}
                className={cn(
                  "flex cursor-pointer flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-10 text-center transition-colors",
                  dragging ? "border-primary bg-primary/5" : "hover:bg-muted/40",
                  job && "pointer-events-none opacity-60",
                )}
              >
                <input
                  type="file"
                  accept=".zip,application/zip"
                  className="sr-only"
                  disabled={!!job}
                  onChange={(e) => pickZip(e.target.files?.[0])}
                />
                {zipFile ? (
                  <>
                    <FileArchive className="size-7 text-primary" />
                    <span className="text-sm font-medium">{zipFile.name}</span>
                    <span className="text-xs text-muted-foreground">
                      {(zipFile.size / 1024 / 1024).toFixed(1)} MB · click to choose another
                    </span>
                  </>
                ) : (
                  <>
                    <Upload className="size-7 text-muted-foreground" />
                    <span className="text-sm font-medium">Drop a .zip here, or click to browse</span>
                    <span className="text-xs text-muted-foreground">
                      A project folder becomes the initial commit. An archive containing a{" "}
                      <code className="font-mono">.git</code> directory keeps its history.
                    </span>
                  </>
                )}
              </label>

              <div className="space-y-2">
                <Label htmlFor="zipname">Name on GitGit</Label>
                <div className="flex items-center gap-2">
                  <span className="shrink-0 font-mono text-sm text-muted-foreground">{user?.username}/</span>
                  <Input
                    id="zipname"
                    value={zipName}
                    onChange={(e) => setZipName(e.target.value)}
                    placeholder="repository-name"
                    pattern="[A-Za-z0-9._-]+"
                    disabled={!!job}
                  />
                </div>
              </div>

              <label className="flex items-start justify-between gap-4 rounded-lg border p-3">
                <span>
                  <span className="text-sm font-medium">Private repository</span>
                  <span className="block text-xs text-muted-foreground">Only you and collaborators can see it.</span>
                </span>
                <Switch checked={isPrivate} onCheckedChange={setPrivate} disabled={!!job} />
              </label>

              {error && <ErrorNote message={error} />}

              {!job && (
                <Button type="submit" disabled={submitting || !zipFile} className="w-full">
                  {submitting ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
                  {submitting ? "Uploading…" : "Upload and create repository"}
                </Button>
              )}
            </form>
          </TabsContent>
        </Tabs>

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
