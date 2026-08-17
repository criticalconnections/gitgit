import { cn } from "@/lib/utils"

// The GitGit ring mark: a commit-graph circle fading orange → emerald.
export function LogoMark({ className, size = 22 }: { className?: string; size?: number }) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="gg-ring" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#F08A24" />
          <stop offset="1" stopColor="#34C98E" />
        </linearGradient>
      </defs>
      <circle cx="12" cy="12" r="9" stroke="url(#gg-ring)" strokeWidth="3" />
      <circle cx="12" cy="3" r="2.6" fill="#34C98E" />
      <circle cx="8" cy="15" r="1.8" fill="#F08A24" />
      <circle cx="15" cy="10" r="1.8" fill="#34C98E" />
    </svg>
  )
}

// Two-tone wordmark. `inverted` renders the first half white for dark bars.
export function Wordmark({ className, inverted = false }: { className?: string; inverted?: boolean }) {
  return (
    <span className={cn("font-extrabold tracking-tight", className)}>
      <span className={inverted ? "text-white" : "text-slate-ink dark:text-foreground"}>Git</span>
      <span className="text-primary">Git</span>
    </span>
  )
}

// The primary lockup: < GitGit > — brackets carry the two ends of the brand
// gradient (tangerine on the left, emerald on the right) around the wordmark.
export function Logo({ className, inverted = false }: { className?: string; inverted?: boolean }) {
  return (
    <span className={cn("inline-flex items-baseline font-extrabold tracking-tight select-none", className)}>
      <span aria-hidden="true" className="text-tangerine">
        &lt;
      </span>
      <Wordmark inverted={inverted} className="mx-[0.14em]" />
      <span aria-hidden="true" className="text-primary">
        &gt;
      </span>
    </span>
  )
}
