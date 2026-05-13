import { useEffect, useState } from 'react'

const STORAGE_KEY = 'tracing-dark-mode'

function readStoredDarkMode() {
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY)
    if (saved === 'true') return true
    if (saved === 'false') return false
  } catch {
    // Ignore storage failures and fall back to the system preference.
  }
  return null
}

function readSystemDarkMode() {
  try {
    if (typeof window.matchMedia !== 'function') return false
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  } catch {
    return false
  }
}

function persistDarkMode(dark: boolean) {
  try {
    window.localStorage.setItem(STORAGE_KEY, String(dark))
  } catch {
    // Ignore storage failures; the app should continue with an in-memory preference.
  }
}

export function getInitialDarkMode() {
  const stored = readStoredDarkMode()
  if (stored !== null) return stored
  return readSystemDarkMode()
}

export function persistDarkModePreference(dark: boolean) {
  persistDarkMode(dark)
}

export function useDarkMode() {
  const [dark, setDark] = useState<boolean>(() => getInitialDarkMode())

  useEffect(() => {
    if (typeof document === 'undefined') return
    const root = document.documentElement
    root.classList.toggle('dark', dark)
    persistDarkModePreference(dark)
  }, [dark])

  return { dark, toggle: () => setDark(d => !d) }
}
