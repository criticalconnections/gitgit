import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { Layers } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { api, type StackItem } from "@/lib/api"
import { CIBadge, EmptyState, ErrorNote, PageLoading, UserLink } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"
import { cn } from "@/lib/utils"

export default function StacksPage() {
  const { repo } = useRepo()
  const [items, setItems] = useState<StackItem[] | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    let live = true
    api
      .stacks(repo.owner, repo.name)
      .then((s) => {
        if (live) setItems(s)
      })
      .catch((e: unknown) => {
        if (live) setError(e instanceof Error ? e.message : "failed to load stacks")
      })
    return () => {
      live = false
    }
  }, [repo.owner, repo.name])

  if (error) return <ErrorNote message={error} />
  if (!items) return <PageLoading />

  // successive runs starting at depth 0 form one stack each
  const groups: StackItem[][] = []
  for (const item of items) {
    if (item.depth === 0 || groups.length === 0) groups.push([item])
    else groups[groups.length - 1].push(item)
  }

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div>
        <h2 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
          <Layers className="size-5 text-tangerine" /> Stacks
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Stacked pull requests keep big changes reviewable — branch off a PR's head, open a PR
          targeting that branch, and merge from the bottom up.
        </p>
      </div>

      {items.length === 0 ? (
        <EmptyState icon={<Layers />} title="No open pull requests to stack">
          <div className="space-y-2 text-left">
            <p>To stack a change on top of an open PR:</p>
            <ol className="list-decimal space-y-1 pl-5">
              <li>
                Branch off the PR's head branch:{" "}
                <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                  git switch -c my-next-step their-head
                </code>
              </li>
              <li>Push your commits and open a PR targeting that branch.</li>
              <li>Merge from the bottom up — bases retarget automatically.</li>
            </ol>
          </div>
        </EmptyState>
      ) : (
        <div className="space-y-4">
          {groups.map((group) => (
            <div key={group[0].number} className="rounded-xl border bg-card p-3">
              <div className="space-y-1">
                {group.map((item) => (
                  <StackRow key={item.number} item={item} repoFullName={repo.full_name} />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function StackRow({ item, repoFullName }: { item: StackItem; repoFullName: string }) {
  return (
    <div
      style={{ marginLeft: item.depth * 16 }}
      className={cn(
        "flex flex-wrap items-center gap-2 rounded-lg px-2 py-1.5 text-sm",
        item.current && "bg-muted/60 ring-1 ring-tangerine/60",
      )}
    >
      <span
        className={cn(
          "w-4 shrink-0 text-center font-mono text-sm select-none",
          item.depth === 0 ? "text-tangerine" : "text-muted-foreground",
        )}
        aria-hidden
      >
        {item.depth === 0 ? "⏚" : "↳"}
      </span>
      <CIBadge status={item.ci_status} />
      <Link
        to={`/${repoFullName}/pull/${item.number}`}
        className="min-w-0 font-medium hover:text-primary hover:underline"
      >
        <span className="text-muted-foreground">#{item.number}</span> {item.title}
      </Link>
      <span className="font-mono text-xs text-muted-foreground">
        {item.base} ← {item.head}
      </span>
      {item.current && (
        <Badge variant="secondary" className="rounded-full px-2 py-0 text-xs">
          this PR
        </Badge>
      )}
      {item.author && (
        <span className="ml-auto flex items-center gap-1 text-xs text-muted-foreground">
          by <UserLink user={item.author} className="text-xs" />
        </span>
      )}
    </div>
  )
}
