import { useEffect, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { ArrowLeftRight, GitCompareArrows, GitPullRequest } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, type CompareResponse } from "@/lib/api"
import { shortSha, timeAgo } from "@/lib/format"
import { CIBadge, EmptyState, ErrorNote, PageLoading } from "@/components/shared"
import { DiffStatLine, DiffView } from "@/components/diff-view"
import { useRepo } from "@/components/repo-layout"

export default function Page() {
  const { repo } = useRepo()
  const [searchParams, setSearchParams] = useSearchParams()
  const base = searchParams.get("base") || repo.default_branch
  const head = searchParams.get("head") || ""

  const [result, setResult] = useState<CompareResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  const repoBase = `/${repo.full_name}`
  const branches = repo.branches ?? []

  const setRange = (nextBase: string, nextHead: string) => {
    const next = new URLSearchParams(searchParams)
    next.set("base", nextBase)
    if (nextHead) next.set("head", nextHead)
    else next.delete("head")
    setSearchParams(next)
  }

  useEffect(() => {
    if (!head) {
      setResult(null)
      setError("")
      return
    }
    let live = true
    setLoading(true)
    setError("")
    api
      .compare(repo.owner, repo.name, base, head)
      .then((r) => {
        if (live) setResult(r)
      })
      .catch((e: unknown) => {
        if (live) {
          setResult(null)
          setError(e instanceof Error ? e.message : "comparison failed")
        }
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [repo.owner, repo.name, base, head])

  const ahead = result?.ahead ?? 0
  const behind = result?.behind ?? 0
  const commits = result?.commits ?? []
  const diff = result?.diff
  const identical = result !== null && ahead === 0 && commits.length === 0 && (diff?.files.length ?? 0) === 0

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">Compare changes</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Pick two branches to see what’s changed, then open a pull request.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2 rounded-xl border bg-card p-4">
        <span className="text-sm text-muted-foreground">base</span>
        <Select value={base} onValueChange={(v) => setRange(v, head)}>
          <SelectTrigger size="sm" className="max-w-52 font-mono text-xs">
            <SelectValue placeholder="base branch" />
          </SelectTrigger>
          <SelectContent>
            {branches.map((b) => (
              <SelectItem key={b.name} value={b.name} className="font-mono text-xs">
                {b.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="ghost"
          size="icon"
          className="size-8 text-muted-foreground"
          title="Swap base and head"
          disabled={!head}
          onClick={() => setRange(head, base)}
        >
          <ArrowLeftRight className="size-4" />
        </Button>
        <span className="text-sm text-muted-foreground">head</span>
        <Select value={head} onValueChange={(v) => setRange(base, v)}>
          <SelectTrigger size="sm" className="max-w-52 font-mono text-xs">
            <SelectValue placeholder="select a branch…" />
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

      {!head ? (
        <EmptyState icon={<GitCompareArrows />} title="Choose a head branch">
          Select the branch with your changes to compare it against{" "}
          <span className="font-mono">{base}</span>.
        </EmptyState>
      ) : loading ? (
        <PageLoading />
      ) : error ? (
        <ErrorNote message={error} />
      ) : identical ? (
        <EmptyState icon={<GitCompareArrows />} title="These branches are identical">
          <span className="font-mono">{head}</span> has nothing to compare with{" "}
          <span className="font-mono">{base}</span> — there’s no pull request to open here.
        </EmptyState>
      ) : result ? (
        <div className="space-y-5">
          <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card px-4 py-3">
            <GitCompareArrows className="size-4 shrink-0 text-primary" />
            <p className="text-sm">
              <span className="font-mono font-medium">{head}</span> is{" "}
              <span className="font-semibold">
                {ahead} commit{ahead === 1 ? "" : "s"} ahead
              </span>
              {behind > 0 && (
                <span className="text-muted-foreground">
                  {" "}
                  ({behind} behind)
                </span>
              )}{" "}
              of <span className="font-mono font-medium">{base}</span>
            </p>
            <div className="ml-auto">
              {result.existing_pull ? (
                <Button asChild size="sm" variant="outline">
                  <Link to={`${repoBase}/pull/${result.existing_pull}`}>
                    <GitPullRequest className="size-4" />
                    View PR #{result.existing_pull}
                  </Link>
                </Button>
              ) : ahead > 0 ? (
                <Button asChild size="sm">
                  <Link
                    to={`${repoBase}/pulls/new?base=${encodeURIComponent(base)}&head=${encodeURIComponent(head)}`}
                  >
                    <GitPullRequest className="size-4" />
                    Create pull request
                  </Link>
                </Button>
              ) : null}
            </div>
          </div>

          {commits.length > 0 && (
            <div className="divide-y overflow-hidden rounded-xl border bg-card">
              {commits.map((c) => (
                <div key={c.sha} className="flex items-center gap-3 px-4 py-3">
                  <CIBadge status={c.ci_status} runNumber={c.ci_run} repo={repo.full_name} className="shrink-0" />
                  <div className="min-w-0 flex-1">
                    <Link
                      to={`${repoBase}/commit/${c.sha}`}
                      className="block truncate text-sm font-medium hover:underline"
                    >
                      {c.subject}
                    </Link>
                    <p className="truncate text-xs text-muted-foreground">
                      {c.author_name} committed {timeAgo(c.when)}
                    </p>
                  </div>
                  <Link
                    to={`${repoBase}/commit/${c.sha}`}
                    className="shrink-0 rounded-md border bg-muted/50 px-2 py-0.5 font-mono text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
                  >
                    {shortSha(c.sha)}
                  </Link>
                </div>
              ))}
            </div>
          )}

          {diff && diff.files.length > 0 && (
            <>
              <DiffStatLine diff={diff} />
              <DiffView diff={diff} />
            </>
          )}
        </div>
      ) : null}
    </div>
  )
}
