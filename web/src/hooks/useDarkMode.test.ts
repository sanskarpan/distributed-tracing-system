import { afterEach, describe, expect, it, vi } from 'vitest'
import { getInitialDarkMode, persistDarkModePreference } from './useDarkMode'

function installWindow(options: {
  stored?: string | null
  matches?: boolean
  storageThrows?: boolean
  matchMedia?: boolean
} = {}) {
  const {
    stored = null,
    matches = false,
    storageThrows = false,
    matchMedia = true,
  } = options

  const getItem = vi.fn(() => {
    if (storageThrows) throw new Error('storage blocked')
    return stored
  })
  const setItem = vi.fn(() => {
    if (storageThrows) throw new Error('storage blocked')
  })

  vi.stubGlobal('window', {
    localStorage: { getItem, setItem },
    matchMedia: matchMedia ? vi.fn(() => ({ matches })) : undefined,
  } as unknown as Window)

  return { getItem, setItem }
}

describe('dark mode helpers', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    Reflect.deleteProperty(globalThis, 'window')
  })

  it('prefers the stored preference when available', () => {
    installWindow({ stored: 'true', matches: false })

    expect(getInitialDarkMode()).toBe(true)
  })

  it('falls back to the system preference when storage is empty', () => {
    installWindow({ stored: null, matches: true })

    expect(getInitialDarkMode()).toBe(true)
  })

  it('falls back safely when browser storage is blocked', () => {
    const { setItem } = installWindow({ storageThrows: true, matchMedia: false })

    expect(getInitialDarkMode()).toBe(false)
    expect(() => persistDarkModePreference(true)).not.toThrow()
    expect(setItem).toHaveBeenCalledTimes(1)
  })
})
