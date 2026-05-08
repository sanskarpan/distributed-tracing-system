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
    api.getConfig().then((c: AppConfig) => {
      cachedConfig = c
      setConfig(c)
    }).catch(() => {})
  }, [])

  return config
}
