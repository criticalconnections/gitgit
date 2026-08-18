import { Link, Outlet, useLocation, useNavigate } from "react-router-dom"
import { Bell, Building2, CloudDownload, Compass, LogOut, Moon, Plus, Search as SearchIcon, Settings, Sun, User as UserIcon } from "lucide-react"
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
import { api } from "@/lib/api"
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
// NotificationBell polls for the unread count. Polling rather than a socket:
// one small query a minute costs far less than holding a connection open for
// something nobody needs to hear about within the second.
function NotificationBell() {
  const [unread, setUnread] = useState(0)
  const location = useLocation()

  useEffect(() => {
    let cancelled = false
    const check = () =>
      api
        .unreadCount()
        .then((r) => !cancelled && setUnread(r.unread))
        .catch(() => {})
    check()
    const t = setInterval(check, 60_000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
    // re-checked on navigation too, so reading the inbox clears the badge
  }, [location.pathname])

  return (
    <Button asChild variant="ghost" size="icon" className="relative size-8 text-muted-foreground">
      <Link to="/notifications" title={unread ? `${unread} unread notifications` : "Notifications"}>
        <Bell className="size-4" />
        {unread > 0 && (
          <span className="absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-primary px-1 text-[10px] font-semibold text-primary-foreground tabular-nums">
            {unread > 9 ? "9+" : unread}
          </span>
        )}
        <span className="sr-only">Notifications</span>
      </Link>
    </Button>
  )
}

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
          <form
            onSubmit={(e) => {
              e.preventDefault()
              const q = new FormData(e.currentTarget).get("q")?.toString().trim()
              if (q) navigate(`/search?q=${encodeURIComponent(q)}`)
            }}
            className="ml-auto hidden md:block"
          >
            <div className="relative">
              <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                name="q"
                placeholder="Search…"
                aria-label="Search"
                className="h-8 w-52 rounded-md border bg-background pr-2 pl-8 text-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
              />
            </div>
          </form>
          <div className="flex items-center gap-2 md:ml-2">
            <Button variant="ghost" size="icon" className="size-8 text-muted-foreground" onClick={toggle}>
              {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </Button>
            {user ? (
              <>
                <NotificationBell />
                <Button asChild size="sm" variant="outline">
                  <Link to="/new">
                    <Plus className="size-4" />
                    New repository
                  </Link>
                </Button>
                <Button asChild size="sm" variant="ghost" className="text-muted-foreground">
                  <Link to="/import" title="Import from GitHub">
                    <CloudDownload className="size-4" />
                    <span className="sr-only sm:not-sr-only">Import</span>
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
                    <DropdownMenuItem onClick={() => navigate("/organizations/new")}>
                      <Building2 className="size-4" /> New organization
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
