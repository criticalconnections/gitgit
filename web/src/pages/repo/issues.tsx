import { useEffect, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { CheckCircle2, CircleDot, MessageSquare, Plus } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api, type Issue } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { EmptyState, ErrorNote, LabelPill, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"

export default function IssuesPage() {
  const { repo } = useRepo()
  const [searchParams, setSearchParams] = useSearchParams()
  const state = searchParams.get("state") === "closed" ? "closed" : "open"

  const [issues, setIssues] = useState<Issue[] | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    let stale = false
    setIssues(null)
    setError("")
    api
      .listIssues(repo.owner, repo.name, state)
      .then((list) => {
        if (!stale) setIssues(list)
      })
      .catch((e: unknown) => {
        if (!stale) setError(e instanceof Error ? e.message : "failed to load issues")
      })
    return () => {
      stale = true
    }
  }, [repo.owner, repo.name, state])

  const base = `/${repo.full_name}`

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-2xl font-semibold tracking-tight">Issues</h2>
        <Button asChild size="sm" className="ml-auto">
          <Link to={`${base}/issues/new`}>
            <Plus className="size-4" />
            New issue
          </Link>
        </Button>
      </div>

      <Tabs value={state} onValueChange={(v) => setSearchParams(v === "open" ? {} : { state: v })}>
        <TabsList>
          <TabsTrigger value="open" className="gap-1.5">
            <CircleDot className="size-4" />
            Open
          </TabsTrigger>
          <TabsTrigger value="closed" className="gap-1.5">
            <CheckCircle2 className="size-4" />
            Closed
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {error ? (
        <ErrorNote message={error} />
      ) : issues === null ? (
        <PageLoading />
      ) : issues.length === 0 ? (
        <EmptyState icon={<CircleDot />} title={state === "open" ? "No open issues" : "No closed issues"}>
          {state === "open" ? (
            <p>
              Track bugs, ideas, and tasks for this repository.{" "}
              <Link to={`${base}/issues/new`} className="font-medium text-primary hover:underline">
                Open the first issue
              </Link>{" "}
              to get the conversation going.
            </p>
          ) : (
            <p>Closed issues will show up here once conversations wrap up.</p>
          )}
        </EmptyState>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <ul className="divide-y">
            {issues.map((issue) => (
              <li key={issue.id} className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/40">
                {issue.state === "open" ? (
                  <CircleDot className="mt-0.5 size-4 shrink-0 text-primary" />
                ) : (
                  <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-violet-600" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <Link to={`${base}/issue/${issue.number}`} className="font-medium hover:text-primary hover:underline">
                      {issue.title}
                    </Link>
                    {issue.labels.map((l) => (
                      <LabelPill key={l.id} label={l} />
                    ))}
                  </div>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    #{issue.number} opened {timeAgo(issue.created_at)} by {issue.author.username}
                  </p>
                </div>
                {issue.comments > 0 && (
                  <span className="flex shrink-0 items-center gap-1 pt-0.5 text-xs text-muted-foreground">
                    <MessageSquare className="size-3.5" />
                    {issue.comments}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
