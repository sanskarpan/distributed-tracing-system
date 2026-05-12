import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface ChartFrameRenderProps {
  width: number
  height: number
}

interface ChartFrameProps {
  className?: string
  children: (size: ChartFrameRenderProps) => ReactNode
}

export function ChartFrame({ className, children }: ChartFrameProps) {
  const ref = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: 0, height: 0 })

  useLayoutEffect(() => {
    const node = ref.current
    if (!node) return

    const updateSize = () => {
      const nextWidth = Math.floor(node.clientWidth)
      const nextHeight = Math.floor(node.clientHeight)
      if (nextWidth <= 0 || nextHeight <= 0) return
      setSize((prev) => {
        if (prev.width === nextWidth && prev.height === nextHeight) {
          return prev
        }
        return { width: nextWidth, height: nextHeight }
      })
    }

    updateSize()
    const observer = new ResizeObserver(() => updateSize())
    observer.observe(node)

    return () => observer.disconnect()
  }, [])

  return (
    <div ref={ref} className={cn('min-w-0', className)}>
      {size.width > 0 && size.height > 0 ? children(size) : null}
    </div>
  )
}
