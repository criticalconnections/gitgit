import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/api"
import { useRepo } from "@/components/repo-layout"

export default function IssueNewPage() {
  const { repo } = useRepo()
  const navigate = useNavigate()
  const [title, setTitle] = useState("")
  const [body, setBody] = useState("")
  const [saving, setSaving] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim() || saving) return
    setSaving(true)
    try {
      const issue = await api.createIssue(repo.owner, repo.name, title.trim(), body)
      navigate(`/${repo.full_name}/issue/${issue.number}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to create issue")
      setSaving(false)
    }
  }

  return (
    <div className="mx-auto max-w-3xl">
      <Card>
        <CardHeader>
          <CardTitle className="text-xl tracking-tight">New issue</CardTitle>
          <CardDescription>Report a bug, propose an idea, or track a task for {repo.full_name}.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="issue-title">Title</Label>
              <Input
                id="issue-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="A short, descriptive summary"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="issue-body">Description</Label>
              <Textarea
                id="issue-body"
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder="Steps to reproduce, context, screenshots…"
                rows={10}
                className="min-h-48 font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">Markdown is supported.</p>
            </div>
            <div className="flex items-center justify-end gap-2">
              <Button asChild type="button" variant="ghost">
                <Link to={`/${repo.full_name}/issues`}>Cancel</Link>
              </Button>
              <Button type="submit" disabled={!title.trim() || saving}>
                {saving ? "Creating…" : "Create issue"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
