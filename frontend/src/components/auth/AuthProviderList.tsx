import { authProviders } from '#/lib/auth-providers/registry'
import { Button } from '#/components/ui/button'
import { Separator } from '#/components/ui/separator'

export function AuthProviderList({ redirect }: { redirect?: string }) {
  if (authProviders.length === 0) {
    return null
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Separator className="flex-1" />
        <span className="text-xs text-muted-foreground">or continue with</span>
        <Separator className="flex-1" />
      </div>
      <div className="grid gap-2">
        {authProviders.map((provider) => (
          <Button
            key={provider.id}
            type="button"
            variant="outline"
            className="h-9 w-full"
            onClick={() => provider.startSignIn(redirect)}
          >
            <provider.Icon className="size-4" />
            {provider.label}
          </Button>
        ))}
      </div>
    </div>
  )
}
