import {
  CompletionContext,
  completionStatus,
  selectedCompletionIndex,
  startCompletion,
} from '@codemirror/autocomplete'
import { defaultKeymap } from '@codemirror/commands'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  acceptCompletionOnTab,
  automaticSQLCompletionTrigger,
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
      boost: 4080,
    })
  })

  it('renders a semantic kind icon and accepts the first soft item with Tab', async () => {
    stubSingleCompletionFetch()
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
      expect(parent.querySelector('[role="option"]')).not.toBeNull()
    })
    expect(parent.querySelector('[role="option"][aria-selected="true"]')).toBeNull()
    expect(selectedCompletionIndex(view.state)).toBeNull()
    expect(parent.querySelector('.cm-completionIcon')).toBeNull()

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

  it('uses Enter for a newline until the user focuses a completion with arrows', async () => {
    stubSingleCompletionFetch()
    const first = createCompletionEditor('wid', [keymap.of(defaultKeymap)])

    startCompletion(first.view)
    await waitForCompletion(first.parent)
    expect(selectedCompletionIndex(first.view.state)).toBeNull()

    const softEnter = dispatchEditorKey(first.view, 'Enter', 13)
    expect(softEnter.defaultPrevented).toBe(true)
    expect(first.view.state.doc.toString()).toBe('wid\n')
    first.destroy()

    stubSingleCompletionFetch()
    const second = createCompletionEditor('wid', [keymap.of(defaultKeymap)])
    startCompletion(second.view)
    await waitForCompletion(second.parent)

    dispatchEditorKey(second.view, 'ArrowDown', 40)
    expect(selectedCompletionIndex(second.view.state)).toBe(0)
    dispatchEditorKey(second.view, 'Enter', 13)
    expect(second.view.state.doc.toString()).toBe('widgets')
    second.destroy()
  })

  it('closes completion with Escape without moving focus out of the editor', async () => {
    stubSingleCompletionFetch()
    const outside = document.createElement('button')
    document.body.append(outside)
    const rendered = createCompletionEditor('wid')
    const bubbled = vi.fn()
    document.addEventListener('keydown', bubbled)

    rendered.view.focus()
    startCompletion(rendered.view)
    await waitForCompletion(rendered.parent)
    expect(rendered.view.hasFocus).toBe(true)

    const escape = dispatchEditorKey(rendered.view, 'Escape', 27)
    expect(escape.defaultPrevented).toBe(true)
    expect(completionStatus(rendered.view.state)).toBeNull()
    expect(rendered.view.hasFocus).toBe(true)
    expect(document.activeElement).toBe(rendered.view.contentDOM)
    expect(bubbled).not.toHaveBeenCalled()

    document.removeEventListener('keydown', bubbled)
    rendered.destroy()
    outside.remove()
  })

  it('keeps Ctrl+Space as an explicit completion shortcut', async () => {
    const fetchMock = stubSingleCompletionFetch()
    const rendered = createCompletionEditor('wid')

    const shortcut = dispatchEditorKey(rendered.view, ' ', 32, {
      code: 'Space',
      ctrlKey: true,
    })
    await waitForCompletion(rendered.parent)

    expect(shortcut.defaultPrevented).toBe(true)
    expect(fetchMock).toHaveBeenCalled()
    expect(selectedCompletionIndex(rendered.view.state)).toBeNull()
    rendered.destroy()
  })

  it('indents with Tab when no completion is active instead of moving browser focus', () => {
    const parent = document.createElement('div')
    document.body.append(parent)
    const view = new EditorView({
      parent,
      state: EditorState.create({
        doc: 'SELECT 1',
        selection: { anchor: 8 },
        extensions: [sqlCompletionExtension({})],
      }),
    })
    const event = new KeyboardEvent('keydown', {
      key: 'Tab',
      code: 'Tab',
      bubbles: true,
      cancelable: true,
    })
    Object.defineProperty(event, 'keyCode', { value: 9 })

    view.contentDOM.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    expect(view.state.doc.toString()).toMatch(/^\s+SELECT 1$/)

    view.destroy()
    parent.remove()
  })

  it('shows connection guidance and local SQL keywords on explicit completion without a connection', async () => {
    const onConnectionRequired = vi.fn()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const parent = document.createElement('div')
    document.body.append(parent)
    const view = new EditorView({
      parent,
      state: EditorState.create({
        doc: 'SEL',
        selection: { anchor: 3 },
        extensions: [sqlCompletionExtension({ onConnectionRequired })],
      }),
    })
    view.focus()
    startCompletion(view)
    await vi.waitFor(() => {
      expect(onConnectionRequired).toHaveBeenCalledOnce()
      const labels = [...parent.querySelectorAll('.cm-completionLabel')].map(
        (element) => element.textContent,
      )
      expect(labels).toContain('SELECT')
    })

    expect(fetchMock).not.toHaveBeenCalled()
    view.destroy()
    parent.remove()
  })

  it('preserves MySQL server insertion text for safe and quoted identifiers', async () => {
    const insertions = [
      { label: 'item', insert_text: 'item' },
      { label: 'order', insert_text: '`order`' },
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) =>
        String(input).includes('completion-vocabulary')
          ? vocabularyResponse()
          : semanticCompletionResponse(
              insertions.map(({ label, insert_text }) => ({
                label,
                kind: 'column',
                insert_text,
                replace_start: 7,
                replace_end: 7,
                score: 100,
              })),
            ),
      ),
    )
    const state = EditorState.create({ doc: 'SELECT ' })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'mysql',
    })

    const result = await source(new CompletionContext(state, state.doc.length, true))

    expect(result?.options.find((option) => option.label === 'item')?.apply).toBe('item')
    expect(result?.options.find((option) => option.label === 'order')?.apply).toBe('`order`')
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

  it('ranks exact and prefix vocabulary matches ahead of broader matches', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              dialect: 'postgres',
              version: 'ranking-test',
              suggestions: [
                { label: 'consumer', kind: 'function', score: 100 },
                { label: 'set_user_mapping', kind: 'function', score: 100 },
                { label: 'summary', kind: 'function', score: 1 },
                { label: 'gross_sum', kind: 'function', score: 100 },
                { label: 'SUM', kind: 'function', score: 1 },
              ],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
      ),
    )
    const state = EditorState.create({ doc: 'sum' })
    const source = remoteSQLCompletionSource({ driver: 'postgres' })

    const result = await source(new CompletionContext(state, state.doc.length, false))

    expect(result?.options.map((option) => option.label)).toEqual([
      'SUM',
      'summary',
      'gross_sum',
      'consumer',
      'set_user_mapping',
    ])
  })

  it.each(['postgres', 'mysql'])(
    'invalidates a broad %s semantic result and refreshes it for the typed prefix',
    async (driver) => {
      const broadSQL = 'SELECT amount FROM payment HAVING '
      const prefixSQL = `${broadSQL}su`
      const exactSQL = `${broadSQL}sum`
      const semanticRequests: string[] = []
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes('completion-vocabulary')) {
          return new Response(
            JSON.stringify({
              dialect: driver,
              version: 'function-prefix-test',
              suggestions: [{ label: 'SUM', kind: 'function', score: 60 }],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          )
        }
        const body = JSON.parse(String(init?.body)) as { sql: string }
        semanticRequests.push(body.sql)
        return semanticCompletionResponse(
          body.sql === broadSQL
            ? [
                {
                  label: 'binary_upgrade_set_next_pg_enum_oid',
                  kind: 'function',
                  insert_text: 'binary_upgrade_set_next_pg_enum_oid',
                  replace_start: broadSQL.length,
                  replace_end: broadSQL.length,
                  score: 100,
                },
              ]
            : body.sql === prefixSQL
              ? [
                  {
                    label: 'array_append_support',
                    kind: 'function',
                    insert_text: 'array_append_support',
                    replace_start: broadSQL.length,
                    replace_end: prefixSQL.length,
                    score: 50,
                  },
                ]
              : [
                  {
                    label: 'binary_upgrade_set_next_pg_enum_oid',
                    kind: 'function',
                    insert_text: 'binary_upgrade_set_next_pg_enum_oid',
                    replace_start: broadSQL.length,
                    replace_end: exactSQL.length,
                    score: 100,
                  },
                ],
        )
      })
      vi.stubGlobal('fetch', fetchMock)
      const source = remoteSQLCompletionSource({
        orgSlug: 'acme',
        workspaceId: 1,
        connectionId: 2,
        driver,
      })

      const broadState = EditorState.create({ doc: broadSQL })
      const broadResult = await source(
        new CompletionContext(broadState, broadState.doc.length, false),
      )
      expect(typeof broadResult?.validFor).toBe('function')
      if (typeof broadResult?.validFor !== 'function') throw new Error('expected validFor function')
      expect(
        broadResult.validFor('', broadResult.from, broadResult.to ?? broadResult.from, broadState),
      ).toBe(true)
      expect(
        broadResult.validFor(
          'su',
          broadResult.from,
          broadResult.to ?? broadResult.from,
          broadState,
        ),
      ).toBe(false)

      const prefixState = EditorState.create({ doc: prefixSQL })
      const prefixResult = await source(
        new CompletionContext(prefixState, prefixState.doc.length, false),
      )

      expect(prefixResult?.options.map((option) => option.label)).toEqual([
        'SUM',
        'array_append_support',
      ])
      expect(typeof prefixResult?.validFor).toBe('function')
      if (typeof prefixResult?.validFor !== 'function')
        throw new Error('expected validFor function')
      expect(
        prefixResult.validFor(
          'sum',
          prefixResult.from,
          prefixResult.to ?? prefixResult.from,
          prefixState,
        ),
      ).toBe(false)

      const exactState = EditorState.create({ doc: exactSQL })
      const exactResult = await source(
        new CompletionContext(exactState, exactState.doc.length, false),
      )
      expect(exactResult?.options[0]?.label).toBe('SUM')
      expect(exactResult?.options.map((option) => option.label)).toContain(
        'binary_upgrade_set_next_pg_enum_oid',
      )
      expect(semanticRequests).toEqual([broadSQL, prefixSQL, exactSQL])
    },
  )

  it.each([
    ['SELECT | FROM users', 'email', 'column'],
    ['SELECT * FROM |', 'users', 'table'],
    ['SELECT * FROM users JOIN |', 'orders', 'table'],
    ['SELECT * FROM users WHERE |', 'email', 'column'],
    ['INSERT INTO |', 'users', 'table'],
    ['INSERT INTO users (|)', 'email', 'column'],
    ['INSERT INTO users (email) VALUES (|)', 'COALESCE', 'function'],
    ['UPDATE | SET email = NULL', 'users', 'table'],
    ['UPDATE users SET |', 'email', 'column'],
    ['DELETE FROM |', 'users', 'table'],
    ['ALTER TABLE |', 'users', 'table'],
    ['ALTER TABLE users |', 'ADD', 'keyword'],
    ['ALTER TABLE users ADD |', 'COLUMN', 'keyword'],
    ['ALTER TABLE users ADD COLUMN |', 'IF', 'keyword'],
    ['CREATE TABLE |', 'IF', 'keyword'],
    ['DROP TABLE |', 'users', 'table'],
  ])(
    'keeps contextual %s completion ahead of vocabulary noise in %j',
    async (markedSQL, contextualLabel, contextualKind) => {
      const cursor = markedSQL.indexOf('|')
      expect(cursor).toBeGreaterThanOrEqual(0)
      expect(markedSQL.indexOf('|', cursor + 1)).toBe(-1)
      const sql = markedSQL.slice(0, cursor) + markedSQL.slice(cursor + 1)
      const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
        String(input).includes('completion-vocabulary')
          ? vocabularyResponse()
          : semanticCompletionResponse([
              {
                label: contextualLabel,
                kind: contextualKind,
                insert_text: contextualLabel,
                replace_start: cursor,
                replace_end: cursor,
                score: contextualKind === 'keyword' ? 40 : 100,
              },
              {
                label: 'GRAMMAR_FALLBACK',
                kind: 'keyword',
                insert_text: 'GRAMMAR_FALLBACK',
                replace_start: cursor,
                replace_end: cursor,
                score: 20,
              },
            ]),
      )
      vi.stubGlobal('fetch', fetchMock)
      const state = EditorState.create({ doc: sql })
      const source = remoteSQLCompletionSource({
        orgSlug: 'acme',
        workspaceId: 1,
        connectionId: 2,
        driver: 'postgres',
      })

      const result = await source(new CompletionContext(state, cursor, true))
      expect(result?.options[0]).toMatchObject({
        label: contextualLabel,
        type: contextualKind,
      })
      expect(result?.options.map((option) => option.label)).not.toContain('COUNT')
      expect(fetchMock).toHaveBeenCalledOnce()
    },
  )

  it('uses prefix-filtered vocabulary only when semantic completion has no answer', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
      String(input).includes('completion-vocabulary')
        ? vocabularyResponse()
        : semanticCompletionResponse([]),
    )
    vi.stubGlobal('fetch', fetchMock)
    const state = EditorState.create({ doc: 'SELECT CO' })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })

    const result = await source(new CompletionContext(state, state.doc.length, true))
    expect(result?.options.map((option) => option.label)).toEqual(['COUNT'])
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it.each(['postgres', 'mysql'])(
    'does not merge the $driver vocabulary into a non-empty contextual result',
    async (driver) => {
      const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
        String(input).includes('completion-vocabulary')
          ? vocabularyResponse()
          : semanticCompletionResponse([
              {
                label: 'ADD',
                kind: 'keyword',
                insert_text: 'ADD',
                replace_start: 18,
                replace_end: 19,
                score: 40,
              },
            ]),
      )
      vi.stubGlobal('fetch', fetchMock)
      const state = EditorState.create({ doc: 'ALTER TABLE users A' })
      const source = remoteSQLCompletionSource({
        orgSlug: 'acme',
        workspaceId: 1,
        connectionId: 2,
        driver,
      })

      const result = await source(new CompletionContext(state, state.doc.length, true))
      expect(result?.options.map((option) => option.label)).toEqual(['ADD'])
      expect(fetchMock).toHaveBeenCalledTimes(2)
    },
  )

  it.each([
    ['SELECT ', ' '],
    ['SELECT * FROM ', ' '],
    ['SELECT * FROM actor a JOIN ', ' '],
    ['SELECT * FROM actor WHERE ', ' '],
    ['SELECT * FROM actor ORDER BY ', ' '],
    ['SELECT * FROM actor a WHERE a.', '.'],
    ['SELECT first_name,', ','],
    ['SELECT COALESCE(', '('],
  ])('recognizes useful automatic semantic context in %j', (source, trigger) => {
    expect(automaticSQLCompletionTrigger(source, source.length)).toBe(trigger)
  })

  it.each([
    'SELECT * FROM actor ',
    'SELECT * ',
    'SELECT * FROM  ',
    "SELECT 'unfinished value ",
    'SELECT "unfinished identifier ',
    'SELECT * FROM actor -- comment ',
    'SELECT * FROM actor /* comment ',
    'SELECT 1.',
  ])('does not treat ordinary or protected typing as a semantic trigger in %j', (source) => {
    expect(automaticSQLCompletionTrigger(source, source.length)).toBeUndefined()
  })

  it('avoids a connection request after a completed relation but requests after FROM', async () => {
    const fetchMock = vi.fn(async () =>
      semanticCompletionResponse([
        {
          label: 'actor',
          kind: 'table',
          insert_text: 'actor',
          replace_start: 14,
          replace_end: 14,
          score: 80,
        },
      ]),
    )
    vi.stubGlobal('fetch', fetchMock)
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })

    const completedRelation = EditorState.create({ doc: 'SELECT * FROM actor ' })
    expect(
      await source(new CompletionContext(completedRelation, completedRelation.doc.length, false)),
    ).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()

    const relationPosition = EditorState.create({ doc: 'SELECT * FROM ' })
    const result = await source(
      new CompletionContext(relationPosition, relationPosition.doc.length, false),
    )
    expect(result?.options.map((option) => option.label)).toContain('actor')
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it.each(['postgres', 'mysql'])(
    'retries %s relation completion for the current prefix when the FROM request is superseded',
    async (driver) => {
      const initialSQL = 'SELECT * FROM '
      const finalSQL = 'SELECT * FROM ver'
      let initialRequestAborted = false
      const fetchMock = vi.fn(
        async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
          if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
          const body = JSON.parse(String(init?.body)) as {
            sql: string
            trigger_kind: string
            trigger_character?: string
          }
          if (body.sql === initialSQL) {
            return new Promise<Response>((_resolve, reject) => {
              init?.signal?.addEventListener('abort', () => {
                initialRequestAborted = true
                reject(new DOMException('Aborted', 'AbortError'))
              })
            })
          }
          expect(body).toEqual({
            sql: finalSQL,
            cursor_offset: finalSQL.length,
            trigger_kind: 'automatic',
          })
          return semanticCompletionResponse([
            {
              label: 'very_long_table_name',
              kind: 'table',
              insert_text: 'very_long_table_name',
              replace_start: finalSQL.length - 3,
              replace_end: finalSQL.length,
              score: 90,
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

      const initialState = EditorState.create({ doc: initialSQL })
      const initialResult = source(
        new CompletionContext(initialState, initialState.doc.length, false),
      )
      await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))

      const finalState = EditorState.create({ doc: finalSQL })
      const finalResult = await source(
        new CompletionContext(finalState, finalState.doc.length, false),
      )

      expect(initialRequestAborted).toBe(true)
      expect(await initialResult).toBeNull()
      expect(finalResult?.options.map((option) => option.label)).toEqual(['very_long_table_name'])
      expect(fetchMock).toHaveBeenCalledTimes(3)
    },
  )

  it.each(['postgres', 'mysql'])(
    'requests contextual %s columns while typing inside a CTE select list',
    async (driver) => {
      const sql = `WITH customer_total_spent AS (
  SELECT cust
  FROM payment
  GROUP BY customer_id
)
SELECT * FROM customer_total_spent`
      const cursor = sql.indexOf('SELECT cust') + 'SELECT cust'.length
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
        expect(JSON.parse(String(init?.body))).toEqual({
          sql,
          cursor_offset: cursor,
          trigger_kind: 'automatic',
        })
        return semanticCompletionResponse([
          {
            label: 'customer_id',
            kind: 'column',
            insert_text: 'customer_id',
            replace_start: cursor - 4,
            replace_end: cursor,
            score: 100,
          },
        ])
      })
      vi.stubGlobal('fetch', fetchMock)
      const state = EditorState.create({ doc: sql, selection: { anchor: cursor } })
      const source = remoteSQLCompletionSource({
        orgSlug: 'acme',
        workspaceId: 1,
        connectionId: 2,
        driver,
      })

      const result = await source(new CompletionContext(state, cursor, false))

      expect(result?.options).toEqual(
        expect.arrayContaining([expect.objectContaining({ label: 'customer_id', type: 'column' })]),
      )
      expect(fetchMock).toHaveBeenCalledTimes(2)
    },
  )

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

