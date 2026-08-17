import { useCallback, useEffect, useState } from "react"
import { Check, Copy, ExternalLink, Eye, Loader2, RotateCw, ScrollText, Server, Smartphone, Square, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { api, qrURL, type Preview, type PreviewEnv, type Repo } from "@/lib/api"
import { shortSha } from "@/lib/format"
import { cn } from "@/lib/utils"

const ENV_LOOK: Record<string, { label: string; dot: string; text: string }> = {
  queued: { label: "Queued", dot: "bg-muted-foreground", text: "text-muted-foreground" },
  building: { label: "Building", dot: "bg-tangerine animate-pulse", text: "text-tangerine" },
  running: { label: "Running", dot: "bg-primary", text: "text-primary" },
  failed: { label: "Failed", dot: "bg-destructive", text: "text-destructive" },
  stopped: { label: "Stopped", dot: "bg-muted-foreground", text: "text-muted-foreground" },
  none: { label: "Not started", dot: "bg-muted-foreground", text: "text-muted-foreground" },
}

// EnvironmentPanel shows the state of the running instance behind a preview,
// polling while it builds, with logs and start/stop controls.
function EnvironmentPanel({
  repo,
  preview,
  onChange,
}: {
  repo: Repo
  preview: Preview
  onChange: () => void
}) {
  const [env, setEnv] = useState<PreviewEnv | null>(preview.env ?? null)
  const [showLog, setShowLog] = useState(false)
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    try {
      setEnv(await api.previewEnv(repo.owner, repo.name, preview.id))
    } catch {
      /* transient */
    }
  }, [repo.owner, repo.name, preview.id])

  // poll while the environment is coming up
  useEffect(() => {
    if (env && (env.status === "building" || env.status === "queued")) {
      const t = setInterval(refresh, 2000)
      return () => clearInterval(t)
    }
  }, [env, refresh])

  useEffect(() => {
    refresh()
  }, [refresh])

  const status = env?.status ?? "none"
  const look = ENV_LOOK[status] ?? ENV_LOOK.none

  return (
    <div className="rounded-xl border bg-muted/30 p-3">
      <div className="flex items-center gap-2">
        <Server className="size-4 text-muted-foreground" />
        <span className="text-sm font-semibold">Preview Environment</span>
        <span className={cn("ml-auto inline-flex items-center gap-1.5 text-xs font-medium", look.text)}>
          <span className={cn("size-1.5 rounded-full", look.dot)} />
          {look.label}
        </span>
      </div>

      <p className="mt-1 text-xs text-muted-foreground">
        {status === "running" && "Your app is running on its own domain, isolated from GitGit."}
        {(status === "building" || status === "queued") && "Building this branch — the link works as soon as it's up."}
        {status === "failed" && (env?.message || "The build or the app exited.")}
        {status === "stopped" && (env?.message || "Not currently running.")}
        {status === "none" && "Starts automatically the first time the link is opened."}
      </p>

      <div className="mt-2 flex items-center gap-2">
        {repo.can_write && (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={async () => {
                setBusy(true)
                try {
                  await api.restartPreviewEnv(repo.owner, repo.name, preview.id)
                  toast.success("Rebuilding environment")
                  await refresh()
                  onChange()
                } catch (e) {
                  toast.error(e instanceof Error ? e.message : "failed")
                } finally {
                  setBusy(false)
                }
              }}
            >
              <RotateCw className="size-3.5" />
              {status === "running" ? "Rebuild" : "Start"}
            </Button>
            {(status === "running" || status === "building") && (
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                onClick={async () => {
                  setBusy(true)
                  try {
                    await api.stopPreviewEnv(repo.owner, repo.name, preview.id)
                    await refresh()
                  } finally {
                    setBusy(false)
                  }
                }}
              >
                <Square className="size-3.5" /> Stop
              </Button>
            )}
          </>
        )}
        <Button variant="ghost" size="sm" className="ml-auto" onClick={() => setShowLog(!showLog)}>
          <ScrollText className="size-3.5" /> {showLog ? "Hide" : "Logs"}
        </Button>
      </div>

      {showLog && (
        <pre className="mt-2 max-h-56 overflow-auto rounded-lg bg-zinc-900 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-zinc-200">
          {env?.log?.trim() || "no output yet"}
        </pre>
      )}
    </div>
  )
}

