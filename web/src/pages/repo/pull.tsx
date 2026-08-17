import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import {
  AlertTriangle,
  ArrowUpFromDot,
  Check,
  CheckCircle2,
  FileDiff,
  GitCommitHorizontal,
  GitMerge,
  Info,
  Layers,
  MessageSquare,
  Pencil,
  RefreshCw,
  XCircle,
} from "lucide-react"
import { toast } from "sonner"
import { PreviewDialog } from "@/components/preview-dialog"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import {
  api,
  type Commit,
  type PullDetail,
  type PullFilesResponse,
  type Repo,
  type StackItem,
  type TimelineItem,
  type User,
} from "@/lib/api"
import {
  CIBadge,
  EmptyState,
  ErrorNote,
  Markdown,
  PageLoading,
  PRStateBadge,
  UserAvatar,
  UserLink,
} from "@/components/shared"
import { DiffStatLine, DiffView } from "@/components/diff-view"
import { useRepo } from "@/components/repo-layout"
import { useAuth } from "@/lib/auth"
import { shortSha, timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"

// ---------------------------------------------------------------- page

export default function PullPage() {
  const { repo, refresh: refreshRepo } = useRepo()
  const { user } = useAuth()
  const { number: numberParam = "", tab } = useParams()
  const number = Number(numberParam)

  const [pull, setPull] = useState<PullDetail | null>(null)
  const [error, setError] = useState("")

  const load = useCallback(async () => {
    try {
      setPull(await api.pull(repo.owner, repo.name, number))
      setError("")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load pull request")
    }
  }, [repo.owner, repo.name, number])

  useEffect(() => {
    setPull(null)
    load()
  }, [load])

  if (error) return <ErrorNote message={error} />
  if (!pull) return <PageLoading />

  const canEdit = !!user && (user.id === pull.author.id || repo.can_write)
  const activeTab = tab === "files" || tab === "commits" ? tab : "conversation"

  // full refetch after state-changing mutations (merge/close/reopen also
  // affect the repo header's open-PR count)
  const refetchAll = async () => {
    await Promise.all([load(), refreshRepo()])
  }

  return (
    <div className="space-y-0">
      <PullHeader repo={repo} pull={pull} canEdit={canEdit} refetch={load} />
      <PullTabBar repo={repo} pull={pull} active={activeTab} />

      {activeTab === "conversation" && (
        <ConversationTab
          repo={repo}
          pull={pull}
          user={user}
          canEdit={canEdit}
          refetch={load}
          refetchAll={refetchAll}
        />
      )}
      {activeTab === "commits" && <CommitsTab repo={repo} pull={pull} />}
      {activeTab === "files" && <FilesTab repo={repo} pull={pull} user={user} refetchPull={load} />}
    </div>
  )
}

// ---------------------------------------------------------------- header

function BranchChip({ name }: { name: string }) {
  return (
    <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">{name}</span>
  )
}

function PullHeader({
  repo,
  pull,
  canEdit,
  refetch,
}: {
  repo: Repo
  pull: PullDetail
  canEdit: boolean
  refetch: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState("")
  const [body, setBody] = useState("")
  const [busy, setBusy] = useState(false)
  const ms = pull.merge_state

  const save = async () => {
    if (!title.trim()) return
    setBusy(true)
    try {
      await api.updatePull(repo.owner, repo.name, pull.number, { title: title.trim(), body })
      await refetch()
      setEditing(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to update pull request")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="pb-4">
      {editing ? (
        <div className="space-y-3">
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="text-lg font-semibold"
            placeholder="Pull request title"
          />
          <Textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={6}
            placeholder="Description (Markdown supported)"
          />
          <div className="flex gap-2">
            <Button size="sm" onClick={save} disabled={busy || !title.trim()}>
              Save
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={busy}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex items-start justify-between gap-4">
          <h1 className="text-2xl font-semibold tracking-tight">
            {pull.title} <span className="font-normal text-muted-foreground">#{pull.number}</span>
          </h1>
          <div className="flex shrink-0 items-center gap-2">
            {pull.state === "open" && repo.can_write && <PreviewDialog repo={repo} refName={pull.head} />}
            {canEdit && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setTitle(pull.title)
                  setBody(pull.body)
                  setEditing(true)
                }}
              >
                <Pencil className="size-3.5" /> Edit
              </Button>
            )}
          </div>
        </div>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-muted-foreground">
        <PRStateBadge state={pull.state} />
        <span className="flex flex-wrap items-center gap-1.5">
          <UserLink user={pull.author} className="text-foreground" />
          <span>wants to merge</span>
          <BranchChip name={pull.head} />
          <span>into</span>
          <BranchChip name={pull.base} />
        </span>
        {pull.state === "open" && ms.branches_ok && (
          <span className="text-xs">
            {ms.ahead} ahead{ms.behind > 0 && `, ${ms.behind} behind`}
          </span>
        )}
        <CIBadge status={pull.ci_status} runNumber={pull.ci_run} repo={repo.full_name} />
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- tab bar

function PullTabBar({
  repo,
  pull,
  active,
}: {
  repo: Repo
  pull: PullDetail
  active: "conversation" | "commits" | "files"
}) {
  const base = `/${repo.full_name}/pull/${pull.number}`
  const tabs = [
    { key: "conversation", to: base, label: "Conversation", icon: MessageSquare, count: pull.comments },
    { key: "commits", to: `${base}/commits`, label: "Commits", icon: GitCommitHorizontal, count: 0 },
    { key: "files", to: `${base}/files`, label: "Files changed", icon: FileDiff, count: pull.review_comments },
  ] as const
  return (
    <nav className="flex gap-1 overflow-x-auto border-b">
      {tabs.map((t) => (
        <Link
          key={t.key}
          to={t.to}
          className={cn(
            "flex items-center gap-1.5 rounded-t-lg border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap text-muted-foreground hover:bg-muted hover:text-foreground",
            active === t.key && "border-tangerine font-semibold text-foreground",
          )}
        >
          <t.icon className="size-4" />
          {t.label}
          {t.count > 0 && (
            <Badge variant="secondary" className="rounded-full px-1.5 py-0 text-xs">
              {t.count}
            </Badge>
          )}
        </Link>
      ))}
    </nav>
  )
}

// ---------------------------------------------------------------- conversation

function ConversationTab({
  repo,
  pull,
  user,
  canEdit,
  refetch,
  refetchAll,
}: {
  repo: Repo
  pull: PullDetail
  user: User | null
  canEdit: boolean
  refetch: () => Promise<void>
  refetchAll: () => Promise<void>
}) {
  return (
    <div className="mt-6 grid gap-8 lg:grid-cols-[1fr_280px]">
      <div className="min-w-0 space-y-4">
        {pull.stack.length > 0 && <StackPanel stack={pull.stack} repoFullName={repo.full_name} />}

        {pull.body.trim() !== "" && (
          <div className="overflow-hidden rounded-xl border bg-card">
            <div className="flex flex-wrap items-center gap-2 border-b bg-muted/40 px-4 py-2.5 text-sm">
              <UserAvatar user={pull.author} className="size-5" />
              <UserLink user={pull.author} />
              <span className="text-muted-foreground">commented {timeAgo(pull.created_at)}</span>
            </div>
            <div className="px-4 py-1">
              <Markdown html={pull.body_html} />
            </div>
          </div>
        )}

        {pull.timeline.map((item) => (
          <TimelineEntry key={`${item.type}-${item.id}`} item={item} />
        ))}

        {pull.state === "open" && (
          <MergeBox repo={repo} pull={pull} user={user} refetchAll={refetchAll} />
        )}
        {pull.state === "merged" && <MergedCard repo={repo} pull={pull} />}
        {pull.state === "closed" && (
          <ClosedCard repo={repo} pull={pull} canEdit={canEdit} refetchAll={refetchAll} />
        )}

        {user && <Composer repo={repo} pull={pull} refetch={refetch} />}
      </div>

      <PullAside repo={repo} pull={pull} user={user} canEdit={canEdit} refetch={refetch} refetchAll={refetchAll} />
    </div>
  )
}

// ---------------------------------------------------------------- stack panel

function StackGlyph({ depth }: { depth: number }) {
  return (
    <span
      className={cn(
        "w-4 shrink-0 text-center font-mono text-sm select-none",
        depth === 0 ? "text-tangerine" : "text-muted-foreground",
      )}
      aria-hidden
    >
      {depth === 0 ? "⏚" : "↳"}
    </span>
  )
}

function StackPanel({ stack, repoFullName }: { stack: StackItem[]; repoFullName: string }) {
  return (
    <div className="rounded-xl border border-l-4 border-l-tangerine bg-card p-4">
      <div className="flex items-center gap-2 font-semibold tracking-tight">
        <Layers className="size-4 text-tangerine" /> Stack
      </div>
      <p className="mt-0.5 text-xs text-muted-foreground">
        Merge from the bottom up — bases retarget automatically.
      </p>
      <div className="mt-3 space-y-1">
        {stack.map((item) => (
          <div
            key={item.number}
            style={{ marginLeft: item.depth * 16 }}
            className={cn(
              "flex flex-wrap items-center gap-2 rounded-lg px-2 py-1.5 text-sm",
              item.current && "bg-muted/60 ring-1 ring-tangerine/60",
            )}
          >
            <StackGlyph depth={item.depth} />
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
          </div>
        ))}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- timeline

function TimelineEntry({ item }: { item: TimelineItem }) {
  if (item.type === "comment") {
    if (item.system) {
      return (
        <div className="flex flex-wrap items-center justify-center gap-1.5 py-0.5 text-center text-xs text-muted-foreground [&_.markdown]:text-xs [&_.markdown_p]:my-0 [&_.markdown_p]:inline">
          <span className="size-1.5 shrink-0 rounded-full bg-border" aria-hidden />
          <UserLink user={item.author} className="text-foreground" />
          <Markdown html={item.body_html} />
          <span>· {timeAgo(item.created_at)}</span>
        </div>
      )
    }
    return (
      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="flex flex-wrap items-center gap-2 border-b bg-muted/40 px-4 py-2.5 text-sm">
          <UserAvatar user={item.author} className="size-5" />
          <UserLink user={item.author} />
          <span className="text-muted-foreground">commented {timeAgo(item.created_at)}</span>
        </div>
        <div className="px-4 py-1">
          <Markdown html={item.body_html} />
        </div>
      </div>
    )
  }

  const cfg = {
    approved: {
      headerCls: "bg-diff-add",
      chip: "approved these changes",
      chipCls: "text-diff-add-strong",
      icon: <Check className="size-4 text-diff-add-strong" />,
    },
    changes_requested: {
      headerCls: "bg-diff-del",
      chip: "requested changes",
      chipCls: "text-diff-del-strong",
      icon: <XCircle className="size-4 text-diff-del-strong" />,
    },
    commented: {
      headerCls: "bg-muted/40",
      chip: "reviewed",
      chipCls: "text-muted-foreground",
      icon: <MessageSquare className="size-4 text-muted-foreground" />,
    },
  }[item.state]

  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <div className={cn("flex flex-wrap items-center gap-2 border-b px-4 py-2.5 text-sm", cfg.headerCls)}>
        {cfg.icon}
        <UserAvatar user={item.author} className="size-5" />
        <UserLink user={item.author} />
        <span className={cn("rounded-full border bg-card px-2 py-0.5 text-xs font-semibold", cfg.chipCls)}>
          {cfg.chip}
        </span>
        <span className="text-xs text-muted-foreground">{timeAgo(item.created_at)}</span>
      </div>
      {item.body.trim() !== "" && (
        <div className="px-4 py-1">
          <Markdown html={item.body_html} />
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------- merge box

const STRATEGY_META = [
  { value: "merge", flag: "allow_merge", label: "Merge commit", desc: "All commits are preserved and joined to the base with a merge commit." },
  { value: "squash", flag: "allow_squash", label: "Squash and merge", desc: "Combine all commits into a single commit on the base branch." },
  { value: "rebase", flag: "allow_rebase", label: "Rebase and merge", desc: "Replay each commit onto the base branch, without a merge commit." },
] as const

function MergeBox({
  repo,
  pull,
  user,
  refetchAll,
}: {
  repo: Repo
  pull: PullDetail
  user: User | null
  refetchAll: () => Promise<void>
}) {
  const ms = pull.merge_state
  const allowed = STRATEGY_META.filter((s) => repo[s.flag])
  const [strategy, setStrategy] = useState<string>(allowed[0]?.value ?? "merge")
  const [message, setMessage] = useState("")
  const [deleteBranch, setDeleteBranch] = useState(repo.delete_branch_on_merge)
  const [busy, setBusy] = useState<"merge" | "update" | "rebase" | null>(null)
  const blocked = ms.blockers.length > 0

  const doMerge = async () => {
    setBusy("merge")
    try {
      await api.mergePull(repo.owner, repo.name, pull.number, {
        strategy,
        message: message.trim() === "" ? undefined : message.trim(),
        delete_branch: deleteBranch,
      })
      toast.success("Merged!")
      await refetchAll()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "merge failed")
    } finally {
      setBusy(null)
    }
  }

  const doBranchOp = async (kind: "update" | "rebase") => {
    setBusy(kind)
    try {
      if (kind === "update") await api.updateBranch(repo.owner, repo.name, pull.number)
      else await api.rebaseBranch(repo.owner, repo.name, pull.number)
      toast.success(kind === "update" ? "Branch updated with base" : "Restacked onto base")
      await refetchAll()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : `${kind === "update" ? "update" : "restack"} failed`)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="space-y-4 rounded-xl border border-l-4 border-l-primary bg-card p-4">
      {ms.clean ? (
        <div className="flex items-center gap-2 text-sm font-medium">
          <CheckCircle2 className="size-4 shrink-0 text-primary" />
          No conflicts with base
        </div>
      ) : (
        <div className="space-y-1.5 text-sm">
          <div className="flex items-center gap-2 font-medium">
            <XCircle className="size-4 shrink-0 text-destructive" />
            This branch cannot be merged cleanly into {pull.base}
          </div>
          {ms.conflicts && ms.conflicts.length > 0 && (
            <ul className="ml-6 space-y-0.5 font-mono text-xs text-muted-foreground">
              {ms.conflicts.map((c, i) => (
                <li key={i}>{c}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      {blocked && (
        <Alert className="border-amber-300/70 bg-amber-50 text-amber-900 dark:border-amber-800/60 dark:bg-amber-950/30 dark:text-amber-200">
          <AlertTriangle className="size-4" />
          <AlertTitle>Merging is blocked</AlertTitle>
          <AlertDescription className="text-amber-800 dark:text-amber-300/90">
            <ul className="space-y-0.5">
              {ms.blockers.map((b) => (
                <li key={b} className="flex gap-1.5">
                  <span aria-hidden>⚠</span>
                  {b}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {user && repo.can_write && (
        <>
          {allowed.length > 0 && (
            <RadioGroup value={strategy} onValueChange={setStrategy} className="gap-2.5">
              {allowed.map((s) => (
                <div key={s.value} className="flex items-start gap-2.5">
                  <RadioGroupItem value={s.value} id={`strategy-${s.value}`} className="mt-0.5" />
                  <Label htmlFor={`strategy-${s.value}`} className="flex flex-col items-start gap-0.5 font-normal">
                    <span className="text-sm font-medium">{s.label}</span>
                    <span className="text-xs text-muted-foreground">{s.desc}</span>
                  </Label>
                </div>
              ))}
            </RadioGroup>
          )}

          <Textarea
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder="Optional merge commit message"
            rows={2}
            className="text-sm"
          />

          <div className="flex items-center gap-2">
            <Checkbox
              id="delete-branch"
              checked={deleteBranch}
              onCheckedChange={(v) => setDeleteBranch(v === true)}
            />
            <Label htmlFor="delete-branch" className="text-sm font-normal">
              Delete head branch after merge
            </Label>
          </div>

          <div className="flex items-center gap-3">
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span tabIndex={blocked ? 0 : -1}>
                    <Button disabled={blocked || busy !== null} onClick={doMerge}>
                      <GitMerge className="size-4" />
                      {busy === "merge" ? "Merging…" : "Merge pull request"}
                    </Button>
                  </span>
                </TooltipTrigger>
                {blocked && (
                  <TooltipContent className="max-w-xs">
                    <ul className="list-disc space-y-0.5 pl-4 text-left">
                      {ms.blockers.map((b) => (
                        <li key={b}>{b}</li>
                      ))}
                    </ul>
                  </TooltipContent>
                )}
              </Tooltip>
            </TooltipProvider>
          </div>

          {ms.behind > 0 && (
            <div className="flex flex-wrap items-center gap-3 border-t pt-3.5 text-sm text-muted-foreground">
              <span>
                This branch is {ms.behind} commit{ms.behind === 1 ? "" : "s"} behind{" "}
                <span className="font-mono text-xs">{pull.base}</span>.
              </span>
              <div className="ml-auto flex gap-2">
                <Button variant="outline" size="sm" disabled={busy !== null} onClick={() => doBranchOp("update")}>
                  <RefreshCw className="size-3.5" />
                  {busy === "update" ? "Updating…" : "Update branch"}
                </Button>
                <Button variant="outline" size="sm" disabled={busy !== null} onClick={() => doBranchOp("rebase")}>
                  <ArrowUpFromDot className="size-3.5" />
                  {busy === "rebase" ? "Restacking…" : "Restack onto base"}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function MergedCard({ repo, pull }: { repo: Repo; pull: PullDetail }) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-xl border border-violet-600/30 bg-violet-600/5 px-4 py-3.5 text-sm">
      <GitMerge className="size-4 shrink-0 text-violet-600" />
      <span className="flex flex-wrap items-center gap-1.5">
        {pull.merged_by && <UserLink user={pull.merged_by} />}
        <span>merged into</span>
        <BranchChip name={pull.base} />
        <span>as</span>
        <Link
          to={`/${repo.full_name}/commit/${pull.merge_commit}`}
          className="font-mono text-xs text-primary hover:underline"
        >
          {shortSha(pull.merge_commit)}
        </Link>
        <span className="text-muted-foreground">{timeAgo(pull.merged_at ?? pull.updated_at)}</span>
      </span>
    </div>
  )
}

function ClosedCard({
  repo,
  pull,
  canEdit,
  refetchAll,
}: {
  repo: Repo
  pull: PullDetail
  canEdit: boolean
  refetchAll: () => Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-muted/40 px-4 py-3.5 text-sm text-muted-foreground">
      <XCircle className="size-4 shrink-0" />
      <span>This pull request was closed without merging.</span>
      {canEdit && (
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          disabled={busy}
          onClick={async () => {
            setBusy(true)
            try {
              await api.reopenPull(repo.owner, repo.name, pull.number)
              await refetchAll()
            } catch (e) {
              toast.error(e instanceof Error ? e.message : "failed to reopen")
            } finally {
              setBusy(false)
            }
          }}
        >
          Reopen pull request
        </Button>
      )}
    </div>
  )
}

// ---------------------------------------------------------------- composer

function Composer({
  repo,
  pull,
  refetch,
}: {
  repo: Repo
  pull: PullDetail
  refetch: () => Promise<void>
}) {
  const [comment, setComment] = useState("")
  const [verdict, setVerdict] = useState("approved")
  const [reviewBody, setReviewBody] = useState("")
  const [busy, setBusy] = useState(false)

  const submitComment = async () => {
    if (!comment.trim()) return
    setBusy(true)
    try {
      await api.pullComment(repo.owner, repo.name, pull.number, comment.trim())
      setComment("")
      await refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to comment")
    } finally {
      setBusy(false)
    }
  }

  const submitReview = async () => {
    setBusy(true)
    try {
      await api.pullReview(repo.owner, repo.name, pull.number, verdict, reviewBody.trim())
      setReviewBody("")
      toast.success("Review submitted")
      await refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to submit review")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-xl border bg-card p-4">
      <Tabs defaultValue="comment">
        <TabsList>
          <TabsTrigger value="comment">Comment</TabsTrigger>
          <TabsTrigger value="review">Review</TabsTrigger>
        </TabsList>

        <TabsContent value="comment" className="mt-3 space-y-3">
          <Textarea
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Leave a comment (Markdown supported)"
            rows={3}
          />
          <div className="flex justify-end">
            <Button onClick={submitComment} disabled={busy || !comment.trim()}>
              Comment
            </Button>
          </div>
        </TabsContent>

        <TabsContent value="review" className="mt-3 space-y-3">
          <RadioGroup value={verdict} onValueChange={setVerdict} className="gap-2.5">
            {[
              { value: "approved", label: "Approve", desc: "Approve merging these changes." },
              { value: "changes_requested", label: "Request changes", desc: "Changes must be addressed before merging." },
              { value: "commented", label: "Comment", desc: "General feedback without an explicit verdict." },
            ].map((v) => (
              <div key={v.value} className="flex items-start gap-2.5">
                <RadioGroupItem value={v.value} id={`verdict-${v.value}`} className="mt-0.5" />
                <Label htmlFor={`verdict-${v.value}`} className="flex flex-col items-start gap-0.5 font-normal">
                  <span className="text-sm font-medium">{v.label}</span>
                  <span className="text-xs text-muted-foreground">{v.desc}</span>
                </Label>
              </div>
            ))}
          </RadioGroup>
          <Textarea
            value={reviewBody}
            onChange={(e) => setReviewBody(e.target.value)}
            placeholder={verdict === "commented" ? "Review feedback (required)" : "Review feedback (optional)"}
            rows={3}
          />
          <div className="flex justify-end">
            <Button onClick={submitReview} disabled={busy || (verdict === "commented" && !reviewBody.trim())}>
              Submit review
            </Button>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ---------------------------------------------------------------- aside

function AsideTitle({ children }: { children: React.ReactNode }) {
  return <h4 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">{children}</h4>
}

function PullAside({
  repo,
  pull,
  user,
  canEdit,
  refetch,
  refetchAll,
}: {
  repo: Repo
  pull: PullDetail
  user: User | null
  canEdit: boolean
  refetch: () => Promise<void>
  refetchAll: () => Promise<void>
}) {
  const ms = pull.merge_state
  const open = pull.state === "open"
  return (
    <aside className="space-y-5 self-start lg:sticky lg:top-6">
      <section className="space-y-2.5">
        <AsideTitle>Reviewers</AsideTitle>
        {pull.verdicts.length === 0 ? (
          <p className="text-sm text-muted-foreground">No reviews yet</p>
        ) : (
          <ul className="space-y-2">
            {pull.verdicts.map((v) => (
              <li key={v.user.id} className="flex items-center gap-2 text-sm">
                <UserAvatar user={v.user} className="size-5" />
                <UserLink user={v.user} />
                {v.state === "approved" ? (
                  <span className="ml-auto flex items-center gap-1 text-xs font-medium text-primary">
                    <Check className="size-3.5" /> approved
                  </span>
                ) : (
                  <span className="ml-auto flex items-center gap-1 text-xs font-medium text-amber-600 dark:text-amber-400">
                    <span className="font-mono" aria-hidden>±</span> changes requested
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
        {repo.require_approvals > 0 && (
          <p
            className={cn(
              "text-xs",
              ms.approvals >= repo.require_approvals ? "text-primary" : "text-muted-foreground",
            )}
          >
            {ms.approvals}/{repo.require_approvals} required approval{repo.require_approvals === 1 ? "" : "s"}
          </p>
        )}
      </section>

      <Separator />

      <section className="space-y-2.5">
        <AsideTitle>CI</AsideTitle>
        {pull.ci_status ? (
          <div className="flex items-center gap-2 text-sm">
            <CIBadge status={pull.ci_status} />
            {pull.ci_run ? (
              <Link to={`/${repo.full_name}/ci/${pull.ci_run}`} className="hover:text-primary hover:underline">
                Run #{pull.ci_run} — <span className="capitalize">{pull.ci_status}</span>
              </Link>
            ) : (
              <span className="capitalize">{pull.ci_status}</span>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No CI runs for this branch</p>
        )}
      </section>

      {open && canEdit && (
        <>
          <Separator />
          <ChangeBase key={pull.base} repo={repo} pull={pull} refetch={refetch} />
          <Separator />
          <section className="space-y-2.5">
            <AsideTitle>Danger</AsideTitle>
            <CloseButton repo={repo} pull={pull} refetchAll={refetchAll} />
          </section>
        </>
      )}
      {!user && (
        <>
          <Separator />
          <p className="text-xs text-muted-foreground">
            <Link to="/login" className="text-primary hover:underline">
              Sign in
            </Link>{" "}
            to review or comment.
          </p>
        </>
      )}
    </aside>
  )
}

function ChangeBase({
  repo,
  pull,
  refetch,
}: {
  repo: Repo
  pull: PullDetail
  refetch: () => Promise<void>
}) {
  const [base, setBase] = useState(pull.base)
  const [busy, setBusy] = useState(false)
  const candidates = (repo.branches ?? []).filter((b) => b.name !== pull.head)

  return (
    <section className="space-y-2.5">
      <AsideTitle>Change base</AsideTitle>
      <div className="flex items-center gap-2">
        <Select value={base} onValueChange={setBase}>
          <SelectTrigger size="sm" className="min-w-0 flex-1 font-mono text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {candidates.map((b) => (
              <SelectItem key={b.name} value={b.name} className="font-mono text-xs">
                {b.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          disabled={busy || base === pull.base}
          onClick={async () => {
            setBusy(true)
            try {
              await api.retargetPull(repo.owner, repo.name, pull.number, base)
              toast.success(`Base changed to ${base}`)
              await refetch()
            } catch (e) {
              toast.error(e instanceof Error ? e.message : "failed to retarget")
            } finally {
              setBusy(false)
            }
          }}
        >
          Retarget
        </Button>
      </div>
    </section>
  )
}

function CloseButton({
  repo,
  pull,
  refetchAll,
}: {
  repo: Repo
  pull: PullDetail
  refetchAll: () => Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  return (
    <Button
      variant="outline"
      size="sm"
      className="w-full border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
      disabled={busy}
      onClick={async () => {
        setBusy(true)
        try {
          await api.closePull(repo.owner, repo.name, pull.number)
          await refetchAll()
        } catch (e) {
          toast.error(e instanceof Error ? e.message : "failed to close")
        } finally {
          setBusy(false)
        }
      }}
    >
      <XCircle className="size-3.5" /> Close pull request
    </Button>
  )
}

// ---------------------------------------------------------------- commits tab

function CommitsTab({ repo, pull }: { repo: Repo; pull: PullDetail }) {
  const [commits, setCommits] = useState<Commit[] | null>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    let live = true
    api
      .pullCommits(repo.owner, repo.name, pull.number)
      .then((c) => {
        if (live) setCommits(c)
      })
      .catch((e: unknown) => {
        if (live) setError(e instanceof Error ? e.message : "failed to load commits")
      })
    return () => {
      live = false
    }
  }, [repo.owner, repo.name, pull.number])

  if (error) return <div className="mt-6"><ErrorNote message={error} /></div>
  if (!commits) return <PageLoading />
  if (commits.length === 0) {
    return (
      <div className="mt-6">
        <EmptyState icon={<GitCommitHorizontal />} title="No commits">
          This pull request has no commits — the head branch may have been deleted.
        </EmptyState>
      </div>
    )
  }

  return (
    <div className="mt-6 divide-y rounded-xl border bg-card">
      {commits.map((c) => (
        <div key={c.sha} className="flex items-center gap-3 px-4 py-3">
          <CIBadge status={c.ci_status} runNumber={c.ci_run} repo={repo.full_name} className="shrink-0" />
          <div className="min-w-0 flex-1">
            <Link
              to={`/${repo.full_name}/commit/${c.sha}`}
              className="block truncate font-medium hover:text-primary hover:underline"
            >
              {c.subject}
            </Link>
            <div className="mt-0.5 text-xs text-muted-foreground">
              {c.author_name} committed {timeAgo(c.when)}
            </div>
          </div>
          <Link
            to={`/${repo.full_name}/commit/${c.sha}`}
            className="shrink-0 font-mono text-xs text-muted-foreground hover:text-primary hover:underline"
          >
            {c.short_sha}
          </Link>
        </div>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------- files tab

function FilesTab({
  repo,
  pull,
  user,
  refetchPull,
}: {
  repo: Repo
  pull: PullDetail
  user: User | null
  refetchPull: () => Promise<void>
}) {
  const [files, setFiles] = useState<PullFilesResponse | null>(null)
  const [error, setError] = useState("")

  const load = useCallback(async () => {
    try {
      setFiles(await api.pullFiles(repo.owner, repo.name, pull.number))
      setError("")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load changed files")
    }
  }, [repo.owner, repo.name, pull.number])

  useEffect(() => {
    load()
  }, [load])

  if (error) return <div className="mt-6"><ErrorNote message={error} /></div>
  if (!files) return <PageLoading />

  return (
    <div className="mt-6 space-y-4">
      {files.from_merge_commit && (
        <div className="flex items-center gap-2 rounded-lg border bg-muted/40 px-4 py-2.5 text-sm text-muted-foreground">
          <Info className="size-4 shrink-0" />
          Head branch was deleted — showing the merge commit's changes.
        </div>
      )}
      <DiffStatLine diff={files} />
      <DiffView
        diff={files}
        review={{
          comments: files.comments,
          canComment: !!user,
          onComment: async (file, line, side, body) => {
            await api.pullReviewComment(repo.owner, repo.name, pull.number, file, line, side, body)
            await Promise.all([load(), refetchPull()])
          },
        }}
      />
    </div>
  )
}
