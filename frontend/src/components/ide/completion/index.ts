import { clearCompletionIndexCache } from './schemaIndex'
import { clearVocabularyCache } from './vocabularyCache'

export * from './source'
export { automaticSQLCompletionTrigger } from './context'
export { invalidateCompletionIndex } from './schemaIndex'

export function clearSQLCompletionCaches(): void {
  clearVocabularyCache()
  clearCompletionIndexCache()
}
