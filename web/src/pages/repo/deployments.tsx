import { useCallback, useEffect, useState } from "react"
import { toast } from "sonner"
import { CheckCircle2, ExternalLink, Loader2, Rocket, ScrollText, ShieldAlert, XCircle } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { EmptyState, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"
import { api, type DeployEnvironment, type Deployment } from "@/lib/api"
import { shortSha } from "@/lib/format"
import { timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

const STATUS = {
  queued: { icon: Loader2, className: "text-muted-foreground", label: "Queued" },
  running: { icon: Loader2, className: "text-tangerine animate-spin", label: "Running" },
  success: { icon: CheckCircle2, className: "text-primary", label: "Deployed" },
  failure: { icon: XCircle, className: "text-destructive", label: "Failed" },
} as const

function StatusIcon({ status }: { status: Deployment["status"] }) {
  const meta = STATUS[status] ?? STATUS.queued
  const Icon = meta.icon
  return <Icon className={cn("size-4 shrink-0", meta.className)} />
}

function LogDialog({ owner, name, id }: { owner: string; name: string; id: number }) {
  const [log, setLog] = useState<string | null>(null)
  return (
    <div className="space-y-2">
      <Button
        variant="ghost"
        size="sm"
        className="h-7 px-2 text-xs"
        onClick={async () => {
          if (log !== null) return setLog(null)
          try {
            const d = await api.deployment(owner, name, id)
            setLog(d.log || "no output")
          } catch {
            setLog("could not load the log")
          }
        }}
      >
        <ScrollText className="size-3.5" /> {log === null ? "Log" : "Hide"}
      </Button>
      {log !== null && (
        <pre className="max-h-72 overflow-auto rounded-lg bg-zinc-900 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-zinc-200">
          {log}
        </pre>
      )}
    </div>
  )
}

export default function Deployments() {
  const { repo } = useRepo()
  const [envs, setEnvs] = useState<DeployEnvironment[] | null>(null)
  const [history, setHistory] = useState<Deployment[]>([])
  const [busy, setBusy] = useState("")

  const load = useCallback(async () => {
    try {
      const res = await api.deployments(repo.owner, repo.name)
      setEnvs(res.environments)
      setHistory(res.deployments)
    } catch {
      setEnvs([])
    }
  }, [repo.owner, repo.name])

  useEffect(() => {
    load()
  }, [load])

  // keep polling while something is in flight
  useEffect(() => {
    if (!history.some((d) => d.status === "queued" || d.status === "running")) return
    const t = setInterval(load, 2000)
    return () => clearInterval(t)
  }, [history, load])

  if (!envs) return <PageLoading />

  const deploy = async (environment: string) => {
    setBusy(environment)
    try {
      await api.deploy(repo.owner, repo.name, environment, repo.default_branch)
      toast.success(`Deploying ${repo.default_branch} to ${environment}`)
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed")
    } finally {
      setBusy("")
    }
  }

  return (
    <div className="space-y-8">
      <section className="space-y-3">
        <h2 className="text-base font-semibold tracking-tight">Environments</h2>
        {envs.length === 0 ? (
          <EmptyState icon={<Rocket />} title="No environments declared">
            Add <code className="font-mono">.gitgit/deploy.yml</code> to {repo.default_branch} with
            an <code className="font-mono">environments:</code> block, and they will appear here.
          </EmptyState>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {envs.map((e) => (
              <div key={e.name} className="rounded-lg border p-4">
                <div className="flex items-center gap-2">
                  <Rocket className="size-4 text-primary" />
                  <span className="font-semibold">{e.name}</span>
                  {e.require_approval && (
                    <Badge variant="outline" className="gap-1 rounded-full text-muted-foreground">
                      <ShieldAlert className="size-3" /> manual only
                    </Badge>
                  )}
                  {!e.require_approval && e.auto_deploy && (
                    <Badge variant="secondary" className="rounded-full text-xs">
                      auto from {e.auto_deploy}
                    </Badge>
                  )}
                </div>

                <div className="mt-2 text-sm text-muted-foreground">
                  {e.current ? (
                    <span className="inline-flex flex-wrap items-center gap-1.5">
                      <StatusIcon status={e.current.status} />
                      <span className="font-mono">{shortSha(e.current.commit)}</span>
                      <span>from {e.current.ref}</span>
                      <span>· {timeAgo(e.current.finished_at || e.current.created_at)}</span>
                    </span>
                  ) : (
                    "Nothing deployed yet."
                  )}
                </div>

                <div className="mt-3 flex items-center gap-2">
                  {repo.can_write && (
                    <Button size="sm" disabled={busy === e.name} onClick={() => deploy(e.name)}>
                      <Rocket className="size-3.5" />
                      Deploy {repo.default_branch}
                    </Button>
                  )}
                  {e.url && (
                    <Button asChild variant="ghost" size="sm">
                      <a href={e.url} target="_blank" rel="noreferrer">
                        <ExternalLink className="size-3.5" /> Visit
                      </a>
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="text-base font-semibold tracking-tight">History</h2>
        {history.length === 0 ? (
          <p className="text-sm text-muted-foreground">No deployments yet.</p>
        ) : (
          <div className="divide-y rounded-lg border">
            {history.map((d) => (
              <div key={d.id} className="space-y-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <StatusIcon status={d.status} />
                  <span className="font-medium">{d.environment}</span>
                  <span className="font-mono text-xs text-muted-foreground">{shortSha(d.commit)}</span>
                  <span className="text-xs text-muted-foreground">from {d.ref}</span>
                  {d.creator && <span className="text-xs text-muted-foreground">· by {d.creator}</span>}
                  <span className="ml-auto text-xs text-muted-foreground">{timeAgo(d.created_at)}</span>
                </div>
                <LogDialog owner={repo.owner} name={repo.name} id={d.id} />
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
