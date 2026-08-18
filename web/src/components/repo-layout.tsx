import { createContext, useCallback, useContext, useEffect, useState } from "react"
import { Link, NavLink, Outlet, useParams } from "react-router-dom"
import {
  BookOpen,
  CircleDot,
  GitBranch,
  GitPullRequest,
  Layers,
  Lock,
  PlayCircle,
  Rocket,
  Settings,
  Star
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { api, type Repo } from "@/lib/api"
import { PageLoading, ErrorNote } from "@/components/shared"
import { useAuth } from "@/lib/auth"
import { cn } from "@/lib/utils"

interface RepoCtx {
  repo: Repo
  refresh: () => Promise<void>
}

const RepoContext = createContext<RepoCtx | null>(null)

export function useRepo(): RepoCtx {
  const ctx = useContext(RepoContext)
  if (!ctx) throw new Error("useRepo outside RepoLayout")
  return ctx
}

// RepoLayout loads the repository, renders the header + tab nav, and provides
// the repo to child routes via context.
export function RepoLayout() {
  const { owner = "", repo: name = "" } = useParams()
  const { user } = useAuth()
  const [repo, setRepo] = useState<Repo | null>(null)
  const [error, setError] = useState("")

  const refresh = useCallback(async () => {
    try {
      setRepo(await api.repo(owner, name))
      setError("")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load repository")
    }
  }, [owner, name])

  useEffect(() => {
    setRepo(null)
    refresh()
  }, [refresh])

  if (error) return <ErrorNote message={error} />
  if (!repo) return <PageLoading />

  const base = `/${repo.full_name}`
  const tabs = [
    { to: base, label: "Code", icon: BookOpen, end: true },
    { to: `${base}/pulls`, label: "Pull requests", icon: GitPullRequest, count: repo.open_pulls },
    { to: `${base}/stacks`, label: "Stacks", icon: Layers },
    { to: `${base}/issues`, label: "Issues", icon: CircleDot, count: repo.open_issues },
    { to: `${base}/ci`, label: "CI", icon: PlayCircle },
    { to: `${base}/deployments`, label: "Deployments", icon: Rocket },
    { to: `${base}/branches`, label: "Branches", icon: GitBranch },
    ...(repo.can_admin ? [{ to: `${base}/settings`, label: "Settings", icon: Settings }] : []),
  ]

  return (
    <RepoContext.Provider value={{ repo, refresh }}>
      <div className="-mx-4 -mt-6 border-b bg-muted/30 px-4 pt-5 sm:-mx-6 sm:px-6">
        <div className="mx-auto max-w-7xl">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="flex items-center gap-1.5 text-xl">
              <Link to={`/${repo.owner}`} className="text-primary hover:underline">
                {repo.owner}
              </Link>
              <span className="text-muted-foreground">/</span>
              <Link to={base} className="font-semibold text-primary hover:underline">
                {repo.name}
              </Link>
            </h1>
            {repo.private && (
              <Badge variant="outline" className="gap-1 rounded-full text-muted-foreground">
                <Lock className="size-3" /> Private
              </Badge>
            )}
            <div className="ml-auto">
              <Button
                variant="outline"
                size="sm"
                disabled={!user}
                onClick={async () => {
                  await api.star(repo.owner, repo.name, !repo.starred)
                  refresh()
                }}
              >
                <Star className={cn("size-4", repo.starred && "fill-tangerine text-tangerine")} />
                {repo.starred ? "Starred" : "Star"}
                <Badge variant="secondary" className="ml-1 rounded-full px-2">
                  {repo.stars}
                </Badge>
              </Button>
            </div>
          </div>
          {repo.description && <p className="mt-1 text-sm text-muted-foreground">{repo.description}</p>}
          <nav className="mt-3 flex gap-1 overflow-x-auto">
            {tabs.map((t) => (
              <NavLink
                key={t.to}
                to={t.to}
                end={t.end}
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-1.5 rounded-t-lg border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap text-muted-foreground hover:bg-muted hover:text-foreground",
                    isActive && "border-tangerine font-semibold text-foreground",
                  )
                }
              >
                <t.icon className="size-4" />
                {t.label}
                {t.count !== undefined && t.count > 0 && (
                  <Badge variant="secondary" className="rounded-full px-1.5 py-0 text-xs">
                    {t.count}
                  </Badge>
                )}
              </NavLink>
            ))}
          </nav>
        </div>
      </div>
      <div className="pt-6">
        <Outlet />
      </div>
    </RepoContext.Provider>
  )
}
