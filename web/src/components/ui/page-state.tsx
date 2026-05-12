import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface PageStateProps {
  title: string
  description?: string
  actionLabel?: string
  onAction?: () => void
  icon?: ReactNode
}

export function PageState({ title, description, actionLabel, onAction, icon }: PageStateProps) {
  return (
    <div className="flex min-h-[40vh] flex-col items-center justify-center px-6 py-12 text-center">
      <div className="max-w-xl space-y-3">
        {icon && (
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full border bg-muted/40 text-muted-foreground">
            {icon}
          </div>
        )}
        <div className="space-y-1">
          <h1 className="text-lg font-semibold">{title}</h1>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {actionLabel && onAction && (
          <div className="pt-2">
            <Button variant="outline" onClick={onAction}>
              {actionLabel}
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
