import { cn } from '#/lib/utils'
import { findFrontendEngine } from './engines/registry'

type Size = 'sm' | 'md'

const sizes: Record<Size, string> = {
  sm: 'size-[18px]',
  md: 'size-9',
}

export function DriverBadge({
  driver,
  size = 'md',
  className,
}: {
  driver: string
  size?: Size
  className?: string
}) {
  const engine = findFrontendEngine(driver)
  if (!engine?.brand.icon) {
    return (
      <div
        className={cn(
          'flex shrink-0 items-center justify-center rounded bg-muted text-[8px] font-bold text-muted-foreground',
          sizes[size],
          className,
        )}
      >
        {engine ? engine.label.slice(0, 2).toUpperCase() : '?'}
      </div>
    )
  }
  return (
    <img
      src={engine.brand.icon}
      alt={engine.label}
      className={cn('shrink-0 object-contain', sizes[size], className)}
    />
  )
}
