import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { BookOpen, CircleDot, GitPullRequest, Lock, ShieldCheck, Star } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card, CardFooter, CardHeader } from "@/components/ui/card"
import { EmptyState, ErrorNote, PageLoading, UserAvatar } from "@/components/shared"
import { api, type Repo, type UserProfile } from "@/lib/api"
import { formatDate, timeAgo } from "@/lib/format"

function RepoCard({ repo }: { repo: Repo }) {
  return (
    <Card className="gap-3 py-5 transition-colors hover:border-primary/40">
      <CardHeader className="gap-1.5 px-5">
        <div className="flex min-w-0 items-center gap-2">
          <Link
            to={`/${repo.full_name}`}
            className="truncate font-semibold text-primary hover:underline"
          >
            {repo.name}
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

export default function Profile() {
  const { owner = "" } = useParams()
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    let cancelled = false
    setProfile(null)
    setError("")
    api
      .userProfile(owner)
      .then((p) => {
        if (!cancelled) setProfile(p)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : "user not found")
      })
    return () => {
      cancelled = true
    }
  }, [owner])

  if (error) return <ErrorNote message={error} />
  if (!profile) return <PageLoading />

  const { user, repos } = profile

  return (
    <div className="space-y-8">
      <header className="flex items-center gap-5">
        <UserAvatar user={user} className="size-16" />
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-semibold tracking-tight">
            {user.full_name || user.username}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
            <span>@{user.username}</span>
            {user.is_admin && (
              <Badge variant="outline" className="gap-1 rounded-full text-muted-foreground">
                <ShieldCheck className="size-3" /> site admin
              </Badge>
            )}
            <span>joined {formatDate(user.created_at)}</span>
          </div>
        </div>
      </header>

      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <h2 className="text-base font-semibold tracking-tight">Repositories</h2>
          {repos.length > 0 && (
            <Badge variant="secondary" className="rounded-full px-2">
              {repos.length}
            </Badge>
          )}
        </div>
        {repos.length === 0 ? (
          <EmptyState icon={<BookOpen />} title="No repositories yet">
            @{user.username} hasn&rsquo;t created any repositories you can see.
          </EmptyState>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {repos.map((r) => (
              <RepoCard key={r.id} repo={r} />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
