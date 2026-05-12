import { Link, useLocation } from 'react-router-dom'
import { MoonStar, Sparkles, SunMedium } from 'lucide-react'
import { useDarkMode } from '@/hooks/useDarkMode'

const navItems = [
  { path: '/', label: 'Search' },
  { path: '/timeline', label: 'Timeline' },
  { path: '/map', label: 'Service Map' },
  { path: '/metrics', label: 'Metrics' },
  { path: '/sampler', label: 'Sampler' },
  { path: '/compare', label: 'Compare' },
]

export function Nav() {
  const location = useLocation()
  const { dark, toggle } = useDarkMode()

  return (
    <nav className="sticky top-0 z-30 border-b border-border/70 bg-background/80 backdrop-blur-xl supports-[backdrop-filter]:bg-background/65">
      <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-3 px-4 py-3 sm:px-6 lg:px-8">
        <Link to="/" className="mr-2 flex items-center gap-3 rounded-2xl border border-border/70 bg-card/80 px-3 py-2 shadow-sm transition-colors hover:bg-card">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-[linear-gradient(135deg,_rgba(14,165,233,0.18),_rgba(15,23,42,0.02))] text-primary">
            <Sparkles className="h-5 w-5" />
          </div>
          <div>
            <div className="text-sm font-semibold tracking-tight text-foreground">Tracing Studio</div>
            <div className="text-[11px] uppercase tracking-[0.22em] text-muted-foreground">
              Distributed systems cockpit
            </div>
          </div>
        </Link>

        <div className="flex flex-1 flex-wrap items-center gap-1 rounded-2xl border border-border/70 bg-card/70 p-1 shadow-sm backdrop-blur">
          {navItems.map(item => (
            <Link
              key={item.path}
              to={item.path}
              aria-current={location.pathname === item.path ? 'page' : undefined}
              className={`rounded-xl px-3 py-2 text-sm font-medium transition-all ${
                location.pathname === item.path
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-accent/80 hover:text-foreground'
              }`}
            >
              {item.label}
            </Link>
          ))}
        </div>

        <button
          type="button"
          onClick={toggle}
          className="ml-auto inline-flex items-center gap-2 rounded-2xl border border-border/70 bg-card/80 px-3 py-2 text-sm font-medium text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-foreground"
          title={dark ? 'Switch to light mode' : 'Switch to dark mode'}
          aria-label="Toggle dark mode"
        >
          {dark ? <SunMedium className="h-4 w-4" /> : <MoonStar className="h-4 w-4" />}
          <span>{dark ? 'Light' : 'Dark'}</span>
        </button>
      </div>
    </nav>
  )
}
