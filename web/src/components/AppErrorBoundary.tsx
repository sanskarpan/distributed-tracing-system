import { Component, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle, Home, RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  errorMessage: string
}

export class AppErrorBoundary extends Component<Props, State> {
  state: State = {
    hasError: false,
    errorMessage: '',
  }

  static getDerivedStateFromError(error: Error): State {
    return {
      hasError: true,
      errorMessage: error.message || 'Unknown UI failure',
    }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Unhandled UI error', error, errorInfo)
  }

  private handleReload = () => {
    window.location.reload()
  }

  private handleReturnHome = () => {
    window.location.assign('/')
  }

  render() {
    if (!this.state.hasError) {
      return this.props.children
    }

    return (
      <div
        data-testid="app-error-boundary"
        className="mx-auto flex min-h-[70vh] w-full max-w-3xl items-center px-4 py-8 sm:px-6"
      >
        <div className="relative w-full overflow-hidden rounded-[32px] border border-border/70 bg-card/95 p-8 shadow-[0_30px_120px_-40px_rgba(15,23,42,0.45)] backdrop-blur">
          <div className="absolute inset-x-0 top-0 h-40 bg-[radial-gradient(circle_at_top,_rgba(14,165,233,0.18),_transparent_60%)]" />
          <div className="relative space-y-6">
            <div className="flex h-16 w-16 items-center justify-center rounded-2xl border border-destructive/30 bg-destructive/10 text-destructive shadow-sm">
              <AlertTriangle className="h-7 w-7" />
            </div>

            <div className="space-y-3">
              <p className="text-xs font-semibold uppercase tracking-[0.28em] text-muted-foreground">
                Application Error
              </p>
              <h1 className="max-w-2xl text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
                The UI hit an unexpected error before it could finish rendering.
              </h1>
              <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
                The collector data is still intact, but this screen failed inside the browser. Reload the app to retry
                the render, or go back to the traces index and continue from a stable route.
              </p>
            </div>

            <div className="rounded-2xl border border-border/70 bg-background/80 p-4">
              <div className="text-xs font-semibold uppercase tracking-[0.24em] text-muted-foreground">
                Error Message
              </div>
              <div className="mt-2 font-mono text-sm text-foreground">
                {this.state.errorMessage}
              </div>
            </div>

            <div className="flex flex-wrap gap-3">
              <Button onClick={this.handleReload}>
                <RotateCcw className="mr-2 h-4 w-4" />
                Reload app
              </Button>
              <Button variant="outline" onClick={this.handleReturnHome}>
                <Home className="mr-2 h-4 w-4" />
                Back to traces
              </Button>
            </div>
          </div>
        </div>
      </div>
    )
  }
}
