import { useEffect, useState } from 'react'
import type { MarkdownHeading } from '../../../components/markdownHeadings'

// ReportTOC — clickable outline of the AI report in the right sidebar.
//
// We use a scroll-spy that watches every heading element via a single
// IntersectionObserver and highlights whichever is currently nearest the top
// of the viewport, mirroring the experience of a docs site sidebar.
export function ReportTOC({ headings }: { headings: MarkdownHeading[] }) {
  const [activeId, setActiveId] = useState<string | null>(null)
  const minLevel = Math.min(...headings.map((h) => h.level))

  useEffect(() => {
    if (!headings.length) return
    const els = headings
      .map((h) => document.getElementById(h.id))
      .filter((el): el is HTMLElement => !!el)
    if (!els.length) return

    const obs = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible[0]) setActiveId(visible[0].target.id)
      },
      { rootMargin: '-80px 0px -65% 0px', threshold: 0 },
    )
    els.forEach((el) => obs.observe(el))
    // Seed the initial highlight so the first visible heading lights up
    // even before the user scrolls. Wrapped in a microtask to keep the
    // setState out of the synchronous effect body (react-hooks rule).
    queueMicrotask(() => setActiveId(els[0].id))
    return () => obs.disconnect()
  }, [headings])

  const onClick = (id: string) => (e: React.MouseEvent) => {
    e.preventDefault()
    const el = document.getElementById(id)
    if (!el) return
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    window.history.replaceState(null, '', `#${id}`)
    setActiveId(id)
  }

  return (
    <div style={{ maxHeight: 340, overflowY: 'auto', paddingRight: 4 }}>
      {headings.map((h) => {
        const active = h.id === activeId
        const indent = (h.level - minLevel) * 12
        return (
          <a
            key={h.id}
            href={`#${h.id}`}
            onClick={onClick(h.id)}
            title={h.text}
            style={{
              display: 'block',
              padding: '5px 8px 5px 10px',
              marginLeft: indent,
              marginBottom: 2,
              fontSize: 12.5,
              lineHeight: 1.45,
              borderLeft: `2px solid ${active ? '#722ed1' : 'transparent'}`,
              borderRadius: '0 4px 4px 0',
              color: active ? '#d8b4fe' : '#b8b8b8',
              background: active ? 'rgba(114, 46, 209, 0.08)' : 'transparent',
              fontWeight: active ? 600 : 400,
              textDecoration: 'none',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              transition: 'background 0.15s, color 0.15s',
            }}
          >
            {h.text}
          </a>
        )
      })}
    </div>
  )
}
