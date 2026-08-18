import { useEffect, useState } from "react"
import { Link, useLocation, useNavigate, useParams } from "react-router-dom"
import {
  BookOpen,
  Check,
  ChevronDown,
  Code2,
  Copy,
  Download,
  FileCode2,
  FileText,
  Folder,
  GitBranch,
  History,
  Terminal,
} from "lucide-react"
import { toast } from "sonner"
import { PreviewDialog } from "@/components/preview-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { api, type BlobResponse, type TreeResponse } from "@/lib/api"
import { splitRefPath } from "@/lib/refpath"
import { formatBytes, shortSha, timeAgo } from "@/lib/format"
import { CIBadge, EmptyState, ErrorNote, Markdown, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"
import { cn } from "@/lib/utils"

const encodePath = (p: string) => p.split("/").map(encodeURIComponent).join("/")

function copyText(text: string) {
  navigator.clipboard
    .writeText(text)
    .then(() => toast.success("Copied to clipboard"))
    .catch(() => toast.error("copy failed"))
}

function CopyButton({ text, className }: { text: string; className?: string }) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={cn("size-7 shrink-0 text-muted-foreground", className)}
      onClick={() => copyText(text)}
      title="Copy"
    >
      <Copy className="size-3.5" />
    </Button>
  )
}

// ---------- branch picker ----------

