import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Nav } from '@/components/Nav'
import { SearchPage } from '@/pages/Search'
import { TraceDetailPage } from '@/pages/TraceDetail'
import { ServiceMapPage } from '@/pages/ServiceMap'
import { MetricsPage } from '@/pages/Metrics'
import { SamplerPage } from '@/pages/Sampler'
import { ComparePage } from '@/pages/Compare'

export default function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-background">
        <Nav />
        <main>
          <Routes>
            <Route path="/" element={<SearchPage />} />
            <Route path="/trace/:id" element={<TraceDetailPage />} />
            <Route path="/map" element={<ServiceMapPage />} />
            <Route path="/metrics" element={<MetricsPage />} />
            <Route path="/sampler" element={<SamplerPage />} />
            <Route path="/compare" element={<ComparePage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
