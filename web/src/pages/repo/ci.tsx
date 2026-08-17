import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { PlayCircle } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { api, type CIRun } from "@/lib/api"
import { shortSha, timeAgo } from "@/lib/format"
import { CIBadge, EmptyState, ErrorNote, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"

const SAMPLE_CI = `jobs:
  lint:
    run: npm run lint
  test:
    run: |
      npm ci
      npm test`

export default function CIPage() {
  const { repo } = useRepo()
  const [runs, setRuns] = useState<CIRun[] | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    let stale = false
    setRuns(null)
    setError("")
    api
      .listRuns(repo.owner, repo.name)
      .then((list) => {
        if (!stale) setRuns(list)
      })
      .catch((e: unknown) => {
        if (!stale) setError(e instanceof Error ? e.message : "failed to load CI runs")
      })
    return () => {
      stale = true
    }
  }, [repo.owner, repo.name])

  const base = `/${repo.full_name}`

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">CI runs</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Define jobs in <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">.gitgit/ci.yml</code> — every
          push runs them.
        </p>
      </div>

      {error ? (
        <ErrorNote message={error} />
      ) : runs === null ? (
        <PageLoading />
      ) : runs.length === 0 ? (
        <EmptyState icon={<PlayCircle />} title="No CI runs yet">
          <p>
            Commit a <code className="font-mono">.gitgit/ci.yml</code> to the repository and GitGit will run its jobs on
            every push:
          </p>
          <pre className="mt-4 overflow-x-auto rounded-lg border bg-muted/50 p-4 text-left font-mono text-xs leading-relaxed">
            {SAMPLE_CI}
          </pre>
        </EmptyState>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <ul className="divide-y">
            {runs.map((run) => (
              <li key={run.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 transition-colors hover:bg-muted/40">
                <CIBadge status={run.status} />
                <Link to={`${base}/ci/${run.number}`} className="font-medium hover:text-primary hover:underline">
                  Run #{run.number}
                </Link>
                <Badge variant="outline" className="rounded-full text-muted-foreground">
                  {run.event}
                </Badge>
                <span className="font-mono text-xs text-muted-foreground">
                  {run.ref} @{" "}
                  <Link to={`${base}/commit/${run.commit}`} className="hover:text-primary hover:underline">
                    {shortSha(run.commit)}
                  </Link>
                </span>
                <span className="text-xs text-muted-foreground">{timeAgo(run.created_at)}</span>
                <span className="ml-auto text-xs text-muted-foreground capitalize">{run.status}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
