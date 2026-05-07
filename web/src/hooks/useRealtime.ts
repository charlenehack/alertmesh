// useRealtime — single-purpose React hook that subscribes to the backend
// `/api/v1/realtime/ws` push channel and invokes `onEvent` whenever a
// matching event arrives.  This is the mechanism that replaced the
// 15s/20s/30s `refetchInterval` polling timers on IncidentDetail,
// IncidentList and Dashboard respectively.
//
// Design notes (see also /Users/1k-sre/.cursor/plans/realtime_push_replaces_polling_*.plan.md):
//
//   * We keep ONE WebSocket per hook invocation.  Pages typically mount
//     one hook so that's one WS per page; multi-tab fan-out is N
//     connections per user.  A SharedWorker-based singleton would cut
//     that further but isn't worth the complexity for our scale.
//
//   * Reconnection uses exponential backoff capped at 30s.  On every
//     successful (re)open we synthesise a `{type:'reconnect'}` event so
//     the caller can refetch and catch anything missed during the gap.
//
//   * onEvent is debounced 200ms inside the hook so a burst of N events
//     (e.g. an alert storm creating 30 incidents in a tick) collapses
//     into a single react-query invalidation per page — the REST layer
//     stays the source of truth; the WS is a "wake up, refresh" signal.
//
//   * The hook tolerates `topics` array identity churn from re-renders
//     by joining-and-comparing inside the effect: the WS only restarts
//     when the *content* of the topic list changes.

import { useEffect, useLayoutEffect, useRef } from 'react'
import { useAuthStore } from '../store/auth'

export interface RealtimeEvent {
  type: string
  incident_id?: string
  severity?: string
  status?: string
}

export interface UseRealtimeOptions {
  /**
   * If false, the hook is inert (no socket opened).  Useful when a
   * parent page hasn't yet resolved the id it would subscribe to.
   * Defaults to true.
   */
  enabled?: boolean
  /**
   * Debounce window in ms — events arriving within this window collapse
   * into a single onEvent call carrying the most recent payload.
   * Defaults to 200ms.
   */
  debounceMs?: number
}

const MAX_BACKOFF_MS = 30_000
const INITIAL_BACKOFF_MS = 1_000
const DEFAULT_DEBOUNCE_MS = 200

export function useRealtime(
  topics: string[],
  onEvent: (e: RealtimeEvent) => void,
  options: UseRealtimeOptions = {},
): void {
  const { enabled = true, debounceMs = DEFAULT_DEBOUNCE_MS } = options
  const token = useAuthStore((s) => s.token)

  // Stable callback ref so the effect can read the latest handler without
  // restarting on every re-render of the consumer component. The
  // useLayoutEffect ensures the ref is up-to-date before any commit-phase
  // effects (including this hook's own connect-loop) read it.
  const onEventRef = useRef(onEvent)
  useLayoutEffect(() => {
    onEventRef.current = onEvent
  }, [onEvent])

  // Joined-string key drives the effect dependency: identical topic
  // contents in a new array reference don't re-open the socket.
  const topicsKey = [...topics].sort().join(',')

  useEffect(() => {
    if (!enabled || !token || topicsKey === '') return

    let ws: WebSocket | null = null
    let backoff = INITIAL_BACKOFF_MS
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let debounceTimer: ReturnType<typeof setTimeout> | null = null
    let pendingEvent: RealtimeEvent | null = null
    let cancelled = false

    const flushPending = () => {
      debounceTimer = null
      if (pendingEvent && !cancelled) {
        const e = pendingEvent
        pendingEvent = null
        try {
          onEventRef.current(e)
        } catch (err) {
          console.error('[realtime] onEvent handler threw:', err)
        }
      }
    }

    const queueEvent = (e: RealtimeEvent) => {
      pendingEvent = e
      if (debounceTimer == null) {
        debounceTimer = setTimeout(flushPending, debounceMs)
      }
    }

    const connect = () => {
      if (cancelled) return
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url =
        `${protocol}//${window.location.host}/api/v1/realtime/ws` +
        `?token=${encodeURIComponent(token)}&topics=${encodeURIComponent(topicsKey)}`

      ws = new WebSocket(url)

      ws.onopen = () => {
        backoff = INITIAL_BACKOFF_MS
        // Synthetic reconnect event lets the caller invalidate caches
        // immediately on (re)connection — covers anything missed
        // between the disconnect and the new socket coming online.
        queueEvent({ type: 'reconnect' })
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data) as RealtimeEvent
          if (msg && typeof msg.type === 'string') queueEvent(msg)
        } catch {
          // Drop frames we can't parse — the WS is invalidation-only,
          // so a malformed payload just costs us one missed nudge.
        }
      }

      ws.onerror = () => {
        // Don't tear down here; onclose will follow and trigger the
        // backoff loop.  Logging at debug to avoid noise during normal
        // server restarts.
      }

      ws.onclose = () => {
        ws = null
        if (cancelled) return
        const delay = Math.min(backoff, MAX_BACKOFF_MS)
        backoff = Math.min(backoff * 2, MAX_BACKOFF_MS)
        reconnectTimer = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      cancelled = true
      if (reconnectTimer != null) clearTimeout(reconnectTimer)
      if (debounceTimer != null) clearTimeout(debounceTimer)
      if (ws) {
        // Setting onclose to a no-op first prevents the backoff loop
        // from racing the user navigating away.
        ws.onclose = null
        ws.close()
      }
    }
  }, [enabled, token, topicsKey, debounceMs])
}
