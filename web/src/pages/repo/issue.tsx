import { useCallback, useEffect, useState } from "react"
import { useNavigate, useParams } from "react-router-dom"
import { CheckCircle2, CircleDot, Pencil, Tag } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { api, type Comment, type IssueDetail, type UserRef } from "@/lib/api"
import { formatDateTime, timeAgo } from "@/lib/format"
import { ErrorNote, IssueStateBadge, LabelPill, Markdown, PageLoading, UserAvatar, UserLink } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"
import { useAuth } from "@/lib/auth"

// GET /issues/{n} returns a redirect stub when the number belongs to a pull.
type IssueFetch = IssueDetail | { is_pull: true; number: number }

function CommentCard({
  author,
  createdAt,
  html,
  verb,
}: {
  author: UserRef
  createdAt: number
  html: string
  verb: string
}) {
  return (
    <Card className="gap-0 overflow-hidden py-0">
      <div className="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-4 py-2.5">
        <UserAvatar user={author} className="size-5" />
        <UserLink user={author} className="text-sm" />
        <span className="text-xs text-muted-foreground">
          {verb} {timeAgo(createdAt)}
        </span>
      </div>
      <div className="px-4 py-4">
        {html.trim() ? (
          <Markdown html={html} className="text-sm" />
        ) : (
          <p className="text-sm text-muted-foreground italic">No description provided.</p>
        )}
      </div>
    </Card>
  )
}

function SystemRow({ comment }: { comment: Comment }) {
  return (
    <div className="flex items-center gap-2 px-2 text-xs text-muted-foreground">
      <span className="size-1.5 shrink-0 rounded-full bg-border" />
      <span className="font-medium text-foreground">{comment.author.username}</span>
      <span className="truncate">{comment.body}</span>
      <span className="shrink-0">{timeAgo(comment.created_at)}</span>
    </div>
  )
}

