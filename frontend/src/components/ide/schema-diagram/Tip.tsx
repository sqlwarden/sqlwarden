import type { ReactElement, ReactNode } from 'react'
import { Tooltip, TooltipContent, TooltipTrigger } from '#/components/ui/tooltip'

/** Wraps a control with a design-system tooltip without adding wrapper DOM (the
 *  trigger renders as the child element). Uses the app-global TooltipProvider. */
export function Tip({
  label,
  side = 'top',
  children,
}: {
  label: ReactNode
  side?: 'top' | 'bottom' | 'left' | 'right'
  children: ReactElement
}) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent side={side}>{label}</TooltipContent>
    </Tooltip>
  )
}
