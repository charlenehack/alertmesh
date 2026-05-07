import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeHighlight from 'rehype-highlight'
import rehypeSlug from 'rehype-slug'
import 'highlight.js/styles/github-dark.css'

// Heading helpers (`extractHeadings`, `MarkdownHeading`) live in
// `./markdownHeadings.ts` so this file only exports a React component and
// Vite's React Fast Refresh can patch it in place.

/**
 * MarkdownView is the single render path for any LLM-produced Markdown in
 * the app (AI 分析报告、追问回复 …).  Centralising it here means:
 *
 *   - One consistent dark-theme palette across every place we surface a
 *     report — no more drift between IncidentDetail, conversation bubbles,
 *     etc.
 *   - GFM (tables, task lists, strikethrough, fenced code) is on by default
 *     because the agent prompts in `internal/ai/agent.go` lean heavily on
 *     fenced code blocks for PromQL / log snippets.
 *   - Code highlighting via rehype-highlight + the github-dark theme.
 *   - Heading anchors via rehype-slug — using the same `github-slugger`
 *     algorithm as `extractHeadings` above, so the TOC's hrefs always
 *     resolve to a real DOM id.  This is the bit that lets the sidebar
 *     scroll-to-section work reliably across re-renders.
 *
 * The component is intentionally NOT given a `style` / `className` prop on
 * the outer wrapper — let the parent decide width / padding so it composes
 * inside Cards, Drawers, chat bubbles equally well.
 */
export default function MarkdownView({
  source,
  inverted = false,
}: {
  source: string
  /**
   * `inverted` flips bullet / link / code colours for use inside the
   * purple "user message" bubbles in the AI chat — those have a coloured
   * background so the default link blue would disappear.
   */
  inverted?: boolean
}) {
  return (
    <div className={`md-view ${inverted ? 'md-view-inverted' : ''}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSlug, rehypeHighlight]}
        // Tighten antd's defaults: paragraphs render as `div` so they don't
        // pick up the global `p { margin-bottom: 1em }` reset and stack
        // double margins inside Cards.
        components={{
          p: ({ children }) => <div className="md-p">{children}</div>,
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
        }}
      >
        {source}
      </ReactMarkdown>
      <style>{markdownCss}</style>
    </div>
  )
}

// Inline so the component is fully self-contained — no global stylesheet to
// import / keep in sync.  Selectors are scoped to `.md-view` so they can't
// leak into the rest of the app.
const markdownCss = `
.md-view {
  color: #e8e8e8;
  font-size: 13.5px;
  line-height: 1.75;
  word-break: break-word;
}
.md-view .md-p { margin: 0 0 10px; }
.md-view h1, .md-view h2, .md-view h3, .md-view h4, .md-view h5, .md-view h6 {
  color: #ffffff;
  font-weight: 600;
  margin: 18px 0 8px;
  line-height: 1.4;
  /* Leave room for the AppLayout sticky header so anchor jumps land below it. */
  scroll-margin-top: 80px;
}
.md-view h1 { font-size: 18px; }
.md-view h2 {
  font-size: 16px;
  border-bottom: 1px solid #2a2a2a;
  padding-bottom: 6px;
}
.md-view h3 { font-size: 14.5px; color: #d8b4fe; }
.md-view h4 { font-size: 13.5px; color: #c4b5fd; }
.md-view ul, .md-view ol { padding-left: 22px; margin: 4px 0 10px; }
.md-view li { margin: 2px 0; }
.md-view li > .md-p { margin: 0; }
.md-view strong { color: #ffffff; font-weight: 600; }
.md-view em { color: #c8c8c8; }
.md-view a { color: #5b8def; text-decoration: none; }
.md-view a:hover { text-decoration: underline; }

/* Inline code */
.md-view code {
  background: #1a1a1a;
  border: 1px solid #2a2a2a;
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 12px;
  color: #f0a868;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
}
/* Fenced code (rehype-highlight wraps with <pre><code class="hljs language-…">) */
.md-view pre {
  background: #0d1117;
  border: 1px solid #1f2937;
  border-radius: 6px;
  padding: 12px 14px;
  margin: 8px 0 14px;
  overflow-x: auto;
}
.md-view pre code {
  background: transparent;
  border: none;
  padding: 0;
  color: #e8e8e8;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre;
}

/* Tables — common in AI reports for "metric / 24h 采样 / 结论" */
.md-view table {
  border-collapse: collapse;
  margin: 8px 0 14px;
  width: 100%;
  font-size: 12.5px;
}
.md-view th, .md-view td {
  border: 1px solid #2a2a2a;
  padding: 6px 10px;
  text-align: left;
}
.md-view th {
  background: #1a1a1a;
  color: #c8c8c8;
  font-weight: 600;
}
.md-view tr:nth-child(2n) td { background: rgba(255,255,255,0.015); }

/* Blockquotes — the agent uses them for caveats / 注意事项 */
.md-view blockquote {
  border-left: 3px solid #722ed1;
  padding: 4px 12px;
  margin: 8px 0;
  background: rgba(114, 46, 209, 0.08);
  color: #d0c4e8;
  border-radius: 0 4px 4px 0;
}
.md-view blockquote .md-p { margin: 0; }

/* Horizontal rule */
.md-view hr {
  border: none;
  border-top: 1px solid #2a2a2a;
  margin: 16px 0;
}

/* Inverted variant — for the purple user-message bubble in chat */
.md-view-inverted { color: #ffffff; }
.md-view-inverted a { color: #c4b5fd; }
.md-view-inverted code {
  background: rgba(0,0,0,0.25);
  border-color: rgba(255,255,255,0.15);
  color: #ffe8c4;
}
.md-view-inverted pre {
  background: rgba(0,0,0,0.35);
  border-color: rgba(255,255,255,0.12);
}
`
