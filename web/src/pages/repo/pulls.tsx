import { useEffect, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { GitPullRequest, MessageSquare, Plus } from "lucide-react"
import { Button } from "@/components/ui/button"
import { api, type Pull } from "@/lib/api"
import { CIBadge, EmptyState, ErrorNote, PageLoading, PRStateBadge, UserLink } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"
import { timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

export default function PullsPage() {
  const { repo } = useRepo()
  const [searchParams, setSearchParams] = useSearchParams()
  const state = searchParams.get("state") === "closed" ? "closed" : "open"

  const [open, setOpen] = useState<Pull[] | null>(null)
  const [closed, setClosed] = useState<Pull[] | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    let live = true
    Promise.all([
      api.listPulls(repo.owner, repo.name, "open"),
      api.listPulls(repo.owner, repo.name, "closed"),
    ])
      .then(([o, c]) => {
        if (!live) return
        setOpen(o)
        setClosed(c)
        setError("")
      })
      .catch((e: unknown) => {
        if (live) setError(e instanceof Error ? e.message : "failed to load pull requests")
      })
    return () => {
      live = false
    }
  }, [repo.owner, repo.name])

  if (error) return <ErrorNote message={error} />
  if (!open || !closed) return <PageLoading />

  const pulls = state === "open" ? open : closed

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-semibold tracking-tight">Pull requests</h2>
        <Button asChild>
          <Link to={`/${repo.full_name}/compare`}>
            <Plus className="size-4" /> New pull request
          </Link>
        </Button>
      </div>

      <div className="flex gap-1 border-b">
        <FilterTab
          active={state === "open"}
          onClick={() => setSearchParams({})}
          label="Open"
          count={open.length}
        />
        <FilterTab
          active={state === "closed"}
          onClick={() => setSearchParams({ state: "closed" })}
          label="Closed"
          count={closed.length}
        />
      </div>

      {pulls.length === 0 ? (
        <EmptyState icon={<GitPullRequest />} title={state === "open" ? "No open pull requests" : "No closed pull requests"}>
          {state === "open" ? (
            <>
              Pull requests let you propose, review, and merge changes.{" "}
              <Link to={`/${repo.full_name}/compare`} className="text-primary hover:underline">
                Compare two branches
              </Link>{" "}
              to open your first one.
            </>
          ) : (
            <>Merged and closed pull requests will show up here.</>
          )}
        </EmptyState>
      ) : (
        <div className="divide-y rounded-xl border bg-card">
          {pulls.map((p) => (
            <PullRow key={p.id} pull={p} repoFullName={repo.full_name} />
          ))}
        </div>
      )}
    </div>
  )
}

function FilterTab({
  active,
  onClick,
  label,
  count,
}: {
  active: boolean
  onClick: () => void
  label: string
  count: number
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center gap-1.5 border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground",
        active && "border-tangerine font-semibold text-foreground",
      )}
    >
      {label}
      <span
        className={cn(
          "rounded-full bg-muted px-2 py-0.5 text-xs font-medium",
          active && "bg-secondary text-secondary-foreground",
        )}
      >
        {count}
      </span>
    </button>
  )
}

function PullRow({ pull, repoFullName }: { pull: Pull; repoFullName: string }) {
  return (
    <div className="flex items-start gap-3 px-4 py-3.5">
      <PRStateBadge state={pull.state} className="mt-0.5 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <Link to={`/${repoFullName}/pull/${pull.number}`} className="font-medium hover:text-primary hover:underline">
            {pull.title}
          </Link>
          <CIBadge status={pull.ci_status} runNumber={pull.ci_run} repo={repoFullName} />
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
          <span>
            #{pull.number} opened {timeAgo(pull.created_at)} by
          </span>
          <UserLink user={pull.author} className="text-xs" />
          <span>·</span>
          <span className="font-mono">
            {pull.base} ← {pull.head}
          </span>
        </div>
      </div>
      {pull.comments > 0 && (
        <span className="mt-1 flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
          <MessageSquare className="size-3.5" />
          {pull.comments}
        </span>
      )}
    </div>
  )
}
