import { useEffect, useState } from 'react'

export function useDebouncedQueryText(initialValue = '', delayMs = 300) {
  const [searchText, setSearchText] = useState(initialValue)
  const [debouncedQuery, setDebouncedQuery] = useState(initialValue.trim())

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedQuery(searchText.trim())
    }, delayMs)

    return () => window.clearTimeout(timer)
  }, [delayMs, searchText])

  return {
    searchText,
    setSearchText,
    debouncedQuery,
    clearSearch: () => setSearchText(''),
  }
}
