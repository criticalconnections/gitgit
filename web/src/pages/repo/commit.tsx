import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { FolderTree, GitCommitHorizontal } from "lucide-react"
import { Button } from "@/components/ui/button"
import { api, type CommitResponse } from "@/lib/api"
import { formatDateTime, shortSha } from "@/lib/format"
import { CIBadge, ErrorNote, PageLoading } from "@/components/shared"
import { DiffStatLine, DiffView } from "@/components/diff-view"
import { useRepo } from "@/components/repo-layout"

export default function Page() {
  const { repo } = useRepo()
  const { sha = "" } = useParams()
  const [data, setData] = useState<CommitResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  const base = `/${repo.full_name}`

  useEffect(() => {
    let live = true
    setLoading(true)
    setError("")
    setData(null)
    api
      .commit(repo.owner, repo.name, sha)
      .then((d) => {
        if (live) setData(d)
      })
      .catch((e: unknown) => {
        if (live) setError(e instanceof Error ? e.message : "failed to load commit")
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [repo.owner, repo.name, sha])

  if (loading) return <PageLoading />
  if (error) return <ErrorNote message={error} />
  if (!data) return null

  const c = data.commit
  const parents = c.parents ?? []

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <div className="rounded-xl border bg-card p-5 sm:p-6">
        <div className="flex flex-wrap items-start gap-3">
          <GitCommitHorizontal className="mt-1 size-5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h2 className="text-lg font-semibold tracking-tight break-words">{c.subject}</h2>
            {c.body && (
              <p className="mt-2 text-sm whitespace-pre-wrap text-muted-foreground">{c.body}</p>
            )}
          </div>
          <Button asChild variant="outline" size="sm" className="shrink-0">
            <Link to={`${base}/tree/${c.sha}`}>
              <FolderTree className="size-4" />
              Browse files
            </Link>
          </Button>
        </div>

        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1.5 border-t pt-4 text-sm text-muted-foreground">
          <span>
            <span className="font-medium text-foreground">{c.author_name}</span>{" "}
            <span className="hidden sm:inline">&lt;{c.author_email}&gt;</span>
          </span>
          <span>{formatDateTime(c.when)}</span>
          <CIBadge status={data.ci_status} runNumber={data.ci_run} repo={repo.full_name} />
          <span className="ml-auto flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs">
            {parents.map((p) => (
              <Link key={p} to={`${base}/commit/${p}`} className="hover:text-foreground">
                parent <span className="text-primary">{shortSha(p)}</span>
              </Link>
            ))}
            <span title={c.sha}>commit {c.sha}</span>
          </span>
        </div>
      </div>

      <DiffStatLine diff={data.diff} />
      <DiffView diff={data.diff} />
    </div>
  )
}
