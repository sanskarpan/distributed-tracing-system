import { useNavigate } from 'react-router-dom'
import { Compass, Undo2 } from 'lucide-react'
import { PageState } from '@/components/ui/page-state'

export function NotFoundPage() {
  const navigate = useNavigate()

  return (
    <div className="mx-auto max-w-4xl">
      <PageState
        title="Page not found"
        description="The route you requested does not exist in the tracing UI. Jump back to active traces or return to the investigation entry point."
        actionLabel="Back to traces"
        onAction={() => navigate('/')}
        icon="404"
      />
      <div className="mt-4 flex flex-wrap justify-center gap-3 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-card/80 px-3 py-1.5">
          <Compass className="h-3.5 w-3.5" />
          Check the URL for a stale deep link
        </span>
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-card/80 px-3 py-1.5 transition-colors hover:bg-accent"
          onClick={() => navigate(-1)}
        >
          <Undo2 className="h-3.5 w-3.5" />
          Go back one step
        </button>
      </div>
    </div>
  )
}
