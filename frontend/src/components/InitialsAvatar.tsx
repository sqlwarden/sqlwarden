import { Avatar, AvatarFallback } from '#/components/ui/avatar'

type InitialsAvatarProps = {
  value: string
  fallback?: string
  size?: 'default' | 'sm' | 'lg'
  className?: string
}

export function InitialsAvatar({ className, fallback = 'U', size, value }: InitialsAvatarProps) {
  return (
    <Avatar className={className} size={size}>
      <AvatarFallback>{getInitials(value, fallback)}</AvatarFallback>
    </Avatar>
  )
}

export function getInitials(value: string, fallback = 'U') {
  const parts = value.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return fallback
  }

  if (parts.length === 1 && parts[0].includes('@')) {
    return parts[0][0]?.toUpperCase() ?? fallback
  }

  return parts
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}
