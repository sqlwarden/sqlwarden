import { Blobatar } from '@blobatar/react'

import { cn } from '#/lib/utils'

const SIZE_PX = {
  sm: 24,
  default: 32,
  lg: 40,
} as const

type UserAvatarSize = keyof typeof SIZE_PX

type UserAvatarProps = {
  value: string
  fallback?: string
  size?: UserAvatarSize | number
  className?: string
}

/**
 * Deterministic per-identity avatar (blobatar.dev). The same `value` always
 * renders the same blobatar, so `value` should be a stable identity string —
 * a name, email, or handle — not a display label that can change. Rendered
 * transparent, with no backdrop shape, so the blob character stands alone.
 */
export function UserAvatar({
  className,
  fallback = 'U',
  size = 'default',
  value,
}: UserAvatarProps) {
  const seed = value.trim() || fallback
  const px = typeof size === 'number' ? size : SIZE_PX[size]

  return (
    <Blobatar
      name={seed}
      size={px}
      background={false}
      title={value.trim() || fallback}
      className={cn('shrink-0', className)}
    />
  )
}
