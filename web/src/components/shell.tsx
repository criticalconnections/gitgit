import { Link, Outlet, useNavigate } from "react-router-dom"
import { Compass, LogOut, Moon, Plus, Settings, Sun, User as UserIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Logo } from "@/components/logo"
import { UserAvatar } from "@/components/shared"
import { useAuth } from "@/lib/auth"
import { Toaster } from "@/components/ui/sonner"

export function useTheme() {
  const [dark, setDark] = useState(() => localStorage.getItem("gitgit-theme") === "dark")
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark)
    localStorage.setItem("gitgit-theme", dark ? "dark" : "light")
  }, [dark])
  return { dark, toggle: () => setDark((d) => !d) }
}

// AppShell wraps all product pages: sticky glass topbar + content outlet.
// The marketing landing page renders outside of it.
export function AppShell() {
  const { user, signOut } = useAuth()
  const { dark, toggle } = useTheme()
  const navigate = useNavigate()

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur-xl">
        <div className="mx-auto flex h-14 w-full max-w-7xl items-center gap-4 px-4 sm:px-6">
          <Link to={user ? "/dashboard" : "/"} className="shrink-0">
            <Logo className="text-[17px]" />
          </Link>
          <nav className="hidden items-center gap-1 sm:flex">
            <Button asChild variant="ghost" size="sm" className="text-muted-foreground">
              <Link to="/explore">
                <Compass className="size-4" />
                Explore
              </Link>
            </Button>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <Button variant="ghost" size="icon" className="size-8 text-muted-foreground" onClick={toggle}>
              {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </Button>
            {user ? (
              <>
                <Button asChild size="sm" variant="outline">
                  <Link to="/new">
                    <Plus className="size-4" />
                    New repository
                  </Link>
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger className="rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring">
                    <UserAvatar user={user} className="size-8" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-52">
                    <DropdownMenuLabel>
                      <div className="font-semibold">{user.username}</div>
                      {user.full_name && <div className="text-xs font-normal text-muted-foreground">{user.full_name}</div>}
                    </DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={() => navigate(`/${user.username}`)}>
                      <UserIcon className="size-4" /> Profile
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => navigate("/settings")}>
                      <Settings className="size-4" /> Settings
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      onClick={async () => {
                        await signOut()
                        navigate("/")
                      }}
                    >
                      <LogOut className="size-4" /> Sign out
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </>
            ) : (
              <>
                <Button asChild variant="ghost" size="sm">
                  <Link to="/login">Sign in</Link>
                </Button>
                <Button asChild size="sm">
                  <Link to="/register">Get started</Link>
                </Button>
              </>
            )}
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <Outlet />
      </main>
      <footer className="border-t py-6">
        <div className="mx-auto w-full max-w-7xl px-4 text-sm text-muted-foreground sm:px-6">
          <Logo className="text-sm" /> <span className="ml-2">Code together. Ship further.</span>
        </div>
      </footer>
      <Toaster position="bottom-right" />
    </div>
  )
}
