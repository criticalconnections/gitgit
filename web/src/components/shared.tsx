import { Link } from "react-router-dom"
import {
  CheckCircle2,
  XCircle,
  CircleDashed,
  CircleDot,
  AlertCircle,
  GitPullRequest,
  GitMerge,
  Loader2,
} from "lucide-react"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import type { CIStatus, UserRef, Label as ApiLabel } from "@/lib/api"

// ---------- CI status badge ----------

export function CIBadge({
  status,
  runNumber,
  repo,
  className,
  showLabel = false,
}: {
  status?: CIStatus | string
  runNumber?: number
  repo?: string // "owner/name" — makes the badge a link to the run
  className?: string
  showLabel?: boolean
}) {
  if (!status) return null
  const icons: Record<string, React.ReactNode> = {
    success: <CheckCircle2 className="size-4 text-primary" />,
    failure: <XCircle className="size-4 text-destructive" />,
    error: <AlertCircle className="size-4 text-destructive" />,
    timeout: <AlertCircle className="size-4 text-destructive" />,
    running: <Loader2 className="size-4 animate-spin text-tangerine" />,
    queued: <CircleDashed className="size-4 text-muted-foreground" />,
  }
  const icon = icons[status] ?? icons.queued
  const inner = (
    <span className={cn("inline-flex items-center gap-1.5 align-middle", className)} title={`CI: ${status}`}>
      {icon}
      {showLabel && <span className="text-sm text-muted-foreground capitalize">{status}</span>}
    </span>
  )
  if (repo && runNumber) {
    return (
      <Link to={`/${repo}/ci/${runNumber}`} className="hover:opacity-80">
        {inner}
      </Link>
    )
  }
  return inner
}

// ---------- PR / issue state badges ----------

export function PRStateBadge({ state, className }: { state: "open" | "merged" | "closed"; className?: string }) {
  const map = {
    open: { label: "Open", icon: <GitPullRequest className="size-3.5" />, cls: "bg-primary text-primary-foreground" },
    merged: { label: "Merged", icon: <GitMerge className="size-3.5" />, cls: "bg-violet-600 text-white" },
    closed: { label: "Closed", icon: <XCircle className="size-3.5" />, cls: "bg-destructive text-white" },
  }[state]
  return (
    <Badge className={cn("gap-1 rounded-full px-3 py-1", map.cls, className)}>
      {map.icon}
      {map.label}
    </Badge>
  )
}

export function IssueStateBadge({ state, className }: { state: "open" | "closed"; className?: string }) {
  return state === "open" ? (
    <Badge className={cn("gap-1 rounded-full bg-primary px-3 py-1 text-primary-foreground", className)}>
      <CircleDot className="size-3.5" /> Open
    </Badge>
  ) : (
    <Badge className={cn("gap-1 rounded-full bg-violet-600 px-3 py-1 text-white", className)}>
      <CheckCircle2 className="size-3.5" /> Closed
    </Badge>
  )
}

// ---------- labels ----------

export function LabelPill({ label, className }: { label: ApiLabel; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold text-white",
        className,
      )}
      style={{ backgroundColor: label.color }}
    >
      {label.name}
    </span>
  )
}

// ---------- avatars ----------

const AVATAR_HUES = [160, 60, 220, 280, 20, 340, 100]

export function UserAvatar({ user, className }: { user?: UserRef | null; className?: string }) {
  const name = user?.username ?? "?"
  const hue = AVATAR_HUES[(name.charCodeAt(0) || 0) % AVATAR_HUES.length]
  return (
    <Avatar className={cn("size-6", className)}>
      <AvatarFallback
        className="text-[0.65em] font-bold text-white uppercase"
        style={{ background: `oklch(0.62 0.13 ${hue})` }}
      >
        {name.slice(0, 2)}
      </AvatarFallback>
    </Avatar>
  )
}

export function UserLink({ user, className }: { user?: UserRef | null; className?: string }) {
  if (!user) return <span className={className}>ghost</span>
  return (
    <Link to={`/${user.username}`} className={cn("font-medium hover:underline", className)}>
      {user.username}
    </Link>
  )
}

// ---------- markdown ----------

// Renders server-sanitized HTML (goldmark escapes raw HTML).
export function Markdown({ html, className }: { html: string; className?: string }) {
  if (!html || html.trim() === "") return null
  return <div className={cn("markdown", className)} dangerouslySetInnerHTML={{ __html: html }} />
}

// ---------- empty / loading states ----------

export function EmptyState({
  icon,
  title,
  children,
  className,
}: {
  icon?: React.ReactNode
  title: string
  children?: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("flex flex-col items-center gap-2 rounded-xl border border-dashed py-16 text-center", className)}>
      {icon && <div className="text-muted-foreground [&_svg]:size-10 [&_svg]:stroke-[1.25]">{icon}</div>}
      <h3 className="text-lg font-semibold">{title}</h3>
      {children && <div className="max-w-md text-sm text-muted-foreground">{children}</div>}
    </div>
  )
}

export function PageLoading() {
  return (
    <div className="flex items-center justify-center py-24 text-muted-foreground">
      <Loader2 className="size-6 animate-spin" />
    </div>
  )
}

export function ErrorNote({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
      {message}
    </div>
  )
}
