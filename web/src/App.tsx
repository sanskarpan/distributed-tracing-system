import { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Activity, Radar, Sparkles } from 'lucide-react'
import { AppErrorBoundary } from '@/components/AppErrorBoundary'
import { Nav } from '@/components/Nav'
import { SearchPage } from '@/pages/Search'
import { NotFoundPage } from '@/pages/NotFound'

const TraceDetailPage = lazy(async () => ({ default: (await import('@/pages/TraceDetail')).TraceDetailPage }))
const ServiceMapPage = lazy(async () => ({ default: (await import('@/pages/ServiceMap')).ServiceMapPage }))
const MetricsPage = lazy(async () => ({ default: (await import('@/pages/Metrics')).MetricsPage }))
const SamplerPage = lazy(async () => ({ default: (await import('@/pages/Sampler')).SamplerPage }))
const ComparePage = lazy(async () => ({ default: (await import('@/pages/Compare')).ComparePage }))
const TimelinePage = lazy(async () => ({ default: (await import('@/pages/Timeline')).TimelinePage }))
const showE2ERoute = import.meta.env.VITE_E2E === 'true'

function RouteFallback() {
  return (
    <div className="mx-auto flex min-h-[48vh] max-w-6xl items-center justify-center px-4 sm:px-6">
      <div className="relative overflow-hidden rounded-[28px] border border-border/70 bg-card/90 px-8 py-10 text-center shadow-[0_24px_90px_-45px_rgba(15,23,42,0.55)] backdrop-blur">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top,_rgba(14,165,233,0.16),_transparent_60%)]" />
        <div className="relative space-y-3">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-border/80 bg-background/80 text-primary">
            <Sparkles className="h-6 w-6" />
          </div>
          <div className="text-xs font-semibold uppercase tracking-[0.24em] text-muted-foreground">Preparing View</div>
          <div className="text-sm text-muted-foreground">Loading the next investigation surface…</div>
        </div>
      </div>
    </div>
  )
}

function E2ECrashRoute(): null {
  throw new Error('Synthetic e2e route crash')
}

export default function App() {
  return (
    <AppErrorBoundary>
      <BrowserRouter>
        <div className="relative min-h-screen overflow-hidden bg-background">
          <div className="pointer-events-none absolute inset-0">
            <div className="absolute inset-x-0 top-[-18rem] h-[34rem] bg-[radial-gradient(circle_at_top,_rgba(14,165,233,0.18),_transparent_58%)]" />
            <div className="absolute right-[-10rem] top-28 h-80 w-80 rounded-full bg-[radial-gradient(circle,_rgba(56,189,248,0.12),_transparent_68%)] blur-3xl" />
            <div className="absolute left-[-8rem] top-64 h-72 w-72 rounded-full bg-[radial-gradient(circle,_rgba(244,114,182,0.10),_transparent_72%)] blur-3xl" />
            <div className="absolute inset-x-0 bottom-0 h-72 bg-[linear-gradient(to_top,_rgba(15,23,42,0.04),_transparent)] dark:bg-[linear-gradient(to_top,_rgba(2,6,23,0.28),_transparent)]" />
          </div>

          <div className="relative">
            <Nav />
            <main className="mx-auto max-w-7xl px-4 pb-12 pt-6 sm:px-6 lg:px-8">
              <div className="mb-6 flex flex-wrap items-center gap-3 text-[11px] font-medium uppercase tracking-[0.22em] text-muted-foreground">
                <span className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-card/70 px-3 py-1 backdrop-blur">
                  <Activity className="h-3.5 w-3.5" />
                  Live trace investigation
                </span>
                <span className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-card/70 px-3 py-1 backdrop-blur">
                  <Radar className="h-3.5 w-3.5" />
                  Search, compare, map, and latency views
                </span>
              </div>

              <Suspense fallback={<RouteFallback />}>
                <Routes>
                  <Route path="/" element={<SearchPage />} />
                  <Route path="/trace/:id" element={<TraceDetailPage />} />
                  <Route path="/map" element={<ServiceMapPage />} />
                  <Route path="/metrics" element={<MetricsPage />} />
                  <Route path="/sampler" element={<SamplerPage />} />
                  <Route path="/compare" element={<ComparePage />} />
                  <Route path="/timeline" element={<TimelinePage />} />
                  {showE2ERoute && <Route path="/__e2e/error-boundary" element={<E2ECrashRoute />} />}
                  <Route path="*" element={<NotFoundPage />} />
                </Routes>
              </Suspense>
            </main>
          </div>
        </div>
      </BrowserRouter>
    </AppErrorBoundary>
  )
}
