import type { ScopePath } from './types'

export function scopeName(scope: ScopePath, kind: string): string {
  for (let index = scope.length - 1; index >= 0; index--) {
    if (scope[index].kind === kind) return scope[index].name
  }
  return ''
}

export function scopeLabel(scope: ScopePath): string {
  return scope.map((segment) => segment.name).join(' / ')
}

export function scopeKey(scope: ScopePath): string {
  return scope
    .map(({ kind, name }) => `${encodeURIComponent(kind)}=${encodeURIComponent(name)}`)
    .join('/')
}
