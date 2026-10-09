import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * Holds a revealed secret value in component state only. The value is dropped
 * on hide, on unmount, and whenever a newer request supersedes an in-flight one,
 * so it never outlives the field that displays it.
 */
export function useSecretReveal(reveal: (() => Promise<string>) | undefined) {
  const [value, setValue] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const generation = useRef(0)

  useEffect(
    () => () => {
      generation.current += 1
    },
    [],
  )

  const hide = useCallback(() => {
    generation.current += 1
    setValue(null)
    setPending(false)
  }, [])

  const show = useCallback(async () => {
    if (!reveal) return
    generation.current += 1
    const current = generation.current
    setPending(true)
    try {
      const revealed = await reveal()
      if (generation.current === current) setValue(revealed)
    } catch {
      // The caller surfaces the failure; the field simply stays hidden.
    } finally {
      if (generation.current === current) setPending(false)
    }
  }, [reveal])

  return { value, pending, show, hide }
}
