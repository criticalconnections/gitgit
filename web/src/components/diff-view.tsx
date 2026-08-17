import { useState } from "react"
import { ChevronDown, ChevronRight, FileCode2, MessageSquarePlus } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Markdown, UserAvatar, UserLink } from "@/components/shared"
import { timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { Diff, DiffFile, DiffLine, ReviewComment } from "@/lib/api"

export function DiffStatLine({ diff, className }: { diff: Diff; className?: string }) {
  return (
    <div className={cn("flex items-center gap-2 text-sm text-muted-foreground", className)}>
      <span>
        {diff.stat.files} file{diff.stat.files === 1 ? "" : "s"} changed
      </span>
      <span className="font-semibold text-diff-add-strong">+{diff.stat.additions}</span>
      <span className="font-semibold text-diff-del-strong">−{diff.stat.deletions}</span>
    </div>
  )
}

const statusBadge: Record<DiffFile["status"], { label: string; cls: string } | null> = {
  modified: null,
  added: { label: "added", cls: "bg-diff-add text-diff-add-strong" },
  deleted: { label: "deleted", cls: "bg-diff-del text-diff-del-strong" },
  renamed: { label: "renamed", cls: "bg-muted text-muted-foreground" },
}

interface LineCommentSupport {
  comments: ReviewComment[]
  canComment: boolean
  onComment: (file: string, line: number, side: "old" | "new", body: string) => Promise<void>
}

export function DiffView({ diff, review }: { diff: Diff; review?: LineCommentSupport }) {
  if (diff.files.length === 0) {
    return <div className="rounded-xl border py-12 text-center text-sm text-muted-foreground">No changes to show.</div>
  }
  return (
    <div className="space-y-4">
      {diff.files.map((f) => (
        <DiffFileCard key={f.path} file={f} review={review} />
      ))}
    </div>
  )
}

function DiffFileCard({ file, review }: { file: DiffFile; review?: LineCommentSupport }) {
  const [open, setOpen] = useState(true)
  const badge = statusBadge[file.status]
  return (
    <div id={anchorFor(file.path)} className="overflow-hidden rounded-xl border bg-card">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 border-b bg-muted/50 px-4 py-2.5 text-left"
      >
        {open ? (
          <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        )}
        <FileCode2 className="size-4 shrink-0 text-muted-foreground" />
        <span className="truncate font-mono text-[13px] font-semibold">{file.path}</span>
        {badge && <Badge className={cn("rounded-full", badge.cls)}>{badge.label}</Badge>}
        <span className="ml-auto flex shrink-0 items-center gap-2 font-mono text-xs">
          <span className="font-semibold text-diff-add-strong">+{file.additions}</span>
          <span className="font-semibold text-diff-del-strong">−{file.deletions}</span>
        </span>
      </button>
      {open &&
        (file.binary ? (
          <div className="px-4 py-6 text-sm text-muted-foreground italic">Binary file not shown</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse font-mono text-[12.5px] leading-relaxed">
              <tbody>
                {file.hunks.map((hunk, hi) => (
                  <HunkRows key={hi} file={file} header={hunk.header} lines={hunk.lines} review={review} />
                ))}
              </tbody>
            </table>
            {file.truncated && (
              <div className="border-t px-4 py-3 text-sm text-muted-foreground italic">Large diff truncated</div>
            )}
          </div>
        ))}
    </div>
  )
}

function anchorFor(path: string) {
  return "d-" + path.replace(/[^a-zA-Z0-9]/g, "-")
}

function HunkRows({
  file,
  header,
  lines,
  review,
}: {
  file: DiffFile
  header: string
  lines: DiffLine[]
  review?: LineCommentSupport
}) {
  return (
    <>
      <tr>
        <td colSpan={2} className="w-[1%] min-w-20 bg-diff-hunk" />
        <td className="bg-diff-hunk px-3 py-1 text-muted-foreground select-none">{header}</td>
      </tr>
      {lines.map((l, i) => (
        <LineRow key={i} file={file} line={l} review={review} />
      ))}
    </>
  )
}

function LineRow({ file, line, review }: { file: DiffFile; line: DiffLine; review?: LineCommentSupport }) {
  const [composerOpen, setComposerOpen] = useState(false)
  const side: "old" | "new" = line.op === "-" ? "old" : "new"
  const num = side === "old" ? line.old : line.new
  const threads =
    review?.comments.filter((c) => c.file === file.path && c.line === num && c.side === side && num > 0) ?? []

  const rowCls =
    line.op === "+" ? "bg-diff-add" : line.op === "-" ? "bg-diff-del" : ""
  const numCls =
    line.op === "+"
      ? "bg-diff-add text-diff-add-strong"
      : line.op === "-"
        ? "bg-diff-del text-diff-del-strong"
        : "text-muted-foreground"

  return (
    <>
      <tr className={cn("group", rowCls)}>
        <td className={cn("w-10 min-w-10 border-r px-2 text-right align-top select-none", numCls)}>
          {line.old || ""}
        </td>
        <td className={cn("w-10 min-w-10 border-r px-2 text-right align-top select-none", numCls)}>
          {line.new || ""}
        </td>
        <td className="px-3 align-top whitespace-pre-wrap">
          <span className="mr-1 inline-block w-3 text-muted-foreground select-none">
            {line.op === " " ? "" : line.op}
          </span>
          <span className="break-all">{line.text}</span>
          {review?.canComment && num > 0 && (
            <button
              type="button"
              onClick={() => setComposerOpen(!composerOpen)}
              className="ml-2 inline-flex align-middle text-primary opacity-0 transition-opacity group-hover:opacity-100"
              title={`Comment on line ${num}`}
            >
              <MessageSquarePlus className="size-4" />
            </button>
          )}
        </td>
      </tr>
      {(threads.length > 0 || composerOpen) && (
        <tr>
          <td colSpan={3} className="border-y bg-amber-50/60 px-4 py-2 dark:bg-amber-950/20">
            <div className="max-w-2xl space-y-2 py-1">
              {threads.map((c) => (
                <div key={c.id} className="rounded-lg border bg-card px-3 py-2">
                  <div className="flex items-center gap-2 font-sans text-xs text-muted-foreground">
                    <UserAvatar user={c.author} className="size-5" />
                    <UserLink user={c.author} className="text-foreground" />
                    commented {timeAgo(c.created_at)}
                  </div>
                  <Markdown html={c.body_html} className="font-sans text-sm" />
                </div>
              ))}
              {review?.canComment && (composerOpen || threads.length > 0) && (
                <LineComposer
                  open={composerOpen}
                  onOpen={() => setComposerOpen(true)}
                  hasThread={threads.length > 0}
                  onSubmit={async (body) => {
                    await review.onComment(file.path, num, side, body)
                    setComposerOpen(false)
                  }}
                />
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

function LineComposer({
  open,
  hasThread,
  onOpen,
  onSubmit,
}: {
  open: boolean
  hasThread: boolean
  onOpen: () => void
  onSubmit: (body: string) => Promise<void>
}) {
  const [body, setBody] = useState("")
  const [busy, setBusy] = useState(false)
  if (!open && hasThread) {
    return (
      <Button variant="ghost" size="sm" className="font-sans" onClick={onOpen}>
        Reply
      </Button>
    )
  }
  return (
    <form
      className="space-y-2 font-sans"
      onSubmit={async (e) => {
        e.preventDefault()
        if (!body.trim()) return
        setBusy(true)
        try {
          await onSubmit(body)
          setBody("")
        } finally {
          setBusy(false)
        }
      }}
    >
      <Textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Leave a comment on this line"
        rows={2}
        className="bg-card text-sm"
      />
      <Button type="submit" size="sm" disabled={busy || !body.trim()}>
        Add comment
      </Button>
    </form>
  )
}
