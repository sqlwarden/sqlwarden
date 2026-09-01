import { expect, it } from 'vitest'
import { renderCompletionRow, suggestionContextText } from './render'
import type { SQLCompletionSuggestion } from '#/lib/api/queries/database'

it('formats a column context as "table · type"', () => {
  const s: SQLCompletionSuggestion = {
    label: 'total',
    kind: 'column',
    qualifier: 'orders',
    data_type: 'numeric',
    replace_start: 0,
    replace_end: 0,
  }
  expect(suggestionContextText(s)).toBe('orders · numeric')
})

it('formats a table context as its schema', () => {
  expect(
    suggestionContextText({
      label: 'orders',
      kind: 'table',
      namespace: 'public',
      replace_start: 0,
      replace_end: 0,
    }),
  ).toBe('public')
})

it('formats keyword/function/type by kind when no structured context', () => {
  expect(
    suggestionContextText({ label: 'COUNT', kind: 'function', replace_start: 0, replace_end: 0 }),
  ).toBe('function')
  expect(
    suggestionContextText({ label: 'SELECT', kind: 'keyword', replace_start: 0, replace_end: 0 }),
  ).toBe('keyword')
})

it('falls back to parsing the legacy detail string for columns', () => {
  expect(
    suggestionContextText({
      label: 'id',
      kind: 'column',
      detail: 'public.orders | int8, NOT NULL',
      replace_start: 0,
      replace_end: 0,
    }),
  ).toBe('orders · int8')
})

it('renders a single-line row with an icon, label, and right-aligned context', () => {
  const node = renderCompletionRow(
    { label: 'total', type: 'column', detail: 'orders · numeric' },
    null,
  )
  expect(node.querySelector('.cm-completionKindIcon')).not.toBeNull()
  expect(node.querySelector('.cm-completionLabel')?.textContent).toBe('total')
  expect(node.querySelector('.cm-completionContext')?.textContent).toBe('orders · numeric')
})
