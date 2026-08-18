import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { BookOpen, Building2, CircleDot, GitPullRequest, Lock, ShieldCheck, Star, UserPlus, X } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card, CardFooter, CardHeader } from "@/components/ui/card"
import { EmptyState, ErrorNote, PageLoading, UserAvatar } from "@/components/shared"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, type OrgMember, type Repo, type UserProfile } from "@/lib/api"
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

// MembersSection lists an organization's members. Owners administer every
// repository the organization owns, so promoting somebody is a real grant and
// the roles are shown plainly rather than implied.
function MembersSection({ org, canAdmin }: { org: string; canAdmin: boolean }) {
  const [members, setMembers] = useState<OrgMember[] | null>(null)
  const [username, setUsername] = useState("")
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setMembers(await api.orgMembers(org))
    } catch {
      setMembers([])
    }
  }, [org])

  useEffect(() => {
    load()
  }, [load])

  async function act(fn: () => Promise<unknown>) {
    setBusy(true)
    try {
      await fn()
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed")
    } finally {
      setBusy(false)
    }
  }

  if (!members) return null

  return (
    <section className="space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="text-base font-semibold tracking-tight">Members</h2>
        <Badge variant="secondary" className="rounded-full px-2">
          {members.length}
        </Badge>
      </div>

      {canAdmin && (
        <div className="flex items-end gap-2">
          <Input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="username"
            className="max-w-56"
          />
          <Button
            variant="outline"
            disabled={busy || !username.trim()}
            onClick={() =>
              act(async () => {
                await api.addOrgMember(org, username.trim(), "member")
                setUsername("")
              })
            }
          >
            <UserPlus className="size-4" /> Add member
          </Button>
        </div>
      )}

      <div className="divide-y rounded-lg border">
        {members.map((m) => (
          <div key={m.username} className="flex items-center gap-3 px-3 py-2">
            <UserAvatar user={m} className="size-7" />
            <Link to={`/${m.username}`} className="min-w-0 flex-1 truncate font-medium hover:underline">
              {m.username}
            </Link>
            {canAdmin ? (
              <select
                value={m.role}
                disabled={busy}
                onChange={(e) => act(() => api.addOrgMember(org, m.username, e.target.value))}
                className="h-8 rounded-md border bg-background px-2 text-xs"
              >
                <option value="member">member</option>
                <option value="owner">owner</option>
              </select>
            ) : (
              <Badge variant="outline" className="rounded-full text-muted-foreground">
                {m.role}
              </Badge>
            )}
            {canAdmin && (
              <Button
                variant="ghost"
                size="icon"
                className="size-8 text-destructive hover:text-destructive"
                title={`Remove ${m.username}`}
                disabled={busy}
                onClick={() => act(() => api.removeOrgMember(org, m.username))}
              >
                <X className="size-4" />
              </Button>
            )}
          </div>
        ))}
      </div>
    </section>
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
  const isOrg = !!user.is_org

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
            {isOrg && (
              <Badge variant="outline" className="gap-1 rounded-full text-muted-foreground">
                <Building2 className="size-3" /> organization
              </Badge>
            )}
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

      {isOrg && <MembersSection org={user.username} canAdmin={!!profile.can_admin} />}
    </div>
  )
}
