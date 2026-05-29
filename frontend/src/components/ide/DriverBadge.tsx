import { cn } from '#/lib/utils'
import { driverBrands } from './connection-drivers/index'

type Size = 'sm' | 'md'

const sizes: Record<Size, string> = {
  sm: 'size-[18px] rounded-[3px] text-[8px] font-bold',
  md: 'size-9 rounded-md text-[11px] font-bold',
}

export function DriverBadge({ driver, size = 'md', className }: { driver: string; size?: Size; className?: string }) {
  const brand = driverBrands[driver] ?? { color: '#6b7280', abbr: driver.slice(0, 2).toUpperCase(), description: '' }
  return (
    <div
      className={cn('flex shrink-0 items-center justify-center text-white', sizes[size], className)}
      style={{ backgroundColor: brand.color }}
    >
      {brand.abbr}
    </div>
  )
}
