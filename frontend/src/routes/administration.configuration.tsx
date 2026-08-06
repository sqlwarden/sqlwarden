import { errorMessage } from '#/lib/api/errors'
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { instanceConfigurationQueryOptions } from '#/lib/api/query'
import { Badge } from '#/components/ui/badge'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/administration/configuration')({
  component: SettingsConfigurationPage,
  pendingComponent: RoutePending,
})

function SettingsConfigurationPage() {
  const configuration = useQuery(instanceConfigurationQueryOptions())

  useEffect(() => {
    if (!configuration.error) return
    toast.error(errorMessage(configuration.error, 'Failed to load deployment configuration'))
  }, [configuration.error])

  if (configuration.isLoading) {
    return <RoutePending />
  }

  if (configuration.isError || !configuration.data) {
    return (
      <div className="flex flex-col gap-1.5">
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Configuration</h2>
        <p className="text-muted-foreground">Failed to load deployment configuration.</p>
      </div>
    )
  }

  const config = configuration.data

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Configuration</h2>
        <p className="text-sm text-muted-foreground">
          Read-only deployment configuration sourced from config file, environment, and CLI flags.
          Change these through your deployment's configuration mechanism, not the UI.
        </p>
      </div>

      {config.deployment_managed ? (
        <Alert>
          <div className="flex items-start gap-3">
            <Badge variant={config.restart_required ? 'destructive' : 'secondary'}>
              {config.restart_required ? 'Restart required' : 'Applies immediately'}
            </Badge>
            <div>
              <AlertTitle>Deployment-managed configuration</AlertTitle>
              <AlertDescription className="mt-1">
                {config.restart_required
                  ? 'These values are set at process startup. Changing them requires editing the deployment configuration and restarting SQLWarden.'
                  : "These values are managed outside SQLWarden's admin UI."}
              </AlertDescription>
            </div>
          </div>
        </Alert>
      ) : null}

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Networking</CardTitle>
          <CardDescription>Base URL and listener configuration.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2">
            <ConfigRow label="Base URL" value={config.base_url} />
            <ConfigRow label="HTTP port" value={String(config.http_port)} />
            <ConfigRow label="TLS enabled" value={config.tls_enabled ? 'Yes' : 'No'} />
            <ConfigRow label="Deployment mode" value={config.deployment_mode} />
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Access &amp; Logging</CardTitle>
          <CardDescription>Instance-wide behavior set at process startup.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2">
            <ConfigRow label="Access mode" value={config.access_mode} />
            <ConfigRow label="Log format" value={config.log_format} />
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Storage</CardTitle>
          <CardDescription>Metadata database and file storage backends.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2">
            <ConfigRow label="Database driver" value={config.database_driver} />
            <ConfigRow
              label="Database auto-migrate"
              value={config.database_automigrate ? 'Yes' : 'No'}
            />
            <ConfigRow label="File storage mode" value={config.file_storage_mode} />
            <ConfigRow label="File storage backend" value={config.file_storage_backend} />
            <ConfigRow
              label="Local SQLite sources allowed"
              value={config.sqlite_local_enabled ? 'Yes' : 'No'}
            />
          </dl>
        </CardContent>
      </Card>
    </div>
  )
}

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-mono text-sm text-foreground">{value}</dd>
    </div>
  )
}
