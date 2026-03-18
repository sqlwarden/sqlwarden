import { Link } from '@tanstack/react-router'
import ThemeToggle from './ThemeToggle'

export default function Header() {
  return (
    <header className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <nav className="container mx-auto flex h-16 items-center justify-between px-4">
        <div className="flex items-center gap-6">
          <Link
            to="/"
            className="flex items-center gap-2 text-lg font-semibold"
          >
            <span className="h-2 w-2 rounded-full bg-primary" />
            SQLWarden
          </Link>
          
          <div className="hidden items-center gap-6 text-sm font-medium md:flex">
            <Link
              to="/"
              className="transition-colors hover:text-foreground/80"
              activeProps={{ className: 'text-foreground' }}
              inactiveProps={{ className: 'text-foreground/60' }}
            >
              Home
            </Link>
            <Link
              to="/about"
              className="transition-colors hover:text-foreground/80"
              activeProps={{ className: 'text-foreground' }}
              inactiveProps={{ className: 'text-foreground/60' }}
            >
              About
            </Link>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <ThemeToggle />
        </div>
      </nav>
    </header>
  )
}