export default function IssuePage() {
  const { repo } = useRepo()
  const { user } = useAuth()
  const { number = "" } = useParams()
  const navigate = useNavigate()

  const [issue, setIssue] = useState<IssueDetail | null>(null)
  const [error, setError] = useState("")

  const [editing, setEditing] = useState(false)
  const [editTitle, setEditTitle] = useState("")
  const [editBody, setEditBody] = useState("")
  const [savingEdit, setSavingEdit] = useState(false)

  const [comment, setComment] = useState("")
  const [commenting, setCommenting] = useState(false)
  const [stateBusy, setStateBusy] = useState(false)

  const [labelSel, setLabelSel] = useState<number[]>([])
  const [savingLabels, setSavingLabels] = useState(false)

  const load = useCallback(async () => {
    try {
      const data: IssueFetch = await api.issue(repo.owner, repo.name, number)
      if ("is_pull" in data) {
        navigate(`/${repo.full_name}/pull/${data.number}`, { replace: true })
        return
      }
      setIssue(data)
      setLabelSel(data.labels.map((l) => l.id))
      setError("")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load issue")
    }
  }, [repo.owner, repo.name, repo.full_name, number, navigate])

  useEffect(() => {
    setIssue(null)
    setEditing(false)
    load()
  }, [load])

  if (error) return <ErrorNote message={error} />
  if (!issue) return <PageLoading />

  const isAuthor = user !== null && user.id === issue.author.id
  const canModify = repo.can_write || isAuthor

  const startEdit = () => {
    setEditTitle(issue.title)
    setEditBody(issue.body)
    setEditing(true)
  }

  const saveEdit = async () => {
    if (!editTitle.trim() || savingEdit) return
    setSavingEdit(true)
    try {
      await api.updateIssue(repo.owner, repo.name, issue.number, { title: editTitle.trim(), body: editBody })
      setEditing(false)
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to update issue")
    } finally {
      setSavingEdit(false)
    }
  }

  const submitComment = async () => {
    if (!comment.trim() || commenting) return
    setCommenting(true)
    try {
      await api.issueComment(repo.owner, repo.name, issue.number, comment)
      setComment("")
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to post comment")
    } finally {
      setCommenting(false)
    }
  }

  const toggleState = async () => {
    if (stateBusy) return
    setStateBusy(true)
    try {
      if (issue.state === "open") await api.closeIssue(repo.owner, repo.name, issue.number)
      else await api.reopenIssue(repo.owner, repo.name, issue.number)
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to update issue")
    } finally {
      setStateBusy(false)
    }
  }

  const labelsDirty =
    [...labelSel].sort((a, b) => a - b).join(",") !==
    issue.labels
      .map((l) => l.id)
      .sort((a, b) => a - b)
      .join(",")

  const saveLabels = async () => {
    if (savingLabels) return
    setSavingLabels(true)
    try {
      await api.setIssueLabels(repo.owner, repo.name, issue.number, labelSel)
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to update labels")
    } finally {
      setSavingLabels(false)
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      {/* header */}
      <div>
        {editing ? (
          <div className="flex flex-wrap items-center gap-2">
            <Input
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
              className="max-w-2xl flex-1 text-lg font-semibold"
              autoFocus
            />
            <Button size="sm" onClick={saveEdit} disabled={!editTitle.trim() || savingEdit}>
              {savingEdit ? "Saving…" : "Save"}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={savingEdit}>
              Cancel
            </Button>
          </div>
        ) : (
          <div className="flex flex-wrap items-start gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {issue.title} <span className="font-normal text-muted-foreground">#{issue.number}</span>
            </h1>
            {canModify && (
              <Button variant="ghost" size="icon" className="mt-0.5 size-8 text-muted-foreground" onClick={startEdit}>
                <Pencil className="size-4" />
                <span className="sr-only">Edit issue</span>
              </Button>
            )}
          </div>
        )}
        <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <IssueStateBadge state={issue.state} />
          <span>
            <UserLink user={issue.author} /> opened this issue {timeAgo(issue.created_at)}
          </span>
          {issue.labels.map((l) => (
            <LabelPill key={l.id} label={l} />
          ))}
        </div>
      </div>

      <Separator />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
        {/* main column */}
        <div className="min-w-0 space-y-4">
          {editing ? (
            <Card className="gap-3 p-4">
              <Textarea
                value={editBody}
                onChange={(e) => setEditBody(e.target.value)}
                rows={10}
                className="min-h-48 font-mono text-sm"
                placeholder="Describe the issue… Markdown is supported."
              />
              <div className="flex justify-end gap-2">
                <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={savingEdit}>
                  Cancel
                </Button>
                <Button size="sm" onClick={saveEdit} disabled={!editTitle.trim() || savingEdit}>
                  {savingEdit ? "Saving…" : "Save changes"}
                </Button>
              </div>
            </Card>
          ) : (
            <CommentCard author={issue.author} createdAt={issue.created_at} html={issue.body_html} verb="opened" />
          )}

          {issue.comment_list.map((c) =>
            c.system ? (
              <SystemRow key={c.id} comment={c} />
            ) : (
              <CommentCard key={c.id} author={c.author} createdAt={c.created_at} html={c.body_html} verb="commented" />
            ),
          )}

          {/* composer */}
          {user ? (
            <Card className="gap-3 p-4">
              <div className="flex items-center gap-2">
                <UserAvatar user={user} className="size-5" />
                <span className="text-sm font-medium">Add a comment</span>
              </div>
              <Textarea
                value={comment}
                onChange={(e) => setComment(e.target.value)}
                rows={4}
                placeholder="Leave a comment… Markdown is supported."
              />
              <div className="flex flex-wrap items-center justify-end gap-2">
                {canModify &&
                  (issue.state === "open" ? (
                    <Button variant="outline" size="sm" onClick={toggleState} disabled={stateBusy}>
                      <CheckCircle2 className="size-4 text-violet-600" />
                      Close issue
                    </Button>
                  ) : (
                    <Button variant="outline" size="sm" onClick={toggleState} disabled={stateBusy}>
                      <CircleDot className="size-4 text-primary" />
                      Reopen issue
                    </Button>
                  ))}
                <Button size="sm" onClick={submitComment} disabled={!comment.trim() || commenting}>
                  {commenting ? "Posting…" : "Comment"}
                </Button>
              </div>
            </Card>
          ) : (
            <p className="rounded-xl border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
              Sign in to join the conversation.
            </p>
          )}
        </div>

        {/* aside */}
        <aside className="space-y-6">
          <div>
            <h3 className="flex items-center gap-1.5 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              <Tag className="size-3.5" /> Labels
            </h3>
            {repo.can_write ? (
              <div className="mt-3 space-y-2.5">
                {issue.all_labels.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No labels defined for this repository.</p>
                ) : (
                  issue.all_labels.map((l) => (
                    <label key={l.id} className="flex cursor-pointer items-center gap-2">
                      <Checkbox
                        checked={labelSel.includes(l.id)}
                        onCheckedChange={(checked) =>
                          setLabelSel((sel) =>
                            checked === true ? [...sel, l.id] : sel.filter((id) => id !== l.id),
                          )
                        }
                      />
                      <LabelPill label={l} />
                    </label>
                  ))
                )}
                {issue.all_labels.length > 0 && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-1"
                    onClick={saveLabels}
                    disabled={!labelsDirty || savingLabels}
                  >
                    {savingLabels ? "Updating…" : "Update labels"}
                  </Button>
                )}
              </div>
            ) : issue.labels.length > 0 ? (
              <div className="mt-3 flex flex-wrap gap-1.5">
                {issue.labels.map((l) => (
                  <LabelPill key={l.id} label={l} />
                ))}
              </div>
            ) : (
              <p className="mt-3 text-sm text-muted-foreground">None yet</p>
            )}
          </div>

          <Separator />

          <div className="space-y-1.5 text-sm text-muted-foreground">
            <p>
              Opened <span className="text-foreground">{formatDateTime(issue.created_at)}</span>
            </p>
            <p>
              {issue.comments} comment{issue.comments === 1 ? "" : "s"}
            </p>
          </div>
        </aside>
      </div>
    </div>
  )
}
