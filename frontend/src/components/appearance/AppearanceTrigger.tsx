import { useState } from 'react'
import { Button } from '#/components/ui/button'
import { Icon } from '#/lib/icons'
import { Tip } from '#/components/ide/schema-diagram/Tip'
import { cn } from '#/lib/utils'
import { AppearanceDialog } from './AppearanceDialog'

export function AppearanceTrigger({
  buttonLabel,
  buttonClassName,
  iconClassName,
}: {
  buttonLabel?: string
  buttonClassName?: string
  iconClassName?: string
}) {
  const [open, setOpen] = useState(false)

  const trigger = (
    <Button
      type="button"
      variant="ghost"
      aria-label={buttonLabel || 'Appearance'}
      aria-pressed={open}
      onClick={() => setOpen(true)}
      className={cn(
        'w-full justify-start gap-2 group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0',
        buttonClassName,
      )}
    >
      <Icon name="paint-board" size={20} className={cn('shrink-0', iconClassName)} />
      {buttonLabel ? (
        <span className="group-data-[collapsible=icon]:hidden">{buttonLabel}</span>
      ) : null}
    </Button>
  )

  return (
    <>
      {buttonLabel ? trigger : <Tip label="Appearance">{trigger}</Tip>}
      <AppearanceDialog open={open} onOpenChange={setOpen} />
    </>
  )
}
