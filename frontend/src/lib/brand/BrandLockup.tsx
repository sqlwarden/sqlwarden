import { cn } from '#/lib/utils'
import { BrandMark } from './BrandMark'

export function BrandLockup({ size = 16, className }: { size?: number; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2 font-semibold tracking-tight', className)}>
      <BrandMark size={size} />
      sqlwarden
    </span>
  )
}
