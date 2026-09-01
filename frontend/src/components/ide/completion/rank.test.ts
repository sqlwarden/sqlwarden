import { expect, it } from 'vitest'
import { rankSuggestions, suggestionToCompletion } from './rank'
import type { SQLCompletionSuggestion } from '#/lib/api/queries/database'

const s = (label: string, kind: string, score = 0): SQLCompletionSuggestion => ({
  label,
  kind,
  replace_start: 0,
  replace_end: 0,
  score,
})

it('keeps prefix-tier ordering when no hint is given', () => {
  const out = rankSuggestions([s('order_by', 'keyword'), s('orders', 'table')], 'order')
  expect(out.map((o) => o.label)).toEqual(['order_by', 'orders']) // equal tier -> alpha
})

it('ranks a schema object above an exact keyword match in a relation position', () => {
  const out = rankSuggestions(
    [s('ORDER', 'keyword', 40), s('orders', 'table', 60)],
    'order',
    'relation',
  )
  expect(out[0].label).toBe('orders')
})

it('ranks columns above keywords in a column position', () => {
  const out = rankSuggestions(
    [s('OR', 'keyword', 40), s('order_ref', 'column', 70)],
    'or',
    'column',
  )
  expect(out[0].label).toBe('order_ref')
})

it('keeps object candidates that only match as a subsequence (goal 2)', () => {
  const out = rankSuggestions([s('order_items', 'table'), s('SELECT', 'keyword')], 'oi', 'any')
  expect(out.map((o) => o.label)).toContain('order_items')
})

it('drops keyword candidates that do not match the prefix', () => {
  const out = rankSuggestions([s('SELECT', 'keyword'), s('orders', 'table')], 'ord', 'any')
  expect(out.map((o) => o.label)).toEqual(['orders'])
})

it('keeps a function that matches only as a two-character subsequence', () => {
  const out = rankSuggestions([s('count', 'function'), s('SELECT', 'keyword')], 'ct', 'any')
  expect(out.map((o) => o.label)).toContain('count')
})

it('still drops a one-character non-prefix match on a non-object candidate', () => {
  const out = rankSuggestions([s('count', 'function'), s('orders', 'table')], 'x', 'any')
  expect(out.map((o) => o.label)).not.toContain('count')
})

it('emits a snippet apply for function suggestions so the caret lands inside the parens', () => {
  const completion = suggestionToCompletion({
    label: 'count',
    kind: 'function',
    replace_start: 0,
    replace_end: 0,
  })
  expect(typeof completion.apply).toBe('function')
})

it('keeps a literal string apply for non-function suggestions', () => {
  const completion = suggestionToCompletion({
    label: 'orders',
    kind: 'table',
    replace_start: 0,
    replace_end: 0,
  })
  expect(completion.apply).toBe('orders')
})

it('respects a backend-provided insert_text for a function instead of a snippet', () => {
  const completion = suggestionToCompletion({
    label: 'now',
    kind: 'function',
    insert_text: 'now()',
    replace_start: 0,
    replace_end: 0,
  })
  expect(completion.apply).toBe('now()')
})

it('is a stable deterministic sort', () => {
  const input = [s('b_tbl', 'table', 10), s('a_tbl', 'table', 10), s('c_tbl', 'table', 10)]
  expect(rankSuggestions(input, '', 'relation').map((o) => o.label)).toEqual([
    'a_tbl',
    'b_tbl',
    'c_tbl',
  ])
})
