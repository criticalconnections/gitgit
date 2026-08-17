import { useCallback, useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { GitBranch, GitPullRequest, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, type BranchRow } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { EmptyState, ErrorNote, PageLoading } from "@/components/shared"
import { useRepo } from "@/components/repo-layout"

const encodePath = (p: string) => p.split("/").map(encodeURIComponent).join("/")

export default function Page() {
  const { repo } = useRepo()
  const [branches, setBranches] = useState<BranchRow[] | null>(null)
  const [error, setError] = useState("")
  const [toDelete, setToDelete] = useState<BranchRow | null>(null)
  const [deleting, setDeleting] = useState(false)

  const base = `/${repo.full_name}`

  const load = useCallback(async () => {
    try {
      setBranches(await api.branches(repo.owner, repo.name))
      setError("")
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load branches")
    }
  }, [repo.owner, repo.name])

  useEffect(() => {
    setBranches(null)
    load()
  }, [load])

  const confirmDelete = async () => {
    if (!toDelete) return
    setDeleting(true)
    try {
      await api.deleteBranch(repo.owner, repo.name, toDelete.name)
      toast.success(`Deleted branch ${toDelete.name}`)
      setToDelete(null)
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to delete branch")
    } finally {
      setDeleting(false)
    }
  }

  if (error) return <ErrorNote message={error} />
  if (!branches) return <PageLoading />

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="text-lg font-semibold tracking-tight">Branches</h2>
        <Badge variant="secondary" className="rounded-full">
          {branches.length}
        </Badge>
      </div>

      {branches.length === 0 ? (
        <EmptyState icon={<GitBranch />} title="No branches yet">
          Push a commit to create the first branch of this repository.
        </EmptyState>
      ) : (
        <div className="overflow-hidden rounded-xl border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead>Branch</TableHead>
                <TableHead className="w-32">Ahead / behind</TableHead>
                <TableHead className="w-40">Pull request</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {branches.map((b) => (
                <TableRow key={b.name}>
                  <TableCell className="py-3">
                    <div className="flex items-center gap-2">
                      <Link
                        to={`${base}/tree/${encodePath(b.name)}`}
                        className="truncate font-mono text-[13px] font-medium hover:text-primary hover:underline"
                      >
                        {b.name}
                      </Link>
                      {b.default && (
                        <Badge variant="outline" className="rounded-full text-muted-foreground">
                          default
                        </Badge>
                      )}
                    </div>
                    <p className="mt-0.5 max-w-md truncate text-xs text-muted-foreground">
                      {b.subject} · {b.author_name} · {timeAgo(b.when)}
                    </p>
                  </TableCell>
                  <TableCell>
                    {b.default ? (
                      <span className="text-xs text-muted-foreground">—</span>
                    ) : (
                      <span className="font-mono text-xs text-muted-foreground">
                        ↑{b.ahead ?? 0} ↓{b.behind ?? 0}
                      </span>
                    )}
                  </TableCell>
                  <TableCell>
                    {b.pull ? (
                      <Link
                        to={`${base}/pull/${b.pull}`}
                        className="inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
                      >
                        <GitPullRequest className="size-3.5" />#{b.pull}
                      </Link>
                    ) : b.default ? null : (
                      <Button asChild variant="outline" size="sm" className="h-7 text-xs">
                        <Link
                          to={`${base}/pulls/new?base=${encodeURIComponent(repo.default_branch)}&head=${encodeURIComponent(b.name)}`}
                        >
                          New PR
                        </Link>
                      </Button>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    {!b.default && repo.can_write && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-7 text-muted-foreground hover:text-destructive"
                        title={`Delete ${b.name}`}
                        onClick={() => setToDelete(b)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={toDelete !== null} onOpenChange={(open) => !open && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete branch?</DialogTitle>
            <DialogDescription>
              This will permanently delete{" "}
              <span className="font-mono font-medium text-foreground">{toDelete?.name}</span>. This action cannot
              be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)} disabled={deleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={deleting}>
              {deleting ? "Deleting…" : "Delete branch"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