// PreviewDialog creates (or reuses) a live preview of a branch and shows its
// shareable URL plus a QR code for testing on a phone. The preview follows
// the branch: push again and the same link serves the new tip.
export function PreviewDialog({
  repo,
  refName,
  size = "sm",
  variant = "outline",
}: {
  repo: Repo
  refName: string
  size?: "sm" | "default"
  variant?: "outline" | "ghost" | "secondary"
}) {
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [host, setHost] = useState("")
  const [error, setError] = useState("")
  const [copied, setCopied] = useState(false)

  const create = useCallback(async () => {
    try {
      const p = await api.createPreview(repo.owner, repo.name, refName)
      setPreview(p)
      setError("")
      // prefer a LAN address for the QR when browsing via localhost
      const lan = p.hosts.find((h) => !h.includes("localhost") && !h.includes("127.0.0.1"))
      setHost(lan ?? p.hosts[0] ?? "")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to create preview")
    }
  }, [repo.owner, repo.name, refName])

  useEffect(() => {
    if (open) {
      setPreview(null)
      create()
    }
  }, [open, create])

  // A Preview Environment owns its whole subdomain, so its URL is the origin
  // itself; static previews live under a path on the main host.
  const url = preview?.url ? preview.url : preview && host ? host + preview.path : ""
  const browserURL = preview?.url ? preview.url : preview ? preview.path : ""

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant={variant} size={size}>
          <Eye className="size-4" />
          Preview
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Eye className="size-5 text-primary" /> {preview?.runnable ? "Preview Environment" : "Branch preview"}
          </DialogTitle>
          <DialogDescription>
            {preview?.runnable ? "A running instance of " : "A live, sandboxed preview of "}
            <code className="rounded bg-muted px-1.5 font-mono text-xs">{refName}</code>
            {preview?.sha && (
              <>
                {" "}
                at <span className="font-mono">{shortSha(preview.sha)}</span>
              </>
            )}{" "}
            — it follows the branch, so new pushes appear on the same link.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}

        {!preview && !error && (
          <div className="flex justify-center py-10 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        )}

        {preview && (
          <div className="space-y-4">
            {preview.runnable && <EnvironmentPanel repo={repo} preview={preview} onChange={create} />}
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded-lg border bg-muted px-3 py-2 font-mono text-xs">
                {url || browserURL}
              </code>
              <Button
                variant="outline"
                size="icon"
                className="size-8 shrink-0"
                onClick={async () => {
                  await navigator.clipboard.writeText(url || location.origin + browserURL)
                  setCopied(true)
                  setTimeout(() => setCopied(false), 1500)
                }}
              >
                {copied ? <Check className="size-4 text-primary" /> : <Copy className="size-4" />}
              </Button>
              <Button asChild variant="outline" size="icon" className="size-8 shrink-0">
                <a href={browserURL} target="_blank" rel="noreferrer">
                  <ExternalLink className="size-4" />
                </a>
              </Button>
            </div>

            <Separator />

            <div>
              <h4 className="mb-1 flex items-center gap-2 text-sm font-semibold">
                <Smartphone className="size-4 text-tangerine" /> Mobile test
              </h4>
              <p className="mb-3 text-xs text-muted-foreground">
                Scan with your phone — same Wi-Fi network as this machine.
              </p>
              {preview.hosts.length > 1 && (
                <Select value={host} onValueChange={setHost}>
                  <SelectTrigger className="mb-3 w-full font-mono text-xs">
                    <SelectValue placeholder="Choose a reachable address" />
                  </SelectTrigger>
                  <SelectContent>
                    {preview.hosts.map((h) => (
                      <SelectItem key={h} value={h} className="font-mono text-xs">
                        {h}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {url ? (
                <div className="flex justify-center rounded-xl border bg-white p-4">
                  <img src={qrURL(url)} alt={`QR code for ${url}`} className="size-44" />
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">No reachable address detected.</p>
              )}
            </div>

            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>
                Anyone with the link can view this{repo.private && " (even on this private repo)"} · expires in{" "}
                {Math.max(1, Math.round((preview.expires_at - Date.now() / 1000) / 3600))}h
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={async () => {
                  try {
                    await api.deletePreview(repo.owner, repo.name, preview.id)
                    toast.success("Preview revoked")
                    setOpen(false)
                  } catch (e) {
                    toast.error(e instanceof Error ? e.message : "failed")
                  }
                }}
              >
                <Trash2 className="size-3.5" /> Revoke
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
