import { useEffect, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { PageLoading } from "@/components/shared"
import { api, type Org } from "@/lib/api"
import { useAuth } from "@/lib/auth"

export default function NewRepo() {
  const { user, loading } = useAuth()
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [isPrivate, setIsPrivate] = useState(false)
  const [autoInit, setAutoInit] = useState(true)
  const [busy, setBusy] = useState(false)
  const [owner, setOwner] = useState("")
  const [orgs, setOrgs] = useState<Org[]>([])

  useEffect(() => {
    if (!loading && !user) navigate("/login")
  }, [loading, user, navigate])

  // Organizations you own are valid owners for a new repository; ones you are
  // merely a member of are not, so the picker only offers what will succeed.
  useEffect(() => {
    if (!user) return
    api
      .myOrgs()
      .then((all) => setOrgs(all.filter((o) => o.role === "owner")))
      .catch(() => setOrgs([]))
  }, [user])

  if (loading || !user) return <PageLoading />

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || busy) return
    setBusy(true)
    try {
      const created = await api.createRepo({
        name: name.trim(),
        description: description.trim() || undefined,
        private: isPrivate,
        auto_init: autoInit,
        owner: owner || undefined,
      })
      navigate(`/${created.full_name}`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "failed to create repository")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex justify-center py-10">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle className="text-xl tracking-tight">Create a new repository</CardTitle>
          <CardDescription>
            A repository contains your project&rsquo;s files, history, pull requests and CI.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="repo-name">Repository name</Label>
              <div className="flex items-center gap-2">
                {orgs.length > 0 ? (
                  <select
                    value={owner}
                    onChange={(e) => setOwner(e.target.value)}
                    aria-label="Owner"
                    className="h-9 max-w-40 shrink-0 truncate rounded-md border bg-background px-2 text-sm"
                  >
                    <option value="">{user.username}</option>
                    {orgs.map((o) => (
                      <option key={o.username} value={o.username}>
                        {o.username}
                      </option>
                    ))}
                  </select>
                ) : (
                  <span className="shrink-0 text-sm text-muted-foreground">{user.username}</span>
                )}
                <span className="shrink-0 text-sm text-muted-foreground">/</span>
                <Input
                  id="repo-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="my-project"
                  pattern="[A-Za-z0-9._\-]+"
                  title="Letters, numbers, dots, dashes and underscores only"
                  autoFocus
                  required
                  className="font-mono"
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Letters, numbers, dots, dashes and underscores.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="repo-desc">
                Description <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Input
                id="repo-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="A short sentence about this project"
              />
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
                <div>
                  <Label htmlFor="repo-private">Private repository</Label>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Only you and collaborators can see this repository.
                  </p>
                </div>
                <Switch id="repo-private" checked={isPrivate} onCheckedChange={setIsPrivate} />
              </div>
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
                <div>
                  <Label htmlFor="repo-init">Initialize with a README</Label>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Start with a first commit so you can clone right away.
                  </p>
                </div>
                <Switch id="repo-init" checked={autoInit} onCheckedChange={setAutoInit} />
              </div>
            </div>

            <div className="flex justify-end">
              <Button type="submit" disabled={busy || !name.trim()}>
                {busy ? "Creating…" : "Create repository"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <p className="mt-4 text-center text-sm text-muted-foreground">
        Already have one elsewhere?{" "}
        <Link to="/import" className="font-medium text-primary hover:underline">
          Import from GitHub
        </Link>
      </p>
    </div>
  )
}
