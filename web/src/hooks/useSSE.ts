import { useEffect, useRef } from 'react'

export function useSSE(url: string, onEvent: (event: unknown) => void) {
  const onEventRef = useRef(onEvent)

  useEffect(() => {
    onEventRef.current = onEvent
  }, [onEvent])

  useEffect(() => {
    let es: EventSource | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let destroyed = false

    function connect() {
      if (destroyed) return
      es = new EventSource(url)

      es.onmessage = (e: MessageEvent) => {
        try {
          onEventRef.current(JSON.parse(String(e.data)) as unknown)
        } catch {
          onEventRef.current(String(e.data))
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
