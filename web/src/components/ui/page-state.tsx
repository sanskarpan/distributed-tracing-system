import type { ReactNode } from 'react'
import { Sparkles } from 'lucide-react'
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
    <div className="flex min-h-[44vh] flex-col items-center justify-center px-4 py-10 text-center sm:px-6">
      <div className="relative max-w-2xl overflow-hidden rounded-[28px] border border-border/70 bg-card/92 px-8 py-10 shadow-[0_24px_90px_-42px_rgba(15,23,42,0.5)] backdrop-blur">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top,_rgba(14,165,233,0.12),_transparent_58%)]" />
        <div className="relative space-y-5">
          <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl border border-border/80 bg-background/85 text-primary shadow-sm">
            {icon ?? <Sparkles className="h-7 w-7" />}
          </div>

          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">{title}</h1>
            {description && <p className="mx-auto max-w-xl text-sm leading-6 text-muted-foreground sm:text-base">{description}</p>}
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
    </div>
  )
}
