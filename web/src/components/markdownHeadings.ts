// Heading slug helpers — kept in a dedicated `.ts` file (not co-located with
// `MarkdownView.tsx`) so React Fast Refresh can patch the component in place
// without a full reload.  Vite's react-refresh boundary requires that any
// module which exports a React component export *only* component-shaped
// values; mixing in a regular function like `extractHeadings` triggers
// "Could not Fast Refresh ... export is incompatible" and forces an HMR
// invalidate.  See:
//   https://github.com/vitejs/vite-plugin-react/tree/main/packages/plugin-react#consistent-components-exports
//
// Both this module and `MarkdownView.tsx` route through the same
// `github-slugger` library so the IDs we precompute for the TOC match
// byte-for-byte the ones `rehype-slug` stamps on the rendered headings,
// even for duplicate titles (it appends `-1`, `-2`, … the same way).

import GithubSlugger from 'github-slugger'

export interface MarkdownHeading {
  level: number       // 1..6
  text: string        // visible label (markdown emphasis stripped)
  id: string          // anchor id (matches the heading element id rehype-slug stamps)
}

/**
 * Walk the markdown source and extract every ATX heading (`#…######`).
 * Skips headings that appear inside fenced code blocks because those are
 * literal content, not section titles.
 *
 * `github-slugger` is stateful per instance — it tracks previously emitted
 * slugs and adds `-1`, `-2`, … suffixes for duplicates, mirroring what
 * `rehype-slug` does on the rendered AST.  We instantiate a fresh slugger
 * per call so concurrent reports never share state.
 */
export function extractHeadings(source: string): MarkdownHeading[] {
  const lines = source.split(/\r?\n/)
  const slugger = new GithubSlugger()
  const out: MarkdownHeading[] = []
  let inFence = false

  for (const raw of lines) {
    if (/^\s*```/.test(raw)) { inFence = !inFence; continue }
    if (inFence) continue

    const m = /^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/.exec(raw)
    if (!m) continue

    const level = m[1].length
    // Strip inline markdown that rehype-slug also strips before slugging:
    // emphasis / code / link wrappers.
    const text = m[2]
      .replace(/`([^`]+)`/g, '$1')
      .replace(/\*\*([^*]+)\*\*/g, '$1')
      .replace(/\*([^*]+)\*/g, '$1')
      .replace(/__([^_]+)__/g, '$1')
      .replace(/_([^_]+)_/g, '$1')
      .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
      .trim()
    if (!text) continue

    out.push({ level, text, id: slugger.slug(text) })
  }
  return out
}
