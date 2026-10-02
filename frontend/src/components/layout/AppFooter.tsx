import { Link } from 'react-router-dom'

export function AppFooter() {
  return (
    <footer className="border-t md:ml-56">
      <div className="flex flex-wrap items-center justify-between gap-4 px-6 py-6 text-xs text-muted-foreground lg:px-10">
        <nav className="flex gap-4" aria-label="Documentation">
          <Link to="/terms">Terms and licensing</Link>
          <Link to="/privacy">Data flow and security</Link>
          <Link to="/guide">User guide</Link>
        </nav>
        <span>Voxis Source-Available</span>
      </div>
    </footer>
  )
}
