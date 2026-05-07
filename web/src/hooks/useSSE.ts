import { useEffect, useRef } from 'react'

export function useSSE(url: string, onEvent: (event: unknown) => void) {
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  useEffect(() => {
    let es: EventSource | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let destroyed = false

    function connect() {
      if (destroyed) return
      es = new EventSource(url)

      es.onmessage = (e: MessageEvent) => {
        try {
          const parsed: unknown = JSON.parse(e.data as string)
          onEventRef.current(parsed)
        } catch {
          onEventRef.current(e.data)
        }
      }

      es.onerror = () => {
        es?.close()
        es = null
        if (!destroyed) {
          reconnectTimer = setTimeout(connect, 2000)
        }
      }
    }

    connect()

    return () => {
      destroyed = true
      if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer)
      }
      es?.close()
    }
  }, [url])
}
