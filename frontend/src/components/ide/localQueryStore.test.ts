import 'fake-indexeddb/auto'
import { IDBFactory } from 'fake-indexeddb'
import { beforeEach, describe, expect, it } from 'vitest'
import { addLocalHistoryEntry, listLocalHistory, listLocalHistoryPage } from './localQueryStore'

beforeEach(() => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- fake-indexeddb reset between tests
  ;(globalThis as any).indexedDB = new IDBFactory()
})

describe('localQueryStore history', () => {
  it('trims history to the retention count', async () => {
    for (let i = 0; i < 5; i++) {
      await addLocalHistoryEntry(
        { connectionId: 1, sqlText: `select ${i}`, status: 'ok', durationMs: 1, rowsAffected: 1 },
        3,
      )
    }

    const entries = await listLocalHistory(1)
    expect(entries).toHaveLength(3)
  })
})

describe('localQueryStore listLocalHistoryPage search', () => {
  beforeEach(async () => {
    await addLocalHistoryEntry(
      {
        connectionId: 1,
        sqlText: 'select * from widgets',
        status: 'ok',
        durationMs: 1,
        rowsAffected: 1,
      },
      100,
    )
    await addLocalHistoryEntry(
      {
        connectionId: 1,
        sqlText: 'select * from gadgets',
        status: 'ok',
        durationMs: 1,
        rowsAffected: 1,
      },
      100,
    )
  })

  it('filters entries by a case-insensitive substring match on sqlText', async () => {
    const page = await listLocalHistoryPage({ search: 'WIDGETS', page: 1, pageSize: 25 })

    expect(page.items).toHaveLength(1)
    expect(page.items[0]?.sqlText).toBe('select * from widgets')
    expect(page.total).toBe(1)
  })

  it('returns all entries when no search term is given', async () => {
    const page = await listLocalHistoryPage({ page: 1, pageSize: 25 })

    expect(page.items).toHaveLength(2)
    expect(page.total).toBe(2)
  })

  it('returns no entries when the search term matches nothing', async () => {
    const page = await listLocalHistoryPage({ search: 'nonexistent', page: 1, pageSize: 25 })

    expect(page.items).toHaveLength(0)
    expect(page.total).toBe(0)
  })
})
