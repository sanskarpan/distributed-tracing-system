import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/api/client'
import type { CollectorReadyDTO } from '@/types'
import { getErrorMessage } from '@/lib/errors'

interface CollectorStatusState {
  status: CollectorReadyDTO | null
  error: string | null
}

export function useCollectorStatus(intervalMs = 15000): CollectorStatusState {
  const [status, setStatus] = useState<CollectorReadyDTO | null>(null)
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const requestIdRef = useRef(0)

  const refresh = useCallback(() => {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    const requestId = ++requestIdRef.current

    api.getReadyz({ signal: controller.signal, timeoutMs: 5000 })
      .then((nextStatus) => {
        if (requestId !== requestIdRef.current) return
        setStatus(nextStatus)
        setError(null)
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === 'AbortError') {
          return
        }
        if (requestId !== requestIdRef.current) return
        setError(getErrorMessage(err, 'Collector status is unavailable.'))
      })
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(refresh, 0)
    const interval = window.setInterval(refresh, intervalMs)
    return () => {
      window.clearTimeout(timer)
      window.clearInterval(interval)
      abortRef.current?.abort()
    }
  }, [intervalMs, refresh])

  return { status, error }
}
