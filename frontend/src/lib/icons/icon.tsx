import { Icon as IconifyIcon } from '@iconify/react'
import type { CSSProperties } from 'react'
import { useIconPack } from './context'
import type { AppIcon } from './registry'

type IconProps = {
  name: AppIcon
  size?: number
  className?: string
  strokeWidth?: number
  style?: CSSProperties
}

export function Icon({ name, size = 20, className, strokeWidth, style }: IconProps) {
  const { iconMap } = useIconPack()

  if (!iconMap) {
    return <span style={{ display: 'inline-block', width: size, height: size }} />
  }

  return (
    <IconifyIcon
      icon={iconMap[name]}
      width={size}
      height={size}
      className={className}
      style={strokeWidth ? { strokeWidth, ...style } : style}
    />
  )
}
