import { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Nav } from '@/components/Nav'
import { SearchPage } from '@/pages/Search'

const TraceDetailPage = lazy(async () => ({ default: (await import('@/pages/TraceDetail')).TraceDetailPage }))
const ServiceMapPage = lazy(async () => ({ default: (await import('@/pages/ServiceMap')).ServiceMapPage }))
const MetricsPage = lazy(async () => ({ default: (await import('@/pages/Metrics')).MetricsPage }))
const SamplerPage = lazy(async () => ({ default: (await import('@/pages/Sampler')).SamplerPage }))
const ComparePage = lazy(async () => ({ default: (await import('@/pages/Compare')).ComparePage }))
const TimelinePage = lazy(async () => ({ default: (await import('@/pages/Timeline')).TimelinePage }))

function RouteFallback() {
  return (
    <div className="mx-auto flex min-h-[40vh] max-w-5xl items-center justify-center px-6">
      <div className="text-sm text-muted-foreground">Loading view...</div>
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-background">
        <Nav />
        <main>
          <Suspense fallback={<RouteFallback />}>
            <Routes>
              <Route path="/" element={<SearchPage />} />
              <Route path="/trace/:id" element={<TraceDetailPage />} />
              <Route path="/map" element={<ServiceMapPage />} />
              <Route path="/metrics" element={<MetricsPage />} />
              <Route path="/sampler" element={<SamplerPage />} />
              <Route path="/compare" element={<ComparePage />} />
              <Route path="/timeline" element={<TimelinePage />} />
            </Routes>
          </Suspense>
        </main>
      </div>
    </BrowserRouter>
  )
}
