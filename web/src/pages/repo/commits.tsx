import { useEffect, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { Check, ChevronDown, ChevronLeft, ChevronRight, GitBranch, History, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { api, type CommitsResponse } from "@/lib/api"
import { shortSha, timeAgo } from "@/lib/format"
import { CIBadge, EmptyState, ErrorNote, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"

const encodePath = (p: string) => p.split("/").map(encodeURIComponent).join("/")

export default function Page() {
  const { repo } = useRepo()
  const navigate = useNavigate()
  const splat = (useParams()["*"] ?? "").replace(/^\/+|\/+$/g, "")
  const ref = splat || repo.default_branch
  const [searchParams, setSearchParams] = useSearchParams()
  const pathFilter = searchParams.get("path") ?? ""
  const page = Math.max(1, Number(searchParams.get("page") ?? "1") || 1)

  const [data, setData] = useState<CommitsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  const base = `/${repo.full_name}`

  useEffect(() => {
    let live = true
    setLoading(true)
    setError("")
    api
      .commits(repo.owner, repo.name, ref, pathFilter, page)
      .then((d) => {
        if (live) setData(d)
      })
      .catch((e: unknown) => {
        if (live) setError(e instanceof Error ? e.message : "failed to load commits")
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [repo.owner, repo.name, ref, pathFilter, page])

  const setPage = (p: number) => {
    const next = new URLSearchParams(searchParams)
    if (p <= 1) next.delete("page")
    else next.set("page", String(p))
    setSearchParams(next)
  }

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-lg font-semibold tracking-tight">Commits</h2>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="max-w-56">
              <GitBranch className="size-4 text-muted-foreground" />
              <span className="truncate font-mono text-xs">{ref}</span>
              <ChevronDown className="size-3.5 text-muted-foreground" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-80 w-64 overflow-y-auto">
            <DropdownMenuLabel>Switch branches</DropdownMenuLabel>
            {(repo.branches ?? []).map((b) => (
              <DropdownMenuItem
                key={b.name}
                onSelect={() =>
                  navigate(
                    `${base}/commits/${encodePath(b.name)}${
                      pathFilter ? `?path=${encodeURIComponent(pathFilter)}` : ""
                    }`,
                  )
                }
              >
                <span className="w-4 shrink-0">{b.name === ref && <Check className="size-4 text-primary" />}</span>
                <span className="truncate font-mono text-xs">{b.name}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        {pathFilter && (
          <span className="inline-flex items-center gap-1.5 rounded-full border bg-muted/50 py-1 pr-1 pl-3 text-xs">
            <span className="text-muted-foreground">History for</span>
            <span className="font-mono font-medium">{pathFilter}</span>
            <button
              type="button"
              className="rounded-full p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
              onClick={() => {
                const next = new URLSearchParams(searchParams)
                next.delete("path")
                next.delete("page")
                setSearchParams(next)
              }}
              title="Clear filter"
            >
              <X className="size-3.5" />
            </button>
          </span>
        )}
      </div>

      {loading ? (
        <PageLoading />
      ) : error ? (
        <ErrorNote message={error} />
      ) : !data || data.commits.length === 0 ? (
        <EmptyState icon={<History />} title="No commits found">
          {pathFilter
            ? `No commits touch ${pathFilter} on ${ref}.`
            : `There are no commits on ${ref} yet.`}
        </EmptyState>
      ) : (
        <>
          <div className="divide-y overflow-hidden rounded-xl border bg-card">
            {data.commits.map((c) => (
              <div key={c.sha} className="flex items-center gap-3 px-4 py-3">
                <CIBadge status={c.ci_status} runNumber={c.ci_run} repo={repo.full_name} className="shrink-0" />
                <div className="min-w-0 flex-1">
                  <Link
                    to={`${base}/commit/${c.sha}`}
                    className="block truncate text-sm font-medium hover:underline"
                  >
                    {c.subject}
                  </Link>
                  <p className="truncate text-xs text-muted-foreground">
                    {c.author_name} committed {timeAgo(c.when)}
                  </p>
                </div>
                <Link
                  to={`${base}/commit/${c.sha}`}
                  className="shrink-0 rounded-md border bg-muted/50 px-2 py-0.5 font-mono text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  {shortSha(c.sha)}
                </Link>
              </div>
            ))}
          </div>

          {(page > 1 || data.has_next) && (
            <div className="flex items-center justify-center gap-2">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>
                <ChevronLeft className="size-4" />
                Newer
              </Button>
              <Button variant="outline" size="sm" disabled={!data.has_next} onClick={() => setPage(page + 1)}>
                Older
                <ChevronRight className="size-4" />
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
