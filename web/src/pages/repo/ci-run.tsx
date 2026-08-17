import { useCallback, useEffect, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { RotateCcw, Timer } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { api, type CIRun } from "@/lib/api"
import { duration, shortSha, timeAgo } from "@/lib/format"
import { CIBadge, ErrorNote, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"

export default function CIRunPage() {
  const { repo } = useRepo()
  const { number = "" } = useParams()
  const navigate = useNavigate()

  const [run, setRun] = useState<CIRun | null>(null)
  const [error, setError] = useState("")
  const [rerunning, setRerunning] = useState(false)

  const load = useCallback(async () => {
    try {
      setRun(await api.run(repo.owner, repo.name, number))
      setError("")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load CI run")
    }
  }, [repo.owner, repo.name, number])

  useEffect(() => {
    setRun(null)
    load()
  }, [load])

  // Live view: poll while the run is still in flight.
  const live = run !== null && (run.status === "queued" || run.status === "running")
  useEffect(() => {
    if (!live) return
    const timer = setInterval(load, 2000)
    return () => clearInterval(timer)
  }, [live, load])

  if (error) return <ErrorNote message={error} />
  if (!run) return <PageLoading />

  const base = `/${repo.full_name}`
  const dur = duration(run.started_at, run.finished_at)

  const rerun = async () => {
    if (rerunning) return
    setRerunning(true)
    try {
      const next = await api.rerun(repo.owner, repo.name, run.number)
      navigate(`${base}/ci/${next.number}`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to re-run")
    } finally {
      setRerunning(false)
    }
  }

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* header */}
      <div className="flex flex-wrap items-center gap-3">
        <CIBadge status={run.status} className="[&_svg]:size-7" />
        <h2 className="text-2xl font-semibold tracking-tight">Run #{run.number}</h2>
        <span className="text-sm text-muted-foreground capitalize">{run.status}</span>
        {live && (
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="size-2 animate-pulse rounded-full bg-tangerine" />
            auto-refreshing
          </span>
        )}
        {repo.can_write && (
          <Button variant="outline" size="sm" className="ml-auto" onClick={rerun} disabled={rerunning}>
            <RotateCcw className="size-4" />
            {rerunning ? "Re-running…" : "Re-run"}
          </Button>
        )}
      </div>

      {/* meta */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-sm text-muted-foreground">
        <Badge variant="outline" className="rounded-full">
          {run.event}
        </Badge>
        <span className="font-mono text-xs">
          {run.ref} @{" "}
          <Link to={`${base}/commit/${run.commit}`} className="hover:text-primary hover:underline">
            {shortSha(run.commit)}
          </Link>
        </span>
        {run.commit_info && <span className="truncate text-foreground">{run.commit_info.subject}</span>}
        <span>queued {timeAgo(run.created_at)}</span>
        {dur && (
          <span className="flex items-center gap-1">
            <Timer className="size-3.5" />
            {dur}
          </span>
        )}
      </div>

      {/* jobs */}
      <div className="space-y-4">
        {(run.jobs ?? []).map((job) => {
          const failed = job.status === "failure" || job.status === "error" || job.status === "timeout"
          const jobDur = duration(job.started_at, job.finished_at)
          return (
            <Card key={job.id} className="gap-0 overflow-hidden py-0">
              <div className="flex flex-wrap items-center gap-2.5 border-b px-4 py-3">
                <CIBadge status={job.status} />
                <span className="text-sm font-semibold">{job.name}</span>
                <span className="text-xs text-muted-foreground capitalize">
                  {job.status}
                  {failed && ` · exit code ${job.exit_code}`}
                </span>
                {jobDur && <span className="ml-auto text-xs text-muted-foreground">{jobDur}</span>}
              </div>
              <pre className="max-h-[480px] overflow-auto rounded-b-xl bg-zinc-900 p-4 font-mono text-xs whitespace-pre-wrap text-zinc-200">
                {job.log ? job.log : <span className="text-zinc-500 italic">no output yet</span>}
              </pre>
            </Card>
          )
        })}
        {(run.jobs ?? []).length === 0 && (
          <p className="rounded-xl border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
            No jobs recorded for this run.
          </p>
        )}
      </div>
    </div>
  )
}
