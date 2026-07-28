import { CompletionContext, startCompletion } from '@codemirror/autocomplete'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  acceptCompletionOnTab,
  clearSQLCompletionVocabularyCache,
  dialectForDriver,
  remoteSQLCompletionSource,
  sqlCompletionExtension,
} from './sqlCompletion'
import { MySQL, PostgreSQL, SQLite, StandardSQL } from '@codemirror/lang-sql'

afterEach(() => {
  clearSQLCompletionVocabularyCache()
  vi.unstubAllGlobals()
})

describe('SQL completion', () => {
  it('selects the matching CodeMirror dialect', () => {
    expect(dialectForDriver('postgres')).toBe(PostgreSQL)
    expect(dialectForDriver('mysql')).toBe(MySQL)
    expect(dialectForDriver('sqlite')).toBe(SQLite)
    expect(dialectForDriver()).toBe(StandardSQL)
  })

  it('maps server suggestions to their semantic kind and sends UTF-16 cursor offsets', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('completion-vocabulary')) {
        return vocabularyResponse()
      }
      expect(JSON.parse(String(init?.body))).toEqual({
        sql: 'SELECT 😀 FROM wid',
        cursor_offset: 18,
        trigger_kind: 'invoked',
      })
      return new Response(
        JSON.stringify({
          suggestions: [
            {
              label: 'widgets',
              kind: 'table',
              insert_text: 'widgets',
              replace_start: 15,
              replace_end: 18,
              score: 80,
            },
          ],
          mode: 'persistent',
          metadata_available: true,
          metadata_status: 'ready',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const state = EditorState.create({ doc: 'SELECT 😀 FROM wid' })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const result = await source(new CompletionContext(state, state.doc.length, true))
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(result?.from).toBe(15)
    expect(result?.options[0]).toMatchObject({
      label: 'widgets',
      apply: 'widgets',
      type: 'table',
      boost: 80,
    })
  })

  it('renders a semantic kind icon and accepts the highlighted item with Tab', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) =>
        String(input).includes('completion-vocabulary')
          ? vocabularyResponse()
          : new Response(
              JSON.stringify({
                suggestions: [
                  {
                    label: 'widgets',
                    kind: 'table',
                    insert_text: 'widgets',
                    replace_start: 0,
                    replace_end: 3,
                    score: 80,
                  },
                ],
                mode: 'persistent',
                metadata_available: true,
                metadata_status: 'ready',
              }),
              { status: 200, headers: { 'Content-Type': 'application/json' } },
            ),
      ),
    )
    const parent = document.createElement('div')
    document.body.append(parent)
    const view = new EditorView({
      parent,
      state: EditorState.create({
        doc: 'wid',
        selection: { anchor: 3 },
        extensions: [
          sqlCompletionExtension({
            orgSlug: 'acme',
            workspaceId: 1,
            connectionId: 2,
            driver: 'postgres',
          }),
        ],
      }),
    })

    view.focus()
    startCompletion(view)
    await vi.waitFor(() => {
      expect(parent.querySelector('.cm-completionKindIcon-table')).not.toBeNull()
      expect(parent.querySelector('[role="option"][aria-selected="true"]')).not.toBeNull()
    })
    expect(parent.querySelector('.cm-completionIcon')).toBeNull()
    // CodeMirror ignores accidental selection for a short interaction window.
    await new Promise((resolve) => setTimeout(resolve, 100))

    const event = new KeyboardEvent('keydown', {
      key: 'Tab',
      code: 'Tab',
      bubbles: true,
      cancelable: true,
    })
    Object.defineProperty(event, 'keyCode', { value: 9 })
    expect(acceptCompletionOnTab(view, event)).toBe(true)
    expect(view.state.doc.toString()).toBe('widgets')

    view.destroy()
    parent.remove()
  })

  it('falls back to local keywords when the request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => Promise.reject(new Error('offline'))),
    )
    const state = EditorState.create({ doc: 'SEL' })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const result = await source(new CompletionContext(state, 3, true))
    expect(result?.options.some((option) => option.label === 'SELECT')).toBe(true)
  })

  it('filters cached vocabulary locally without calling connection completion', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (!String(input).includes('completion-vocabulary')) {
        throw new Error(`unexpected semantic request: ${String(input)}`)
      }
      return vocabularyResponse()
    })
    vi.stubGlobal('fetch', fetchMock)
    const state = EditorState.create({ doc: 'SE' })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const result = await source(new CompletionContext(state, 2, false))
    expect(result?.options.map((option) => option.label)).toEqual(['SELECT'])
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it('calls semantic completion on a dot and preserves display labels', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).not.toContain('completion-vocabulary')
      expect(JSON.parse(String(init?.body))).toMatchObject({
        sql: 'SELECT * FROM inventory i JOIN store s WHERE s.',
        trigger_kind: 'automatic',
        trigger_character: '.',
      })
      return new Response(
        JSON.stringify({
          suggestions: [
            {
              label: 'id',
              display_label: 'id (2)',
              kind: 'column',
              insert_text: 'id',
              replace_start: 47,
              replace_end: 47,
              score: 100,
            },
          ],
          mode: 'persistent',
          metadata_available: true,
          metadata_status: 'ready',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const sql = 'SELECT * FROM inventory i JOIN store s WHERE s.'
    const state = EditorState.create({ doc: sql })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const result = await source(new CompletionContext(state, sql.length, false))
    expect(result?.options[0]).toMatchObject({ label: 'id', displayLabel: 'id (2)', apply: 'id' })
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it.each([
    {
      driver: 'mysql',
      finalSQL: 'select * from film f\njoin film_actor fa\nwhere f.`description` = fa.',
    },
    {
      driver: 'postgres',
      finalSQL: 'select * from film f\njoin film_actor fa\nwhere f."description" = fa.',
    },
  ])(
    'discards an earlier $driver alias response when a later qualified alias is requested',
    async ({ driver, finalSQL }) => {
      let firstRequestAborted = false
      const firstSQL = 'select * from film f\njoin film_actor fa\nwhere f.'
      const fetchMock = vi.fn(
        async (_input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
          const body = JSON.parse(String(init?.body)) as { sql: string }
          if (body.sql === firstSQL) {
            return new Promise<Response>((_resolve, reject) => {
              init?.signal?.addEventListener('abort', () => {
                firstRequestAborted = true
                reject(new DOMException('Aborted', 'AbortError'))
              })
            })
          }
          expect(body.sql).toBe(finalSQL)
          return semanticCompletionResponse([
            {
              label: 'actor_id',
              kind: 'column',
              insert_text: 'actor_id',
              replace_start: finalSQL.length,
              replace_end: finalSQL.length,
              score: 100,
            },
          ])
        },
      )
      vi.stubGlobal('fetch', fetchMock)
      const source = remoteSQLCompletionSource({
        orgSlug: 'acme',
        workspaceId: 1,
        connectionId: 2,
        driver,
      })

      const firstState = EditorState.create({ doc: firstSQL })
      const firstResult = source(new CompletionContext(firstState, firstSQL.length, false))
      await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))

      const finalState = EditorState.create({ doc: finalSQL })
      const finalResult = await source(new CompletionContext(finalState, finalSQL.length, false))

      expect(firstRequestAborted).toBe(true)
      expect(await firstResult).toBeNull()
      expect(finalResult?.options.map((option) => option.label)).toEqual(['actor_id'])
      expect(fetchMock).toHaveBeenCalledTimes(2)
    },
  )

  it('loads one vocabulary promise for concurrent editors', async () => {
    const fetchMock = vi.fn(async () => vocabularyResponse())
    vi.stubGlobal('fetch', fetchMock)
    const source = remoteSQLCompletionSource({ driver: 'postgres' })
    const first = EditorState.create({ doc: 'SE' })
    const second = EditorState.create({ doc: 'CO' })
    await Promise.all([
      source(new CompletionContext(first, 2, false)),
      source(new CompletionContext(second, 2, false)),
    ])
    expect(fetchMock).toHaveBeenCalledOnce()
  })
})

function vocabularyResponse() {
  return new Response(
    JSON.stringify({
      dialect: 'postgres',
      version: 'test-version',
      suggestions: [
        { label: 'SELECT', kind: 'keyword', score: 40 },
        { label: 'COUNT', kind: 'function', score: 60 },
      ],
    }),
    { status: 200, headers: { 'Content-Type': 'application/json' } },
  )
}

function semanticCompletionResponse(
  suggestions: Array<{
    label: string
    kind: string
    insert_text: string
    replace_start: number
    replace_end: number
    score: number
  }>,
) {
  return new Response(
    JSON.stringify({
      suggestions,
      mode: 'persistent',
      metadata_available: true,
      metadata_status: 'ready',
    }),
    { status: 200, headers: { 'Content-Type': 'application/json' } },
  )
}