function BranchPicker({ current, toBranch }: { current: string; toBranch: (name: string) => string }) {
  const { repo } = useRepo()
  const navigate = useNavigate()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" className="max-w-56">
          <GitBranch className="size-4 text-muted-foreground" />
          <span className="truncate font-mono text-xs">{current}</span>
          <ChevronDown className="size-3.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-80 w-64 overflow-y-auto">
        <DropdownMenuLabel>Switch branches</DropdownMenuLabel>
        {(repo.branches ?? []).map((b) => (
          <DropdownMenuItem key={b.name} onSelect={() => navigate(toBranch(b.name))}>
            <span className="w-4 shrink-0">{b.name === current && <Check className="size-4 text-primary" />}</span>
            <span className="truncate font-mono text-xs">{b.name}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ---------- breadcrumbs ----------

function PathBreadcrumb({ refName, path, leaf }: { refName: string; path: string; leaf?: boolean }) {
  const { repo } = useRepo()
  const base = `/${repo.full_name}`
  const segs = path === "" ? [] : path.split("/")
  return (
    <Breadcrumb>
      <BreadcrumbList className="flex-nowrap">
        <BreadcrumbItem>
          {segs.length === 0 ? (
            <BreadcrumbPage className="font-semibold">{repo.name}</BreadcrumbPage>
          ) : (
            <BreadcrumbLink asChild>
              <Link to={`${base}/tree/${encodePath(refName)}`} className="font-semibold text-foreground">
                {repo.name}
              </Link>
            </BreadcrumbLink>
          )}
        </BreadcrumbItem>
        {segs.map((seg, i) => {
          const last = i === segs.length - 1
          const to = `${base}/${last && leaf ? "blob" : "tree"}/${encodePath(refName)}/${encodePath(segs.slice(0, i + 1).join("/"))}`
          return (
            <BreadcrumbItem key={i}>
              <BreadcrumbSeparator />
              {last ? (
                <BreadcrumbPage className="font-semibold">{seg}</BreadcrumbPage>
              ) : (
                <BreadcrumbLink asChild>
                  <Link to={to}>{seg}</Link>
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          )
        })}
      </BreadcrumbList>
    </Breadcrumb>
  )
}

// ---------- empty repo: quick setup ----------

function CommandBlock({ title, lines }: { title: string; lines: string[] }) {
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-semibold">{title}</h3>
      <div className="flex items-start gap-2 rounded-lg border bg-muted/50 p-3">
        <pre className="flex-1 overflow-x-auto font-mono text-[12.5px] leading-relaxed">{lines.join("\n")}</pre>
        <CopyButton text={lines.join("\n")} />
      </div>
    </div>
  )
}

function QuickSetup() {
  const { repo } = useRepo()
  const clone = repo.clone_url ?? ""
  const branch = repo.default_branch || "main"
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="rounded-xl border bg-card p-6">
        <div className="flex items-center gap-2">
          <Terminal className="size-5 text-primary" />
          <h2 className="text-lg font-semibold tracking-tight">Quick setup</h2>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          This repository is empty. Get started by cloning it, or push some code from your terminal.
        </p>
        <div className="mt-4 flex items-center gap-2">
          <Input
            readOnly
            value={clone}
            onClick={(e) => e.currentTarget.select()}
            className="font-mono text-xs"
          />
          <CopyButton text={clone} className="size-9" />
        </div>
        <div className="mt-6 space-y-6">
          <CommandBlock
            title="Create a new repository on the command line"
            lines={[
              `echo "# ${repo.name}" >> README.md`,
              "git init",
              "git add README.md",
              'git commit -m "first commit"',
              `git branch -M ${branch}`,
              `git remote add origin ${clone}`,
              `git push -u origin ${branch}`,
            ]}
          />
          <CommandBlock
            title="Push an existing repository from the command line"
            lines={[`git remote add origin ${clone}`, `git push -u origin ${branch}`]}
          />
        </div>
      </div>
    </div>
  )
}

// ---------- tree mode ----------

function TreeView({ tree, refName, path }: { tree: TreeResponse; refName: string; path: string }) {
  const { repo } = useRepo()
  const base = `/${repo.full_name}`
  const parent = path.split("/").slice(0, -1).join("/")
  const latest = tree.latest
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <BranchPicker current={refName} toBranch={(b) => `${base}/tree/${encodePath(b)}`} />
        <PathBreadcrumb refName={refName} path={path} />
        <div className="ml-auto flex items-center gap-2">
          {tree.commit_count !== undefined && (
            <Link
              to={`${base}/commits/${encodePath(refName)}`}
              className="inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <History className="size-3.5" />
              {tree.commit_count} commit{tree.commit_count === 1 ? "" : "s"}
            </Link>
          )}
          {repo.can_write && <PreviewDialog repo={repo} refName={refName} />}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm">
                <Code2 className="size-4" />
                Code
                <ChevronDown className="size-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-80 p-3">
              <p className="mb-2 text-xs font-semibold text-muted-foreground">Clone with HTTP</p>
              <div className="flex items-center gap-2">
                <Input
                  readOnly
                  value={repo.clone_url ?? ""}
                  onClick={(e) => e.currentTarget.select()}
                  onKeyDown={(e) => e.stopPropagation()}
                  className="h-8 font-mono text-xs"
                />
                <CopyButton text={repo.clone_url ?? ""} />
              </div>
              {repo.ssh_clone_url && (
                <>
                  <p className="mt-3 mb-2 text-xs font-semibold text-muted-foreground">Clone with SSH</p>
                  <div className="flex items-center gap-2">
                    <Input
                      readOnly
                      value={repo.ssh_clone_url}
                      onClick={(e) => e.currentTarget.select()}
                      onKeyDown={(e) => e.stopPropagation()}
                      className="h-8 font-mono text-xs"
                    />
                    <CopyButton text={repo.ssh_clone_url} />
                  </div>
                  <p className="mt-1.5 text-[11px] text-muted-foreground">
                    Add a key under Settings &rarr; SSH keys first.
                  </p>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        {latest && (
          <div className="flex items-center gap-2.5 border-b bg-muted/40 px-4 py-2.5 text-sm">
            <CIBadge status={tree.ci_status} runNumber={tree.ci_run} repo={repo.full_name} />
            <Link
              to={`${base}/commit/${latest.sha}`}
              className="min-w-0 truncate font-medium hover:underline"
            >
              {latest.subject}
            </Link>
            <span className="hidden truncate text-muted-foreground sm:inline">
              {latest.author_name} · {timeAgo(latest.when)}
            </span>
            <Link
              to={`${base}/commit/${latest.sha}`}
              className="ml-auto shrink-0 font-mono text-xs text-muted-foreground hover:text-foreground"
            >
              {shortSha(latest.sha)}
            </Link>
          </div>
        )}
        {tree.entries.length === 0 ? (
          <div className="py-12 text-center text-sm text-muted-foreground">This directory is empty.</div>
        ) : (
          <div className="divide-y">
            {path !== "" && (
              <Link
                to={`${base}/tree/${encodePath(refName)}${parent ? `/${encodePath(parent)}` : ""}`}
                className="flex items-center gap-2.5 px-4 py-2 text-sm hover:bg-muted/50"
              >
                <Folder className="size-4 shrink-0 text-muted-foreground" />
                <span className="font-mono text-[13px] text-muted-foreground">..</span>
              </Link>
            )}
            {tree.entries.map((e) => {
              const isDir = e.type === "tree"
              return (
                <Link
                  key={e.path}
                  to={`${base}/${isDir ? "tree" : "blob"}/${encodePath(refName)}/${encodePath(e.path)}`}
                  className="flex items-center gap-2.5 px-4 py-2 text-sm hover:bg-muted/50"
                >
                  {isDir ? (
                    <Folder className="size-4 shrink-0 fill-muted-foreground/15 text-muted-foreground" />
                  ) : (
                    <FileText className="size-4 shrink-0 text-muted-foreground" />
                  )}
                  <span className="truncate">{e.name}</span>
                  {e.type === "blob" && (
                    <span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground">
                      {formatBytes(e.size)}
                    </span>
                  )}
                </Link>
              )
            })}
          </div>
        )}
      </div>

      {tree.readme && (
        <div className="overflow-hidden rounded-xl border bg-card">
          <div className="flex items-center gap-2 border-b bg-muted/40 px-4 py-2.5">
            <BookOpen className="size-4 text-muted-foreground" />
            <span className="font-mono text-[13px] font-semibold">{tree.readme_path ?? "README.md"}</span>
          </div>
          <div className="px-6 py-5 sm:px-8">
            <Markdown html={tree.readme} />
          </div>
        </div>
      )}
    </div>
  )
}

// ---------- blob mode ----------

function BlobView({ blob, refName, path }: { blob: BlobResponse; refName: string; path: string }) {
  const { repo } = useRepo()
  const base = `/${repo.full_name}`
  const isMarkdown = /\.(md|markdown)$/i.test(path) && !!blob.rendered
  const [view, setView] = useState<"rendered" | "source">("rendered")
  const fileName = path.split("/").pop() ?? path
  const lines = blob.content !== undefined ? blob.content.split("\n") : []
  if (lines.length > 0 && lines.at(-1) === "") lines.pop()

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <BranchPicker current={refName} toBranch={(b) => `${base}/tree/${encodePath(b)}`} />
        <PathBreadcrumb refName={refName} path={path} leaf />
        <div className="ml-auto flex items-center gap-2">
          {isMarkdown && (
            <div className="flex overflow-hidden rounded-md border">
              <button
                type="button"
                onClick={() => setView("rendered")}
                className={cn(
                  "px-2.5 py-1.5 text-xs font-medium",
                  view === "rendered" ? "bg-muted text-foreground" : "text-muted-foreground hover:text-foreground",
                )}
              >
                Rendered
              </button>
              <button
                type="button"
                onClick={() => setView("source")}
                className={cn(
                  "border-l px-2.5 py-1.5 text-xs font-medium",
                  view === "source" ? "bg-muted text-foreground" : "text-muted-foreground hover:text-foreground",
                )}
              >
                Source
              </button>
            </div>
          )}
          <Button asChild variant="outline" size="sm">
            <Link to={`${base}/commits/${encodePath(refName)}?path=${encodeURIComponent(path)}`}>
              <History className="size-4" />
              History
            </Link>
          </Button>
          <Button asChild variant="outline" size="sm">
            <a href={blob.raw_url} target="_blank" rel="noreferrer">
              <FileCode2 className="size-4" />
              Raw
            </a>
          </Button>
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="flex items-center gap-2 border-b bg-muted/40 px-4 py-2.5 text-sm">
          <FileText className="size-4 text-muted-foreground" />
          <span className="font-mono text-[13px] font-semibold">{fileName}</span>
          <span className="text-xs text-muted-foreground">
            {!blob.binary && blob.content !== undefined && `${lines.length} lines · `}
            {formatBytes(blob.size)}
          </span>
          {blob.truncated && (
            <span className="text-xs text-tangerine italic">Large file — display truncated</span>
          )}
        </div>

        {blob.binary ? (
          <EmptyState icon={<Download />} title="Binary file" className="m-4 border-0">
            This file can’t be displayed here.{" "}
            <a href={blob.raw_url} className="font-medium text-primary hover:underline" download>
              Download {formatBytes(blob.size)}
            </a>
          </EmptyState>
        ) : isMarkdown && view === "rendered" ? (
          <div className="px-6 py-5 sm:px-8">
            <Markdown html={blob.rendered ?? ""} />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse font-mono text-[12.5px] leading-relaxed">
              <tbody>
                {lines.map((line, i) => (
                  <tr key={i} className="hover:bg-muted/40">
                    <td className="w-10 min-w-12 border-r px-2 text-right align-top text-muted-foreground select-none">
                      {i + 1}
                    </td>
                    <td className="px-4 align-top whitespace-pre">{line}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

// ---------- page ----------

export default function Page() {
  const { repo } = useRepo()
  const splat = useParams()["*"] ?? ""
  const location = useLocation()
  const isBlob = location.pathname.includes("/blob/")
  const { ref, path } = splitRefPath(repo, splat)

  const [tree, setTree] = useState<TreeResponse | null>(null)
  const [blob, setBlob] = useState<BlobResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  useEffect(() => {
    let live = true
    setLoading(true)
    setError("")
    setTree(null)
    setBlob(null)
    const load = async () => {
      try {
        if (isBlob) {
          const b = await api.blob(repo.owner, repo.name, ref, path)
          if (live) setBlob(b)
        } else {
          const t = await api.tree(repo.owner, repo.name, ref, path)
          if (live) setTree(t)
        }
      } catch (e) {
        if (live) setError(e instanceof Error ? e.message : "failed to load")
      } finally {
        if (live) setLoading(false)
      }
    }
    load()
    return () => {
      live = false
    }
  }, [repo.owner, repo.name, ref, path, isBlob])

  if (loading) return <PageLoading />
  if (error) return <ErrorNote message={error} />
  if (isBlob && blob) return <BlobView key={`${ref}:${path}`} blob={blob} refName={ref} path={path} />
  if (tree?.empty) return <QuickSetup />
  if (tree) return <TreeView tree={tree} refName={ref} path={path} />
  return null
}