function stubSingleCompletionFetch() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
    String(input).includes('completion-vocabulary')
      ? vocabularyResponse()
      : semanticCompletionResponse([
          {
            label: 'widgets',
            kind: 'table',
            insert_text: 'widgets',
            replace_start: 0,
            replace_end: 3,
            score: 80,
          },
        ]),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function createCompletionEditor(doc: string, extensions: Extension[] = []) {
  const parent = document.createElement('div')
  document.body.append(parent)
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc,
      selection: { anchor: doc.length },
      extensions: [
        ...extensions,
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
  return {
    parent,
    view,
    destroy() {
      view.destroy()
      parent.remove()
    },
  }
}

async function waitForCompletion(parent: HTMLElement): Promise<void> {
  await vi.waitFor(() => {
    expect(parent.querySelector('[role="option"]')).not.toBeNull()
  })
}

function dispatchEditorKey(
  view: EditorView,
  key: string,
  keyCode: number,
  init: KeyboardEventInit = {},
): KeyboardEvent {
  const event = new KeyboardEvent('keydown', {
    key,
    code: init.code ?? key,
    bubbles: true,
    cancelable: true,
    ...init,
  })
  Object.defineProperty(event, 'keyCode', { value: keyCode })
  view.contentDOM.dispatchEvent(event)
  return event
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
