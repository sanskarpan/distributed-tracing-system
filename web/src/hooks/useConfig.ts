import { useEffect, useState } from 'react'
import { api } from '@/api/client'

interface AppConfig {
  logLinkTemplate?: string
}

let cachedConfig: AppConfig | null = null

export function useConfig(): AppConfig {
  const [config, setConfig] = useState<AppConfig>(cachedConfig ?? {})

  useEffect(() => {
    if (cachedConfig !== null) return
    const controller = new AbortController()

    api.getConfig({ signal: controller.signal }).then((c: AppConfig) => {
      cachedConfig = c
      setConfig(c)
    }).catch(() => {})

    return () => controller.abort()
  }, [])

  return config
}
