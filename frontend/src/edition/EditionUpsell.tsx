import type { EditionFeatureCapability } from '#/lib/api/types'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'

const stateMessage: Record<Exclude<EditionFeatureCapability['state'], 'available'>, string> = {
  upgrade: 'This feature is part of SQLWarden Enterprise.',
  unlicensed: 'Your Enterprise license does not include this feature.',
}

export function EditionUpsell({ feature }: { feature: EditionFeatureCapability }) {
  if (feature.state === 'available') return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{feature.label}</CardTitle>
        <CardDescription>{stateMessage[feature.state]}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-sm text-muted-foreground">
        <p>{feature.description}</p>
        {feature.docs_url ? (
          <a className="text-primary underline-offset-4 hover:underline" href={feature.docs_url}>
            Read the documentation
          </a>
        ) : null}
      </CardContent>
    </Card>
  )
}
