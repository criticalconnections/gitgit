import { useCallback, useEffect, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { AtSign, Bell, CheckCheck, CircleDot, GitPullRequest, MessageSquare, PenLine, PlayCircle, Users } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { EmptyState, PageLoading } from "@/components/shared"
import { useAuth } from "@/lib/auth"
import { api, type Notification } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

// Each reason gets its own icon and words, because "why am I seeing this" is
// the question an inbox exists to answer.
const REASONS: Record<string, { icon: typeof Bell; label: string; className: string }> = {
  mention: { icon: AtSign, label: "You were mentioned", className: "text-tangerine" },
  author: { icon: PenLine, label: "You opened this", className: "text-primary" },
  review: { icon: GitPullRequest, label: "Reviewed", className: "text-primary" },
  ci: { icon: PlayCircle, label: "CI failed", className: "text-destructive" },
  comment: { icon: MessageSquare, label: "New activity", className: "text-muted-foreground" },
  repo: { icon: Users, label: "In a repository you maintain", className: "text-muted-foreground" },
}

export default function Notifications() {
  const { user, loading } = useAuth()
  const navigate = useNavigate()
  const [items, setItems] = useState<Notification[] | null>(null)
  const [showRead, setShowRead] = useState(true)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await api.notifications(showRead)
      setItems(res.notifications)
    } catch {
      setItems([])
    }
  }, [showRead])

  useEffect(() => {
    if (!loading && !user) navigate("/login")
  }, [loading, user, navigate])

  useEffect(() => {
    if (user) load()
  }, [user, load])

  if (loading || !user || !items) return <PageLoading />

  const unread = items.filter((n) => !n.read).length

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
          <Bell className="size-5 text-primary" /> Notifications
        </h1>
        {unread > 0 && (
          <Badge variant="secondary" className="rounded-full px-2">
            {unread} unread
          </Badge>
        )}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => setShowRead(!showRead)}>
            {showRead ? "Unread only" : "Show all"}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={busy || unread === 0}
            onClick={async () => {
              setBusy(true)
              try {
                await api.markAllNotificationsRead()
                await load()
              } finally {
                setBusy(false)
              }
            }}
          >
            <CheckCheck className="size-4" /> Mark all read
          </Button>
        </div>
      </header>

      {items.length === 0 ? (
        <EmptyState icon={<Bell />} title="Nothing to catch up on">
          You will hear about mentions, replies on your threads, reviews, and failing CI on your
          pull requests.
        </EmptyState>
      ) : (
        <div className="divide-y rounded-lg border">
          {items.map((n) => {
            const meta = REASONS[n.reason] ?? REASONS.comment
            const Icon = meta.icon
            const Subject = n.subject === "pull" ? GitPullRequest : CircleDot
            return (
              <Link
                key={n.id}
                to={n.url}
                onClick={() => api.markNotificationRead(n.id).catch(() => {})}
                className={cn(
                  "flex items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/50",
                  !n.read && "bg-primary/[0.03]",
                )}
              >
                <Icon className={cn("mt-0.5 size-4 shrink-0", meta.className)} />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    {!n.read && <span className="size-1.5 shrink-0 rounded-full bg-primary" />}
                    <span className={cn("truncate", !n.read && "font-medium")}>{n.title}</span>
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                    <span className="inline-flex items-center gap-1">
                      <Subject className="size-3" />
                      {n.repo}#{n.number}
                    </span>
                    <span>·</span>
                    <span>{meta.label}</span>
                    {n.actor && (
                      <>
                        <span>·</span>
                        <span>by {n.actor}</span>
                      </>
                    )}
                  </div>
                </div>
                <span className="shrink-0 text-xs text-muted-foreground">{timeAgo(n.updated_at)}</span>
              </Link>
            )
          })}
        </div>
      )}
    </div>
  )
}
