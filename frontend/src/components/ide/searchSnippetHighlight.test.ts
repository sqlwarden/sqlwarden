import { describe, expect, it } from 'vitest'
import { highlightSnippet } from './searchSnippetHighlight'

describe('highlightSnippet', () => {
  it('splits a single match into unmatched/matched/unmatched segments', () => {
    expect(highlightSnippet('select * from orders', 'orders')).toEqual([
      { text: 'select * from ', matched: false },
      { text: 'orders', matched: true },
    ])
  })

  it('highlights multiple occurrences', () => {
    expect(highlightSnippet('where orders.id = orders.parent_id', 'orders')).toEqual([
      { text: 'where ', matched: false },
      { text: 'orders', matched: true },
      { text: '.id = ', matched: false },
      { text: 'orders', matched: true },
      { text: '.parent_id', matched: false },
    ])
  })

  it('matches case-insensitively but preserves original casing in the output', () => {
    expect(highlightSnippet('SELECT * FROM Orders', 'orders')).toEqual([
      { text: 'SELECT * FROM ', matched: false },
      { text: 'Orders', matched: true },
    ])
  })

  it('returns the whole excerpt unmatched when the query does not appear', () => {
    expect(highlightSnippet('select * from customers', 'orders')).toEqual([
      { text: 'select * from customers', matched: false },
    ])
  })

  it('returns the whole excerpt unmatched for an empty query', () => {
    expect(highlightSnippet('select * from orders', '')).toEqual([
      { text: 'select * from orders', matched: false },
    ])
    expect(highlightSnippet('select * from orders', '   ')).toEqual([
      { text: 'select * from orders', matched: false },
    ])
  })
})
