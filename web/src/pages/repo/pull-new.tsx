import { useState } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { ArrowLeft, GitPullRequest, Layers } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { api } from "@/lib/api"
import { useRepo } from "@/components/repo-layout"

export default function PullNewPage() {
  const { repo, refresh } = useRepo()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const branches = repo.branches ?? []

  const [base, setBase] = useState(params.get("base") || repo.default_branch)
  const [head, setHead] = useState(params.get("head") || "")
  const [title, setTitle] = useState("")
  const [body, setBody] = useState("")
  const [busy, setBusy] = useState(false)

  const canSubmit = title.trim() !== "" && base !== "" && head !== "" && base !== head

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!canSubmit || busy) return
    setBusy(true)
    try {
      const pull = await api.createPull(repo.owner, repo.name, {
        title: title.trim(),
        body,
        base,
        head,
      })
      refresh()
      navigate(`/${repo.full_name}/pull/${pull.number}`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to create pull request")
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <h2 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
        <GitPullRequest className="size-5 text-primary" /> New pull request
      </h2>

      <Card className="rounded-xl">
        <CardHeader>
          <CardTitle className="text-base">Choose branches</CardTitle>
          <CardDescription>
            The head branch's commits will be merged into the base branch.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-6" onSubmit={submit}>
            <div className="flex flex-wrap items-end gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="pr-base" className="text-xs text-muted-foreground">
                  base
                </Label>
                <Select value={base} onValueChange={setBase}>
                  <SelectTrigger id="pr-base" className="min-w-44 font-mono text-xs">
                    <SelectValue placeholder="base branch" />
                  </SelectTrigger>
                  <SelectContent>
                    {branches.map((b) => (
                      <SelectItem key={b.name} value={b.name} className="font-mono text-xs">
                        {b.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <ArrowLeft className="mb-2.5 size-4 shrink-0 text-muted-foreground" />
              <div className="space-y-1.5">
                <Label htmlFor="pr-head" className="text-xs text-muted-foreground">
                  head
                </Label>
                <Select value={head} onValueChange={setHead}>
                  <SelectTrigger id="pr-head" className="min-w-44 font-mono text-xs">
                    <SelectValue placeholder="head branch" />
                  </SelectTrigger>
                  <SelectContent>
                    {branches.map((b) => (
                      <SelectItem key={b.name} value={b.name} className="font-mono text-xs">
                        {b.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
              <Layers className="mt-0.5 size-3.5 shrink-0 text-tangerine" />
              Choose another PR's head branch as base to stack this PR on top of it.
            </p>
            {base === head && head !== "" && (
              <p className="text-xs text-destructive">Base and head must be different branches.</p>
            )}

            <div className="space-y-1.5">
              <Label htmlFor="pr-title">Title</Label>
              <Input
                id="pr-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="A short summary of the change"
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="pr-body">Description</Label>
              <Textarea
                id="pr-body"
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder="Describe what changed and why…"
                rows={8}
              />
              <p className="text-xs text-muted-foreground">Markdown is supported.</p>
            </div>

            <div className="flex justify-end">
              <Button type="submit" disabled={!canSubmit || busy}>
                <GitPullRequest className="size-4" />
                {busy ? "Creating…" : "Create pull request"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
