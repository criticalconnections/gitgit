import type { Repo } from "@/lib/api"

// splitRefPath splits a URL splat like "feature/x/src/main.go" into
// (ref, path), preferring the longest branch-name prefix — branches may
// contain slashes. Falls back to treating the first segment as the ref.
export function splitRefPath(repo: Repo, splat: string): { ref: string; path: string } {
  const trimmed = splat.replace(/^\/+|\/+$/g, "")
  if (trimmed === "") return { ref: repo.default_branch, path: "" }
  const segs = trimmed.split("/")
  const names = new Set((repo.branches ?? []).map((b) => b.name))
  for (let i = segs.length; i >= 1; i--) {
    const cand = segs.slice(0, i).join("/")
    if (names.has(cand)) {
      return { ref: cand, path: segs.slice(i).join("/") }
    }
  }
  return { ref: segs[0], path: segs.slice(1).join("/") }
}

export function joinRefPath(ref: string, path: string): string {
  return path ? `${ref}/${path}` : ref
}
