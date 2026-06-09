const ENTITY_COLORS = [
  'bg-violet-500/10 text-violet-600',
  'bg-blue-500/10 text-blue-600',
  'bg-emerald-500/10 text-emerald-600',
  'bg-orange-500/10 text-orange-600',
  'bg-rose-500/10 text-rose-600',
  'bg-amber-500/10 text-amber-600',
  'bg-cyan-500/10 text-cyan-600',
] as const satisfies readonly string[]

export function entityColor(name: string): string {
  if (!name) return ENTITY_COLORS[0]
  const hash = name.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
  return ENTITY_COLORS[hash % ENTITY_COLORS.length]
}

export const GROUP_COLOR = 'bg-emerald-500/10 text-emerald-600'
