// Standalone account-creation page (no app chrome).
import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Logo } from "@/components/logo"
import { ErrorNote } from "@/components/shared"
import { useTheme } from "@/components/shell"
import { api } from "@/lib/api"
import { useAuth } from "@/lib/auth"

export default function Register() {
  useTheme() // applies the persisted theme outside the AppShell
  const { refresh } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.register(username.trim(), email.trim(), password)
      await refresh()
      navigate("/dashboard")
    } catch (err) {
      setError(err instanceof Error ? err.message : "registration failed")
      setBusy(false)
    }
  }

  return (
    <div className="relative grid min-h-screen place-items-center bg-muted/30 px-6 py-16">
      <Link
        to="/"
        className="absolute top-6 left-6 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        ← gitgit.io
      </Link>

      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center">
          <Link to="/">
            <Logo className="text-xl" />
          </Link>
        </div>

        <Card className="w-full">
          <CardHeader className="text-center">
            <CardTitle className="text-lg tracking-tight">Create your account</CardTitle>
            <CardDescription>The first account becomes the site admin.</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit} className="grid gap-4">
              {error && <ErrorNote message={error} />}
              <div className="grid gap-2">
                <Label htmlFor="username">Username</Label>
                <Input
                  id="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  autoFocus
                  required
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="email">
                  Email <span className="font-normal text-muted-foreground">(optional)</span>
                </Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoComplete="email"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="new-password"
                  required
                />
              </div>
              <Button type="submit" className="w-full" disabled={busy}>
                {busy && <Loader2 className="size-4 animate-spin" />}
                Create account
              </Button>
            </form>
          </CardContent>
        </Card>

        <p className="mt-6 text-center text-sm text-muted-foreground">
          Already have an account?{" "}
          <Link to="/login" className="font-medium text-foreground underline-offset-4 hover:underline">
            Sign in
          </Link>
        </p>
      </div>
    </div>
  )
}
