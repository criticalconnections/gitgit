import { useCallback, useEffect, useState } from "react"
import { Check, Copy, ExternalLink, Eye, Loader2, Smartphone, Trash2 } from "lucide-react"
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
import { api, qrURL, type Preview, type Repo } from "@/lib/api"
import { shortSha } from "@/lib/format"

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

  const url = preview && host ? host + preview.path : ""
  const browserURL = preview ? preview.path : ""

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
            <Eye className="size-5 text-primary" /> Branch preview
          </DialogTitle>
          <DialogDescription>
            A live, sandboxed preview of <code className="rounded bg-muted px-1.5 font-mono text-xs">{refName}</code>
            {preview?.sha && <> at <span className="font-mono">{shortSha(preview.sha)}</span></>} — it follows the
            branch, so new pushes appear on the same link.
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
                Anyone with the link can view this branch{repo.private && " (even on this private repo)"} · expires in{" "}
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
