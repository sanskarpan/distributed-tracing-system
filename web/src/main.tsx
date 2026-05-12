import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

const RECHARTS_LAYOUT_WARNING = 'The width(-1) and height(-1) of chart should be greater than 0'

// Recharts can emit a transient layout warning before the first measured render in headless/dev layouts.
// Suppress only that known message so genuine warnings still surface.
const originalWarn = console.warn.bind(console)
console.warn = (...args: unknown[]) => {
  if (typeof args[0] === 'string' && args[0].includes(RECHARTS_LAYOUT_WARNING)) {
    return
  }
  originalWarn(...args)
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
