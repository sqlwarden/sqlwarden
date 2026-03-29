import { createFileRoute, Link } from '@tanstack/react-router'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Badge } from '#/components/ui/badge'

export const Route = createFileRoute('/')({ component: App })

function App() {
  return (
    <main className="container mx-auto px-4 py-12">
      <div className="mb-12 text-center">
        <Badge className="mb-4">Welcome</Badge>
        <h1 className="mb-4 text-4xl font-bold tracking-tight sm:text-5xl md:text-6xl">
          SQLWarden
        </h1>
        <p className="mx-auto mb-8 max-w-2xl text-lg text-muted-foreground">
          A powerful SQL management and monitoring tool built with modern web technologies
        </p>
        <div className="flex justify-center gap-4">
          <Link to="/about">
            <Button>Learn More</Button>
          </Link>
          <a href="https://github.com" target="_blank" rel="noopener noreferrer">
            <Button variant="outline">View on GitHub</Button>
          </a>
        </div>
      </div>

      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Database Monitoring</CardTitle>
            <CardDescription>Real-time database health and performance metrics</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Monitor your database connections, queries, and performance in real-time with comprehensive dashboards.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Query Management</CardTitle>
            <CardDescription>Execute and analyze SQL queries efficiently</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Write, test, and optimize your SQL queries with built-in syntax highlighting and auto-completion.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Security & Compliance</CardTitle>
            <CardDescription>Keep your data secure and compliant</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Advanced security features including role-based access control and audit logging.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Schema Visualization</CardTitle>
            <CardDescription>Understand your database structure</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Interactive diagrams and visual representations of your database schema and relationships.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Backup & Recovery</CardTitle>
            <CardDescription>Protect your valuable data</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Automated backup schedules and easy restoration tools to keep your data safe.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Team Collaboration</CardTitle>
            <CardDescription>Work together seamlessly</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Share queries, dashboards, and insights with your team in real-time.
            </p>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
