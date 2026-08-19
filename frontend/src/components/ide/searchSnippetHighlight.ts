/** Splits a snippet excerpt into alternating matched/unmatched segments for
 *  highlighted rendering. Matching is case-insensitive; each segment's
 *  `text` preserves the excerpt's original casing. */
export function highlightSnippet(
  excerpt: string,
  query: string,
): { text: string; matched: boolean }[] {
  const trimmedQuery = query.trim()
  if (!trimmedQuery) return [{ text: excerpt, matched: false }]

  const lowerExcerpt = excerpt.toLowerCase()
  const lowerQuery = trimmedQuery.toLowerCase()
  const segments: { text: string; matched: boolean }[] = []
  let cursor = 0

  while (cursor < excerpt.length) {
    const matchIndex = lowerExcerpt.indexOf(lowerQuery, cursor)
    if (matchIndex === -1) {
      segments.push({ text: excerpt.slice(cursor), matched: false })
      break
    }
    if (matchIndex > cursor) {
      segments.push({ text: excerpt.slice(cursor, matchIndex), matched: false })
    }
    segments.push({
      text: excerpt.slice(matchIndex, matchIndex + lowerQuery.length),
      matched: true,
    })
    cursor = matchIndex + lowerQuery.length
  }

  return segments.length > 0 ? segments : [{ text: excerpt, matched: false }]
}
