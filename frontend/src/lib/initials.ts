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
