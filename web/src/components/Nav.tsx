import { Link, useLocation } from 'react-router-dom'

const navItems = [
  { path: '/', label: 'Search' },
  { path: '/map', label: 'Service Map' },
  { path: '/metrics', label: 'Metrics' },
  { path: '/sampler', label: 'Sampler' },
]

export function Nav() {
  const location = useLocation()

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
    </nav>
  )
}
