import { useEffect, useState, type RefObject } from 'react'

interface ElementSize {
  width: number
  height: number
}

export function useElementSize<T extends Element>(ref: RefObject<T | null>): ElementSize {
  const [size, setSize] = useState<ElementSize>({ width: 0, height: 0 })

  useEffect(() => {
    const node = ref.current
    if (!node) return

    const updateSize = () => {
      const rect = node.getBoundingClientRect()
      setSize((prev) => {
        const width = Math.round(rect.width)
        const height = Math.round(rect.height)
        if (prev.width === width && prev.height === height) {
          return prev
        }
        return { width, height }
      })
    }

    updateSize()

    const observer = new ResizeObserver(() => updateSize())
    observer.observe(node)

    return () => observer.disconnect()
  }, [ref])

  return size
}
