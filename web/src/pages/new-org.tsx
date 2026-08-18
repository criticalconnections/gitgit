import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { Building2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { PageLoading } from "@/components/shared"
import { useAuth } from "@/lib/auth"
import { api } from "@/lib/api"

export default function NewOrg() {
  const { user, loading } = useAuth()
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!loading && !user) navigate("/login")
  }, [loading, user, navigate])

  if (loading || !user) return <PageLoading />

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || busy) return
    setBusy(true)
    try {
      const org = await api.createOrg(name.trim(), description.trim())
      navigate(`/${org.username}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "failed to create organization")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex justify-center py-10">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-xl tracking-tight">
            <Building2 className="size-5 text-primary" /> Create an organization
          </CardTitle>
          <CardDescription>
            Organizations own repositories together. Owners administer every repository the
            organization owns; members can read them and be added to individual repositories.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="org-name">Name</Label>
              <Input
                id="org-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="acme"
                pattern="[A-Za-z0-9._\-]+"
                title="Letters, numbers, dots, dashes and underscores only"
                autoFocus
                required
              />
              <p className="text-xs text-muted-foreground">
                Repositories will live at <span className="font-mono">{name || "acme"}/…</span>.
                This name shares the same space as usernames.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="org-desc">
                Description <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Input
                id="org-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What this organization is for"
              />
            </div>
            <Button type="submit" disabled={busy || !name.trim()} className="w-full">
              {busy ? "Creating…" : "Create organization"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
