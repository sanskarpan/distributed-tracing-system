import { Link, useLocation } from 'react-router-dom'
import { useDarkMode } from '@/hooks/useDarkMode'

const navItems = [
  { path: '/', label: 'Search' },
  { path: '/timeline', label: 'Timeline' },
  { path: '/map', label: 'Service Map' },
  { path: '/metrics', label: 'Metrics' },
  { path: '/sampler', label: 'Sampler' },
]

export function Nav() {
  const location = useLocation()
  const { dark, toggle } = useDarkMode()

  return (
    <nav className="border-b bg-background px-4 py-2 flex items-center gap-1">
      <span className="font-bold text-sm mr-4">Tracing</span>
      {navItems.map(item => (
        <Link
          key={item.path}
          to={item.path}
          className={`px-3 py-1.5 text-sm rounded-md transition-colors ${
            location.pathname === item.path
              ? 'bg-primary text-primary-foreground'
              : 'hover:bg-accent text-muted-foreground hover:text-foreground'
          }`}
        >
          {item.label}
        </Link>
      ))}
      <button
        onClick={toggle}
        className="ml-auto p-1.5 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground transition-colors text-base"
        title={dark ? 'Switch to light mode' : 'Switch to dark mode'}
        aria-label="Toggle dark mode"
      >
        {dark ? '☀' : '☾'}
      </button>
    </nav>
  )
}
