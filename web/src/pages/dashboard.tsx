import { useEffect, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { CircleDot, Compass, FolderGit2, GitPullRequest, Lock, Plus, Star } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardFooter, CardHeader } from "@/components/ui/card"
import { EmptyState, ErrorNote, PageLoading } from "@/components/shared"
import { api, type Repo } from "@/lib/api"
import { useAuth } from "@/lib/auth"
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

export default function Dashboard() {
  const { user, loading } = useAuth()
  const navigate = useNavigate()
  const [repos, setRepos] = useState<Repo[] | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!loading && !user) navigate("/login")
  }, [loading, user, navigate])

  useEffect(() => {
    if (!user) return
    let cancelled = false
    api
      .listRepos()
      .then((r) => {
        if (!cancelled) setRepos(r)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : "failed to load repositories")
      })
    return () => {
      cancelled = true
    }
  }, [user])

  if (loading || !user) return <PageLoading />
  if (error) return <ErrorNote message={error} />
  if (!repos) return <PageLoading />

  const yours = repos.filter((r) => r.can_write)
  const discover = repos.filter((r) => !r.can_write)

  return (
    <div className="space-y-10">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Good to see you, {user.username}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Here&rsquo;s what&rsquo;s happening across your repositories.
          </p>
        </div>
        <Button asChild>
          <Link to="/new">
            <Plus className="size-4" />
            New repository
          </Link>
        </Button>
      </header>

      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <h2 className="text-base font-semibold tracking-tight">Your repositories</h2>
          {yours.length > 0 && (
            <Badge variant="secondary" className="rounded-full px-2">
              {yours.length}
            </Badge>
          )}
        </div>
        {yours.length === 0 ? (
          <EmptyState icon={<FolderGit2 />} title="Create your first repository">
            <p>Repositories hold your code, pull requests, issues and CI — all in one place.</p>
            <Button asChild className="mt-4">
              <Link to="/new">
                <Plus className="size-4" />
                New repository
              </Link>
            </Button>
          </EmptyState>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {yours.map((r) => (
              <RepoCard key={r.id} repo={r} />
            ))}
          </div>
        )}
      </section>

      {discover.length > 0 && (
        <section className="space-y-4">
          <div className="flex items-center gap-2">
            <Compass className="size-4 text-muted-foreground" />
            <h2 className="text-base font-semibold tracking-tight">Discover</h2>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            {discover.map((r) => (
              <RepoCard key={r.id} repo={r} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
