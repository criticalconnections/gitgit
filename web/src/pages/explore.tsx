import { useEffect, useRef, useState } from "react"
import { Link } from "react-router-dom"
import { CircleDot, GitPullRequest, Lock, Search, Star, Telescope } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card, CardFooter, CardHeader } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { EmptyState, ErrorNote, PageLoading } from "@/components/shared"
import { api, type Repo } from "@/lib/api"
import { timeAgo } from "@/lib/format"

function RepoCard({ repo }: { repo: Repo }) {
  return (
    <Card className="gap-3 py-5 transition-colors hover:border-primary/40">
      <CardHeader className="gap-1.5 px-5">
        <div className="flex min-w-0 items-center gap-2">
          <Link
            to={`/${repo.full_name}`}
            className="truncate font-semibold text-primary hover:underline"
          >
            {repo.full_name}
          </Link>
          {repo.private && (
            <Badge variant="outline" className="gap-1 rounded-full text-muted-foreground">
              <Lock className="size-3" /> Private
            </Badge>
          )}
        </div>
        <p className="line-clamp-2 min-h-10 text-sm text-muted-foreground">
          {repo.description || "No description provided."}
        </p>
      </CardHeader>
      <CardFooter className="gap-4 px-5 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1" title="Stars">
          <Star className="size-3.5" /> {repo.stars}
        </span>
        <span className="inline-flex items-center gap-1" title="Open pull requests">
          <GitPullRequest className="size-3.5" /> {repo.open_pulls}
        </span>
        <span className="inline-flex items-center gap-1" title="Open issues">
          <CircleDot className="size-3.5" /> {repo.open_issues}
        </span>
        <span className="ml-auto">{timeAgo(repo.created_at)}</span>
      </CardFooter>
    </Card>
  )
}

export default function Explore() {
  const [q, setQ] = useState("")
  const [repos, setRepos] = useState<Repo[] | null>(null)
  const [error, setError] = useState("")
  const firstLoad = useRef(true)

  useEffect(() => {
    const delay = firstLoad.current ? 0 : 250
    firstLoad.current = false
    let cancelled = false
    const t = setTimeout(async () => {
      try {
        const r = await api.listRepos(q.trim() || undefined)
        if (!cancelled) {
          setRepos(r)
          setError("")
        }
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "failed to load repositories")
      }
    }, delay)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
  }, [q])

  return (
    <div className="space-y-8">
      <header className="space-y-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Explore</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Discover repositories across this GitGit instance.
          </p>
        </div>
        <div className="relative max-w-md">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search repositories…"
            className="pl-9"
            aria-label="Search repositories"
          />
        </div>
      </header>

      {error ? (
        <ErrorNote message={error} />
      ) : repos === null ? (
        <PageLoading />
      ) : repos.length === 0 ? (
        <EmptyState icon={<Telescope />} title={q.trim() ? "No repositories match" : "Nothing here yet"}>
          {q.trim()
            ? `Nothing matched “${q.trim()}”. Try a different search.`
            : "Public repositories will appear here as people create them."}
        </EmptyState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {repos.map((r) => (
            <RepoCard key={r.id} repo={r} />
          ))}
        </div>
      )}
    </div>
  )
}
