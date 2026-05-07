import { create } from 'zustand'
import type { SamplerConfig, TraceSummaryDTO } from '@/types'

const LIVE_TRACE_LIMIT = 500

interface TracingStore {
  services: string[]
  setServices: (s: string[]) => void
  activeSampler: SamplerConfig | null
  setActiveSampler: (s: SamplerConfig | null) => void
  liveTraces: TraceSummaryDTO[]
  addLiveTrace: (t: TraceSummaryDTO) => void
}

export const useTracingStore = create<TracingStore>((set) => ({
  services: [],
  setServices: (s) => set({ services: s }),
  activeSampler: null,
  setActiveSampler: (s) => set({ activeSampler: s }),
  liveTraces: [],
  addLiveTrace: (t) =>
    set((state) => {
      // Deduplicate by traceId and keep last 500
      const filtered = state.liveTraces.filter((lt) => lt.traceId !== t.traceId)
      const next = [t, ...filtered]
      return { liveTraces: next.slice(0, LIVE_TRACE_LIMIT) }
    }),
}))
