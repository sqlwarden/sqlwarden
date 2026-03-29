export default function Footer() {
  const year = new Date().getFullYear()

  return (
    <footer className="border-t">
      <div className="container mx-auto px-4 py-8">
        <div className="flex flex-col items-center justify-between gap-4 text-center sm:flex-row sm:text-left">
          <p className="text-sm text-muted-foreground">
            &copy; {year} SQLWarden. All rights reserved.
          </p>
          <p className="text-sm text-muted-foreground">
            Built with Go, React, and shadcn/ui
          </p>
        </div>
      </div>
    </footer>
  )
}
