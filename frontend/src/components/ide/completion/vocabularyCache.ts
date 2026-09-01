import {
  getSQLCompletionVocabulary,
  type SQLCompletionSuggestion,
} from '#/lib/api/queries/database'

const vocabularyCache = new Map<string, Promise<SQLCompletionSuggestion[]>>()

export function normalizedDriver(driver?: string): string {
  switch (driver?.toLowerCase()) {
    case 'postgresql':
      return 'postgres'
    case 'mariadb':
      return 'mysql'
    case 'sqlite3':
      return 'sqlite'
    default:
      return driver?.toLowerCase() || 'standard'
  }
}

export function loadVocabulary(driver: string): Promise<SQLCompletionSuggestion[]> {
  const key = normalizedDriver(driver)
  const cached = vocabularyCache.get(key)
  if (cached) return cached
  const pending = getSQLCompletionVocabulary(key)
    .then((result) => result.suggestions)
    // Keep the failed lookup memoized as an empty vocabulary for this page.
    // Otherwise an unavailable endpoint would be retried on every identifier.
    .catch(() => [])
  vocabularyCache.set(key, pending)
  return pending
}

export function clearVocabularyCache(): void {
  vocabularyCache.clear()
}
