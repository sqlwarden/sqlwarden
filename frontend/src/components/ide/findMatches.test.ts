import { describe, expect, it } from 'vitest'
import { EditorState, EditorSelection } from '@codemirror/state'
import { SearchQuery } from '@codemirror/search'
import { computeMatchInfo } from './findMatches'

function stateWith(doc: string, selection?: { anchor: number; head: number }) {
  return EditorState.create({
    doc,
    selection: selection ? EditorSelection.single(selection.anchor, selection.head) : undefined,
  })
}

describe('computeMatchInfo', () => {
  it('returns zero for an empty query', () => {
    const state = stateWith('foo foo foo')
    expect(computeMatchInfo(state, new SearchQuery({ search: '' }))).toEqual({ current: 0, total: 0 })
  })

  it('returns zero when the query has no matches', () => {
    const state = stateWith('foo foo foo')
    expect(computeMatchInfo(state, new SearchQuery({ search: 'bar' }))).toEqual({ current: 0, total: 0 })
  })

  it('counts all matches with no current match when nothing is selected', () => {
    const state = stateWith('foo foo foo')
    expect(computeMatchInfo(state, new SearchQuery({ search: 'foo' }))).toEqual({ current: 0, total: 3 })
  })

  it('reports the 1-based index of the selected match', () => {
    // Select the second "foo" (indices 4..7).
    const state = stateWith('foo foo foo', { anchor: 4, head: 7 })
    expect(computeMatchInfo(state, new SearchQuery({ search: 'foo' }))).toEqual({ current: 2, total: 3 })
  })

  it('honours case sensitivity', () => {
    const state = stateWith('Foo foo Foo')
    expect(computeMatchInfo(state, new SearchQuery({ search: 'Foo', caseSensitive: true }))).toEqual({
      current: 0,
      total: 2,
    })
  })

  it('supports regular expressions', () => {
    const state = stateWith('f1o f2o f3o')
    expect(computeMatchInfo(state, new SearchQuery({ search: 'f.o', regexp: true }))).toEqual({
      current: 0,
      total: 3,
    })
  })

  it('returns zero for an invalid regular expression', () => {
    const state = stateWith('foo')
    expect(computeMatchInfo(state, new SearchQuery({ search: '(', regexp: true }))).toEqual({ current: 0, total: 0 })
  })
})
