// GitGit marketing landing page. Standalone: renders its own nav + footer.
import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import {
  ArrowRight,
  CheckCircle2,
  CircleDot,
  FileCode2,
  GitBranch,
  GitMerge,
  GitPullRequest,
  Layers,
  Lock,
  Moon,
  Sun,
  Webhook,
  Workflow,
  type LucideIcon,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Logo } from "@/components/logo"
import { CIBadge, PRStateBadge } from "@/components/shared"
import { useTheme } from "@/components/shell"
import { useAuth } from "@/lib/auth"
import { cn } from "@/lib/utils"

// ---------- nav ----------

const NAV_LINKS = [
  { label: "Features", href: "#features" },
  { label: "Stacked PRs", href: "#stacked" },
  { label: "CI/CD", href: "#ci" },
  { label: "Open source", href: "#open-source" },
]

function LandingNav() {
  const { user } = useAuth()
  const { dark, toggle } = useTheme()
  return (
    <header className="sticky top-0 z-50 border-b border-border/60 bg-background/70 backdrop-blur-xl">
      <div className="mx-auto flex h-16 w-full max-w-6xl items-center px-6">
        <Link to="/" className="shrink-0">
          <Logo className="text-[17px]" />
        </Link>
        <nav className="ml-8 hidden items-center gap-1 md:flex">
          {NAV_LINKS.map((l) => (
            <Button key={l.href} asChild variant="ghost" size="sm" className="text-muted-foreground">
              <a href={l.href}>{l.label}</a>
            </Button>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" size="icon" className="size-8 text-muted-foreground" onClick={toggle}>
            {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            <span className="sr-only">Toggle theme</span>
          </Button>
          {user ? (
            <Button asChild size="sm">
              <Link to="/dashboard">
                Open app
                <ArrowRight className="size-4" />
              </Link>
            </Button>
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
  )
}

// ---------- hero mockup: a faux stacked-PR browser window ----------

interface MockStackRow {
  n: number
  title: string
  glyph: string
  current?: boolean
}

const MOCK_STACK: MockStackRow[] = [
  { n: 7, title: "Parser core", glyph: "⏚" },
  { n: 8, title: "Formatter", glyph: "↳", current: true },
  { n: 9, title: "CLI entrypoint", glyph: "↳" },
]

function ProductMockup() {
  return (
    <div className="relative mx-auto mt-16 w-full max-w-3xl">
      <div aria-hidden className="absolute -inset-x-10 top-12 -bottom-10 rounded-[3rem] bg-primary/5 blur-2xl" />
      <div className="relative overflow-hidden rounded-2xl border bg-card text-left shadow-2xl">
        {/* title bar */}
        <div className="flex items-center border-b bg-muted/40 px-4 py-2.5">
          <div className="flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-destructive/60" />
            <span className="size-2.5 rounded-full bg-tangerine/70" />
            <span className="size-2.5 rounded-full bg-primary/70" />
          </div>
          <div className="mx-auto hidden items-center gap-1.5 rounded-md border bg-background px-3 py-1 font-mono text-[11px] text-muted-foreground sm:flex">
            <Lock className="size-3" />
            gitgit.dev/desmond/demo/pull/8
          </div>
          <span className="hidden w-[46px] sm:block" aria-hidden />
        </div>

        <div className="p-5 sm:p-6">
          {/* PR header */}
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <h3 className="text-[15px] font-semibold tracking-tight">
              Formatter: pretty-print numeric literals{" "}
              <span className="font-normal text-muted-foreground">#8</span>
            </h3>
            <PRStateBadge state="open" className="px-2 py-0.5 text-[10px] [&_svg]:size-3" />
          </div>
          <p className="mt-2 font-mono text-[11px] text-muted-foreground">
            desmond wants to merge{" "}
            <span className="rounded bg-primary/10 px-1.5 py-0.5 text-primary">fmt/numeric</span> into{" "}
            <span className="rounded bg-muted px-1.5 py-0.5">fmt/parser</span>
          </p>

          <div className="mt-5 grid gap-4 sm:grid-cols-[minmax(0,15rem)_minmax(0,1fr)]">
            {/* stack panel */}
            <div className="rounded-lg border bg-muted/30 p-2">
              <p className="px-2 pt-1 pb-2 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
                Stack · 3 pull requests
              </p>
              <div className="space-y-0.5">
                {MOCK_STACK.map((row) => (
                  <div
                    key={row.n}
                    className={cn(
                      "flex items-center gap-2 rounded-md px-2 py-1.5",
                      row.current && "bg-background shadow-xs ring-1 ring-primary/30",
                    )}
                  >
                    <span className="w-4 text-center font-mono text-xs text-muted-foreground">{row.glyph}</span>
                    <span className="font-mono text-[11px] text-muted-foreground">#{row.n}</span>
                    <span className="truncate text-xs font-medium">{row.title}</span>
                    <CIBadge status="success" className="ml-auto [&_svg]:size-3.5" />
                  </div>
                ))}
              </div>
            </div>

            {/* checks + merge box */}
            <div className="flex flex-col rounded-lg border">
              <div className="flex items-center gap-2 border-b px-4 py-2">
                <CheckCircle2 className="size-3.5 text-primary" />
                <span className="font-mono text-xs">build</span>
                <span className="ml-auto font-mono text-[11px] text-muted-foreground">41s</span>
              </div>
              <div className="flex items-center gap-2 border-b px-4 py-2">
                <CheckCircle2 className="size-3.5 text-primary" />
                <span className="font-mono text-xs">test</span>
                <span className="ml-auto font-mono text-[11px] text-muted-foreground">1m 12s</span>
              </div>
              <div className="flex flex-1 flex-wrap items-center gap-3 px-4 py-3">
                <div className="min-w-0">
                  <p className="flex items-center gap-1.5 text-[13px] font-semibold">
                    <GitMerge className="size-3.5 text-primary" />
                    No conflicts with base
                  </p>
                  <p className="mt-0.5 text-[11px] text-muted-foreground">All checks passed · 2 approvals</p>
                </div>
                <Button size="sm" className="pointer-events-none ml-auto h-7 px-3 text-xs" tabIndex={-1}>
                  Merge pull request
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ---------- small building blocks ----------

function SectionHeading({ children, sub }: { children: ReactNode; sub?: string }) {
  return (
    <div className="mx-auto max-w-2xl text-center">
      <h2 className="text-3xl font-semibold tracking-tight text-balance sm:text-4xl">{children}</h2>
      {sub && <p className="mt-4 text-base text-pretty text-muted-foreground sm:text-lg">{sub}</p>}
    </div>
  )
}

function Point({ children }: { children: ReactNode }) {
  return (
    <li className="flex items-start gap-3 text-sm text-muted-foreground">
      <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-primary" />
      <span>{children}</span>
    </li>
  )
}

// ---------- stats ----------

const STATS = [
  { value: "1", label: "binary to deploy" },
  { value: "0", label: "external services" },
  { value: "3", label: "merge strategies" },
  { value: "∞", label: "private repos" },
]

// ---------- features ----------

const FEATURES: { icon: LucideIcon; title: string; copy: string }[] = [
  {
    icon: GitBranch,
    title: "Git over HTTP",
    copy: "Clone, push, and pull over smart HTTP with access tokens — no SSH keys to babysit.",
  },
  {
    icon: GitPullRequest,
    title: "Pull requests & review",
    copy: "Inline comments, approvals, required reviewers, and three merge strategies.",
  },
  {
    icon: Layers,
    title: "Stacked PRs",
    copy: "Chain dependent branches and merge bottom-first while the stack retargets itself.",
  },
  {
    icon: Workflow,
    title: "Built-in CI",
    copy: "One YAML file in your repo and every push gets a build. No runners to rent.",
  },
  {
    icon: CircleDot,
    title: "Issues & labels",
    copy: "Lightweight issue tracking with labels, markdown, and a timeline that stays out of the way.",
  },
  {
    icon: Webhook,
    title: "Webhooks & API",
    copy: "Outbound webhooks and a clean JSON API for everything you see in the UI.",
  },
]

// ---------- stacked-PR deep-dive diagram ----------

const STACK_DIAGRAM = [
  { n: 7, title: "Parser core", branch: "fmt/parser", base: "main", glyph: "⏚" },
  { n: 8, title: "Formatter", branch: "fmt/numeric", base: "fmt/parser", glyph: "↳" },
  { n: 9, title: "CLI entrypoint", branch: "fmt/cli", base: "fmt/numeric", glyph: "↳" },
]

function StackDiagram() {
  return (
    <div className="relative mx-auto w-full max-w-sm">
      <div aria-hidden className="absolute top-4 bottom-8 left-[1.4rem] w-px bg-border" />
      <div className="relative flex flex-col gap-3">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center gap-1.5 rounded-full border bg-muted/50 py-1 pr-3 pl-2.5 font-mono text-xs text-muted-foreground">
            <GitBranch className="size-3.5" />
            main
          </span>
        </div>
        {STACK_DIAGRAM.map((item) => (
          <div key={item.n} className="flex items-center gap-3 rounded-xl border bg-card p-4 shadow-sm">
            <span className="w-4 shrink-0 text-center font-mono text-sm text-muted-foreground">{item.glyph}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                <span className="mr-1.5 font-mono text-xs text-muted-foreground">#{item.n}</span>
                {item.title}
              </p>
              <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
                {item.branch} → {item.base}
              </p>
            </div>
            <CheckCircle2 className="size-4 shrink-0 text-primary" />
          </div>
        ))}
      </div>
      <p className="mt-5 text-center text-sm text-muted-foreground">
        Merge <span className="font-mono text-xs">#7</span> and <span className="font-mono text-xs">#8</span> retargets
        to <span className="font-mono text-xs">main</span> — automatically.
      </p>
    </div>
  )
}

// ---------- CI terminal card (deliberately dark in both themes) ----------

function CITerminal() {
  return (
    <div className="mx-auto w-full max-w-md overflow-hidden rounded-xl border border-zinc-800 bg-zinc-950 shadow-2xl">
      <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2.5">
        <FileCode2 className="size-3.5 text-zinc-500" />
        <span className="font-mono text-xs text-zinc-400">.gitgit/ci.yml</span>
      </div>
      <pre className="overflow-x-auto p-5 font-mono text-[13px] leading-6">
        <code>
          <span className="text-zinc-500">jobs:</span>
          {"\n"}
          <span className="text-zinc-500">{"  build:"}</span>
          {"\n"}
          <span className="text-zinc-500">{"    steps:"}</span>
          {"\n"}
          <span className="text-zinc-600">{"      - run: "}</span>
          <span className="text-zinc-100">go vet ./...</span>
          {"\n"}
          <span className="text-zinc-600">{"      - run: "}</span>
          <span className="text-zinc-100">go build ./...</span>
          {"\n"}
          <span className="text-zinc-500">{"  test:"}</span>
          {"\n"}
          <span className="text-zinc-500">{"    steps:"}</span>
          {"\n"}
          <span className="text-zinc-600">{"      - run: "}</span>
          <span className="text-zinc-100">go test ./...</span>
        </code>
      </pre>
      <div className="space-y-1.5 border-t border-zinc-800 px-5 py-3.5 font-mono text-xs">
        <p>
          <span className="text-primary">✓ build</span>
          <span className="text-zinc-500"> — passed in 41s</span>
        </p>
        <p>
          <span className="text-primary">✓ test</span>
          <span className="text-zinc-500"> — passed in 1m 12s</span>
        </p>
      </div>
    </div>
  )
}

// ---------- footer ----------

interface FooterLink {
  label: string
  to: string
}

function FooterCol({ title, links }: { title: string; links: FooterLink[] }) {
  return (
    <div>
      <h4 className="text-sm font-semibold">{title}</h4>
      <ul className="mt-4 space-y-2.5 text-sm text-muted-foreground">
        {links.map((l) => (
          <li key={l.label}>
            {l.to.startsWith("/") ? (
              <Link to={l.to} className="transition-colors hover:text-foreground">
                {l.label}
              </Link>
            ) : (
              <a href={l.to} className="transition-colors hover:text-foreground">
                {l.label}
              </a>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

// ---------- page ----------

export default function Landing() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <LandingNav />

      {/* hero */}
      <section className="relative overflow-hidden">
        <div aria-hidden className="pointer-events-none absolute inset-0">
          <div className="absolute -top-48 left-1/2 h-[32rem] w-[52rem] -translate-x-1/2 rounded-full bg-primary/10 blur-3xl" />
          <div className="absolute top-16 right-[8%] h-72 w-72 rounded-full bg-tangerine/[0.07] blur-3xl" />
        </div>
        <div className="relative mx-auto max-w-6xl px-6 pt-20 pb-24 text-center sm:pt-28">
          <Badge variant="outline" className="rounded-full bg-background/60 px-3.5 py-1 text-xs font-medium text-muted-foreground">
            Self-hosted · Single binary · MIT
          </Badge>
          <h1 className="mt-6 text-5xl font-semibold tracking-tight text-balance sm:text-6xl lg:text-7xl">
            Code together.
            <br />
            <span className="text-primary">Ship further.</span>
          </h1>
          <p className="mx-auto mt-6 max-w-2xl text-lg text-pretty text-muted-foreground">
            The self-hosted forge with stacked pull requests, code review, and CI built in. One binary, your hardware,
            your code.
          </p>
          <div className="mt-9 flex flex-wrap items-center justify-center gap-3">
            <Button asChild size="lg">
              <Link to="/register">
                Get started
                <ArrowRight className="size-4" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <Link to="/explore">Explore the demo</Link>
            </Button>
          </div>
          <ProductMockup />
        </div>
      </section>

      {/* stats / credo band */}
      <section className="border-y bg-muted/20">
        <div className="mx-auto grid max-w-6xl grid-cols-2 gap-y-10 px-6 py-14 md:grid-cols-4">
          {STATS.map((s) => (
            <div key={s.label} className="text-center">
              <p className="text-4xl font-semibold tracking-tight sm:text-5xl">{s.value}</p>
              <p className="mt-2 text-sm text-muted-foreground">{s.label}</p>
            </div>
          ))}
        </div>
      </section>

      {/* feature grid */}
      <section id="features" className="scroll-mt-16 py-20 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <SectionHeading sub="The workflows you expect from a modern forge — without the fleet of services you don't.">
            Everything a forge should be
          </SectionHeading>
          <div className="mt-14 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {FEATURES.map((f) => (
              <div key={f.title} className="rounded-xl border bg-card p-6">
                <span className="inline-flex size-10 items-center justify-center rounded-lg border bg-muted/40">
                  <f.icon className="size-5 text-muted-foreground" strokeWidth={1.75} />
                </span>
                <h3 className="mt-4 text-[15px] font-semibold tracking-tight">{f.title}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{f.copy}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* deep dive A — stacked PRs */}
      <section id="stacked" className="scroll-mt-16 border-t py-20 sm:py-24">
        <div className="mx-auto grid max-w-6xl items-center gap-14 px-6 lg:grid-cols-2 lg:gap-20">
          <div>
            <p className="text-sm font-semibold text-primary">Stacked pull requests</p>
            <h2 className="mt-2 text-3xl font-semibold tracking-tight text-balance sm:text-4xl">Stack your work.</h2>
            <p className="mt-4 leading-relaxed text-muted-foreground">
              Big features rarely fit in one reviewable pull request. GitGit lets you chain branches into a stack —
              open a PR for each layer, get each one reviewed on its own, and merge from the bottom up. When the bottom
              lands, everything above retargets automatically. No rebase gymnastics, no 3,000-line reviews.
            </p>
            <ul className="mt-7 space-y-3.5">
              <Point>Merge bottom-first — GitGit retargets the rest of the stack for you</Point>
              <Point>Every layer gets its own review, its own CI, its own merge</Point>
              <Point>The stack panel shows exactly where each change sits</Point>
            </ul>
          </div>
          <StackDiagram />
        </div>
      </section>

      {/* deep dive B — CI (mirrored) */}
      <section id="ci" className="scroll-mt-16 border-t py-20 sm:py-24">
        <div className="mx-auto grid max-w-6xl items-center gap-14 px-6 lg:grid-cols-2 lg:gap-20">
          <div className="lg:order-2">
            <p className="text-sm font-semibold text-tangerine">Continuous integration</p>
            <h2 className="mt-2 text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
              CI that lives with your code.
            </h2>
            <p className="mt-4 leading-relaxed text-muted-foreground">
              No plugin marketplace, no runner fleet, no YAML dialect with a PhD. Commit a{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.85em]">.gitgit/ci.yml</code> next to
              your code and every push gets a run — logs stream live, commits wear their status, and merging can
              require green.
            </p>
            <ul className="mt-7 space-y-3.5">
              <Point>Zero setup — jobs run right where GitGit runs</Point>
              <Point>Required checks keep red builds out of your default branch</Point>
              <Point>Live logs, one-click reruns, per-commit status</Point>
            </ul>
          </div>
          <div className="lg:order-1">
            <CITerminal />
          </div>
        </div>
      </section>

      {/* quick-start band — the developer moment */}
      <section id="open-source" className="scroll-mt-16 bg-zinc-900 py-20 sm:py-24">
        <div className="mx-auto grid max-w-6xl items-center gap-12 px-6 lg:grid-cols-2 lg:gap-20">
          <div>
            <p className="text-sm font-semibold text-primary">Open source</p>
            <h2 className="mt-2 text-3xl font-semibold tracking-tight text-balance text-zinc-50 sm:text-4xl">
              Up and running in three commands.
            </h2>
            <p className="mt-4 leading-relaxed text-zinc-400">
              GitGit is MIT-licensed and ships as one static binary with SQLite inside. No compose file, no Postgres,
              no Redis, no license server. Build it, start it, push to it.
            </p>
            <div className="mt-7 flex flex-wrap gap-2">
              {["MIT license", "Go + SQLite", "No telemetry"].map((chip) => (
                <span
                  key={chip}
                  className="rounded-full border border-zinc-700 px-3 py-1 text-xs font-medium text-zinc-300"
                >
                  {chip}
                </span>
              ))}
            </div>
          </div>
          <div className="overflow-hidden rounded-xl border border-zinc-800 bg-zinc-950 shadow-2xl">
            <div className="flex items-center gap-1.5 border-b border-zinc-800 px-4 py-2.5">
              <span className="size-2.5 rounded-full bg-zinc-700" />
              <span className="size-2.5 rounded-full bg-zinc-700" />
              <span className="size-2.5 rounded-full bg-zinc-700" />
            </div>
            <pre className="overflow-x-auto p-5 font-mono text-[13px] leading-7 sm:p-6">
              <code>
                <span className="text-zinc-500">$ </span>
                <span className="text-zinc-100">go build -o gitgit .</span>
                {"\n"}
                <span className="text-zinc-500">$ </span>
                <span className="text-zinc-100">./gitgit</span>
                {"\n"}
                <span className="text-primary">✓ </span>
                <span className="text-zinc-500">serving your forge on :3000</span>
                {"\n"}
                <span className="text-zinc-500">$ </span>
                <span className="text-zinc-100">git push origin main</span>
              </code>
            </pre>
          </div>
        </div>
      </section>

      {/* final CTA */}
      <section className="relative overflow-hidden">
        <div aria-hidden className="pointer-events-none absolute inset-0">
          <div className="absolute -bottom-40 left-1/2 h-96 w-[44rem] -translate-x-1/2 rounded-full bg-primary/[0.07] blur-3xl" />
        </div>
        <div className="relative mx-auto max-w-6xl px-6 py-24 text-center sm:py-32">
          <h2 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">Own your forge.</h2>
          <p className="mx-auto mt-4 max-w-xl text-lg text-pretty text-muted-foreground">
            Set it up on your own hardware this afternoon — and never ask permission to ship again.
          </p>
          <div className="mt-9 flex flex-wrap items-center justify-center gap-3">
            <Button asChild size="lg">
              <Link to="/register">
                Get started
                <ArrowRight className="size-4" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="ghost" className="text-muted-foreground">
              <Link to="/login">Sign in</Link>
            </Button>
          </div>
        </div>
      </section>

      {/* footer */}
      <footer className="border-t py-14">
        <div className="mx-auto grid max-w-6xl gap-10 px-6 sm:grid-cols-2 lg:grid-cols-[1.5fr_1fr_1fr_1fr]">
          <div>
            <Logo className="text-[17px]" />
            <p className="mt-3 max-w-xs text-sm leading-relaxed text-muted-foreground">
              The self-hosted forge. Code together. Ship further.
            </p>
          </div>
          <FooterCol
            title="Product"
            links={[
              { label: "Features", to: "#features" },
              { label: "Stacked PRs", to: "#stacked" },
              { label: "CI/CD", to: "#ci" },
              { label: "Explore the demo", to: "/explore" },
            ]}
          />
          <FooterCol
            title="Resources"
            links={[
              { label: "Get started", to: "/register" },
              { label: "Sign in", to: "/login" },
              { label: "Quick start", to: "#open-source" },
            ]}
          />
          <div>
            <h4 className="text-sm font-semibold">Open source</h4>
            <ul className="mt-4 space-y-2.5 text-sm text-muted-foreground">
              <li>MIT licensed</li>
              <li>One static binary</li>
              <li>Your hardware, your data</li>
            </ul>
          </div>
        </div>
        <div className="mx-auto mt-12 flex max-w-6xl flex-wrap items-center justify-between gap-2 border-t px-6 pt-6 text-xs text-muted-foreground">
          <span>© 2026 GitGit. MIT licensed.</span>
          <span className="font-mono">v1.0.0</span>
        </div>
      </footer>
    </div>
  )
}
