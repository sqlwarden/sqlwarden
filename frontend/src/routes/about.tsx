import { createFileRoute } from '@tanstack/react-router'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Badge } from '#/components/ui/badge'
import { Separator } from '#/components/ui/separator'

export const Route = createFileRoute('/about')({
  component: About,
})

function About() {
  return (
    <main className="container mx-auto px-4 py-12">
      <div className="mx-auto max-w-3xl">
        <Badge className="mb-4">About</Badge>
        <h1 className="mb-4 text-4xl font-bold tracking-tight sm:text-5xl">
          About SQLWarden
        </h1>
        <p className="mb-8 text-lg text-muted-foreground">
          SQLWarden is a modern database management platform designed to help developers and teams work with databases more efficiently.
        </p>

        <Separator className="my-8" />

        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Our Mission</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground">
              We believe that database management should be simple, secure, and accessible to everyone. 
              SQLWarden provides the tools you need to monitor, query, and manage your databases without the complexity 
              of traditional database management systems.
            </p>
          </CardContent>
        </Card>

        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Technology Stack</CardTitle>
            <CardDescription>Built with modern, reliable technologies</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              <Badge variant="secondary">Go</Badge>
              <Badge variant="secondary">React</Badge>
              <Badge variant="secondary">TypeScript</Badge>
              <Badge variant="secondary">TanStack Router</Badge>
              <Badge variant="secondary">shadcn/ui</Badge>
              <Badge variant="secondary">Tailwind CSS</Badge>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Get Started</CardTitle>
            <CardDescription>Ready to try SQLWarden?</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground">
              SQLWarden is open source and actively maintained. Check out our documentation to get started, 
              or contribute to the project on GitHub.
            </p>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
