import { useEdition } from '#/hooks/use-edition'
import { Badge } from '#/components/ui/badge'

interface EnterpriseUpsellProps {
  title: string
  description: string
}

// The locked/upsell surface for enterprise features. Community servers get
// the marketing message; unlicensed enterprise servers get the apply-key
// prompt. This is core (AGPL) on purpose: community users are exactly who
// needs to see it.
export function EnterpriseUpsell({ title, description }: EnterpriseUpsellProps) {
  const edition = useEdition()
  const locked = edition.data?.edition === 'enterprise'

  return (
    <div className="mx-auto w-full max-w-lg p-6">
      <div className="flex items-center gap-2">
        <h1 className="text-lg font-semibold">{title}</h1>
        <Badge variant="secondary">Enterprise</Badge>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      <p className="mt-4 text-sm text-muted-foreground">
        {locked
          ? 'This server runs SQLWarden Enterprise without an active license. Apply a license key to enable this feature.'
          : 'This feature is part of SQLWarden Enterprise.'}
      </p>
    </div>
  )
}
