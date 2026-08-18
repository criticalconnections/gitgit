import { useEffect, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { BookOpen, CircleDot, Code2, GitPullRequest, Search as SearchIcon, User as UserIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { EmptyState, PageLoading, UserAvatar } from "@/components/shared"
import { api, type SearchResults } from "@/lib/api"
import { cn } from "@/lib/utils"

const TABS = [
  { key: "", label: "Everything" },
  { key: "repos", label: "Repositories" },
  { key: "issues", label: "Issues" },
  { key: "pulls", label: "Pull requests" },
  { key: "code", label: "Code" },
  { key: "users", label: "People" },
]

export default function Search() {
  const [params, setParams] = useSearchParams()
  const q = params.get("q") ?? ""
  const type = params.get("type") ?? ""
  const [draft, setDraft] = useState(q)
  const [results, setResults] = useState<SearchResults | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => setDraft(q), [q])

  useEffect(() => {
    if (!q) {
      setResults(null)
      return
    }
    let cancelled = false
    setBusy(true)
    api
      .search(q, type || undefined)
      .then((r) => !cancelled && setResults(r))
      .catch(() => !cancelled && setResults(null))
      .finally(() => !cancelled && setBusy(false))
    return () => {
      cancelled = true
    }
  }, [q, type])

  const total =
    (results?.repos?.length ?? 0) +
    (results?.issues?.length ?? 0) +
    (results?.code?.length ?? 0) +
    (results?.users?.length ?? 0)

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setParams(draft.trim() ? { q: draft.trim(), ...(type ? { type } : {}) } : {})
        }}
        className="flex gap-2"
      >
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Search repositories, issues, code and people"
          autoFocus
          className="flex-1"
        />
        <Button type="submit" disabled={!draft.trim()}>
          <SearchIcon className="size-4" /> Search
        </Button>
      </form>

      {q && (
        <div className="flex flex-wrap gap-1.5">
          {TABS.map((t) => (
            <Button
              key={t.key}
              size="sm"
              variant={type === t.key ? "secondary" : "ghost"}
              onClick={() => setParams(t.key ? { q, type: t.key } : { q })}
            >
              {t.label}
            </Button>
          ))}
        </div>
      )}

      {busy && !results ? (
        <PageLoading />
      ) : !q ? (
        <EmptyState icon={<SearchIcon />} title="Search GitGit">
          Code search runs over the default branch of every repository you can see.
        </EmptyState>
      ) : total === 0 ? (
        <EmptyState icon={<SearchIcon />} title={`Nothing matched “${q}”`}>
          Try fewer words, or a different tab.
        </EmptyState>
      ) : (
        <div className="space-y-8">
          {!!results?.repos?.length && (
            <section className="space-y-2">
              <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
                <BookOpen className="size-4 text-primary" /> Repositories
                <Badge variant="secondary" className="rounded-full px-2">{results.repos.length}</Badge>
              </h2>
              <div className="divide-y rounded-lg border">
                {results.repos.map((r) => (
                  <Link key={r.id} to={`/${r.full_name}`} className="block px-4 py-3 hover:bg-muted/50">
                    <div className="font-medium text-primary">{r.full_name}</div>
                    <p className="truncate text-sm text-muted-foreground">
                      {r.description || "No description provided."}
                    </p>
                  </Link>
                ))}
              </div>
            </section>
          )}

          {!!results?.issues?.length && (
            <section className="space-y-2">
              <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
                <CircleDot className="size-4 text-primary" /> Issues &amp; pull requests
                <Badge variant="secondary" className="rounded-full px-2">{results.issues.length}</Badge>
              </h2>
              <div className="divide-y rounded-lg border">
                {results.issues.map((h) => {
                  const Icon = h.type === "pull" ? GitPullRequest : CircleDot
                  return (
                    <Link key={`${h.repo}-${h.type}-${h.number}`} to={h.url} className="flex items-start gap-3 px-4 py-3 hover:bg-muted/50">
                      <Icon className={cn("mt-0.5 size-4 shrink-0", h.state === "open" ? "text-primary" : "text-muted-foreground")} />
                      <div className="min-w-0">
                        <div className="truncate font-medium">{h.title}</div>
                        <div className="text-xs text-muted-foreground">
                          {h.repo}#{h.number} · {h.state}
                        </div>
                      </div>
                    </Link>
                  )
                })}
              </div>
            </section>
          )}

          {!!results?.code?.length && (
            <section className="space-y-2">
              <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
                <Code2 className="size-4 text-primary" /> Code
                <Badge variant="secondary" className="rounded-full px-2">{results.code.length}</Badge>
              </h2>
              <div className="divide-y rounded-lg border">
                {results.code.map((m, i) => (
                  <Link
                    key={`${m.repo}-${m.path}-${m.line}-${i}`}
                    to={`/${m.repo}/blob/${m.ref}/${m.path}#L${m.line}`}
                    className="block px-4 py-3 hover:bg-muted/50"
                  >
                    <div className="truncate text-xs text-muted-foreground">
                      {m.repo} <span className="font-mono">{m.path}</span>:{m.line}
                    </div>
                    <pre className="mt-1 overflow-x-auto rounded bg-muted/60 px-2 py-1 font-mono text-xs">
                      {m.text}
                    </pre>
                  </Link>
                ))}
              </div>
            </section>
          )}

          {!!results?.users?.length && (
            <section className="space-y-2">
              <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
                <UserIcon className="size-4 text-primary" /> People
                <Badge variant="secondary" className="rounded-full px-2">{results.users.length}</Badge>
              </h2>
              <div className="divide-y rounded-lg border">
                {results.users.map((u) => (
                  <Link key={u.id} to={`/${u.username}`} className="flex items-center gap-3 px-4 py-2.5 hover:bg-muted/50">
                    <UserAvatar user={u} className="size-7" />
                    <span className="font-medium">{u.username}</span>
                    {u.is_org && (
                      <Badge variant="outline" className="rounded-full text-muted-foreground">organization</Badge>
                    )}
                    {u.full_name && <span className="text-sm text-muted-foreground">{u.full_name}</span>}
                  </Link>
                ))}
              </div>
            </section>
          )}
        </div>
      )}
    </div>
  )
}
