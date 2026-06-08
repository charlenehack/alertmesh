import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeHighlight from 'rehype-highlight'
import rehypeSlug from 'rehype-slug'
import 'highlight.js/styles/github.css'
import 'highlight.js/styles/github-dark.css'
import { useThemeMode } from '../theme/ThemeContext'

/**
 * MarkdownView — theme-aware Markdown renderer.
 * Switches between light / dark CSS palettes based on the app theme,
 * and supports an `inverted` mode for purple chat bubbles.
 */
export default function MarkdownView({
  source,
  inverted = false,
}: {
  source: string
  inverted?: boolean
}) {
  const { mode } = useThemeMode()
  const isDark = mode === 'dark'

  return (
    <div className={`md-view ${isDark ? 'md-view-dark' : 'md-view-light'} ${inverted ? 'md-view-inverted' : ''}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSlug, rehypeHighlight]}
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

const markdownCss = `
/* ===== shared base ===== */
.md-view {
  font-size: 13.5px;
  line-height: 1.75;
  word-break: break-word;
}
.md-view .md-p { margin: 0 0 10px; }
.md-view h1, .md-view h2, .md-view h3, .md-view h4, .md-view h5, .md-view h6 {
  font-weight: 600;
  margin: 18px 0 8px;
  line-height: 1.4;
  scroll-margin-top: 80px;
}
.md-view h1 { font-size: 18px; }
.md-view h2 { font-size: 16px; padding-bottom: 6px; }
.md-view h3 { font-size: 14.5px; }
.md-view h4 { font-size: 13.5px; }
.md-view ul, .md-view ol { padding-left: 22px; margin: 4px 0 10px; }
.md-view li { margin: 2px 0; }
.md-view li > .md-p { margin: 0; }
.md-view code {
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 12px;
  font-family: 'SF Mono', Menlo, Consolas, monospace;
}
.md-view pre {
  border-radius: 6px;
  padding: 12px 14px;
  margin: 8px 0 14px;
  overflow-x: auto;
}
.md-view pre code {
  background: transparent !important;
  border: none !important;
  padding: 0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre;
}
.md-view table {
  border-collapse: collapse;
  margin: 8px 0 14px;
  width: 100%;
  font-size: 12.5px;
}
.md-view th, .md-view td { padding: 6px 10px; text-align: left; }
.md-view th { font-weight: 600; }
.md-view blockquote {
  border-left: 3px solid #7c3aed;
  padding: 4px 12px;
  margin: 8px 0;
  border-radius: 0 4px 4px 0;
}
.md-view blockquote .md-p { margin: 0; }
.md-view hr { border: none; margin: 16px 0; }

/* ===== dark theme ===== */
.md-view-dark { color: #e8e8e8; }
.md-view-dark h1, .md-view-dark h2, .md-view-dark h3,
.md-view-dark h4, .md-view-dark h5, .md-view-dark h6 { color: #ffffff; }
.md-view-dark h2 { border-bottom: 1px solid #2a2a2a; }
.md-view-dark h3 { color: #d8b4fe; }
.md-view-dark h4 { color: #c4b5fd; }
.md-view-dark strong { color: #ffffff; font-weight: 600; }
.md-view-dark em { color: #c8c8c8; }
.md-view-dark a { color: #5b8def; text-decoration: none; }
.md-view-dark a:hover { text-decoration: underline; }
.md-view-dark code { background: #1a1a1a; border: 1px solid #2a2a2a; color: #f0a868; }
.md-view-dark pre { background: #0d1117; border: 1px solid #1f2937; }
.md-view-dark pre code { color: #e8e8e8 !important; }
.md-view-dark th, .md-view-dark td { border: 1px solid #2a2a2a; }
.md-view-dark th { background: #1a1a1a; color: #c8c8c8; }
.md-view-dark tr:nth-child(2n) td { background: rgba(255,255,255,0.015); }
.md-view-dark blockquote { background: rgba(114,46,209,0.08); color: #d0c4e8; }
.md-view-dark hr { border-top: 1px solid #2a2a2a; }
.md-view-dark .hljs { background: #0d1117 !important; color: #e8e8e8 !important; }

/* ===== light theme ===== */
.md-view-light { color: #111827; }
.md-view-light h1, .md-view-light h2, .md-view-light h3,
.md-view-light h4, .md-view-light h5, .md-view-light h6 { color: #111827; }
.md-view-light h2 { border-bottom: 1px solid #e5e7eb; }
.md-view-light h3 { color: #6d28d9; }
.md-view-light h4 { color: #7c3aed; }
.md-view-light strong { color: #111827; font-weight: 600; }
.md-view-light em { color: #374151; }
.md-view-light .md-p { color: #111827; }
.md-view-light li { color: #111827; }
.md-view-light p, .md-view-light span { color: #111827; }
.md-view-light a { color: #2563eb; text-decoration: none; }
.md-view-light a:hover { text-decoration: underline; }
.md-view-light code { background: #f3f4f6; border: 1px solid #e5e7eb; color: #d97706; }
.md-view-light pre { background: #f8f9fa; border: 1px solid #e5e7eb; }
.md-view-light pre code { color: #1f2937 !important; }
.md-view-light .hljs,
.md-view-light .hljs * { color: #24292e !important; }
.md-view-light .hljs-keyword { color: #d73a49 !important; }
.md-view-light .hljs-string { color: #032f62 !important; }
.md-view-light .hljs-comment { color: #6a737d !important; }
.md-view-light .hljs-number { color: #005cc5 !important; }
.md-view-light .hljs-attr { color: #005cc5 !important; }
.md-view-light th, .md-view-light td { border: 1px solid #e5e7eb; }
.md-view-light th { background: #f3f4f6; color: #374151; }
.md-view-light tr:nth-child(2n) td { background: rgba(0,0,0,0.02); }
.md-view-light blockquote { background: rgba(124,58,237,0.05); color: #4b5563; }
.md-view-light hr { border-top: 1px solid #e5e7eb; }
.md-view-light .hljs { background: #f8f9fa !important; }

/* ===== inverted (purple user chat bubble) ===== */
.md-view-inverted { color: #ffffff !important; }
.md-view-inverted .md-p { color: #ffffff; }
.md-view-inverted strong { color: #ffffff; }
.md-view-inverted em { color: rgba(255,255,255,0.85); }
.md-view-inverted a { color: #c4b5fd; }
.md-view-inverted code {
  background: rgba(0,0,0,0.2) !important;
  border-color: rgba(255,255,255,0.2) !important;
  color: #fde68a !important;
}
.md-view-inverted pre {
  background: rgba(0,0,0,0.25) !important;
  border-color: rgba(255,255,255,0.15) !important;
}
.md-view-inverted pre code { color: #f3f4f6 !important; }
`
