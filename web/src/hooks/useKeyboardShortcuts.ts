import { useEffect } from 'react'

export interface KeyboardShortcut {
  key: string
  metaKey?: boolean
  ctrlKey?: boolean
  shiftKey?: boolean
  description: string
  handler: () => void
}

export function useKeyboardShortcuts(shortcuts: KeyboardShortcut[]) {
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      // Skip when typing in an input/textarea
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement ||
        (e.target as HTMLElement)?.isContentEditable
      ) {
        return
      }
      for (const sc of shortcuts) {
        const keyMatch = e.key.toLowerCase() === sc.key.toLowerCase()
        const metaMatch = !sc.metaKey || e.metaKey
        const ctrlMatch = !sc.ctrlKey || e.ctrlKey
        const shiftMatch = !sc.shiftKey || e.shiftKey
        if (keyMatch && metaMatch && ctrlMatch && shiftMatch) {
          e.preventDefault()
          sc.handler()
          break
        }
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [shortcuts])
}
