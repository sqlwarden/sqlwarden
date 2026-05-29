import { cn } from '#/lib/utils'
import { driverBrands } from './connection-drivers/index'

type Size = 'sm' | 'md'

const sizes: Record<Size, string> = {
  sm: 'size-[18px]',
  md: 'size-9',
}

export function DriverBadge({ driver, size = 'md', className }: { driver: string; size?: Size; className?: string }) {
  const brand = driverBrands[driver]
  if (!brand) {
    return (
      <div className={cn('flex shrink-0 items-center justify-center rounded bg-muted text-[8px] font-bold text-muted-foreground', sizes[size], className)}>
        {driver.slice(0, 2).toUpperCase()}
      </div>
    )
  }
  return (
    <img
      src={brand.icon}
      alt={driver}
      className={cn('shrink-0 object-contain', sizes[size], className)}
    />
  )
}
