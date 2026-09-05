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
  clearSQLCompletionCaches,
  dialectForDriver,
  remoteSQLCompletionSource,
  sqlCompletionExtension,
} from './index'
import { MySQL, PostgreSQL, SQLite, StandardSQL } from '@codemirror/lang-sql'

afterEach(() => {
  clearSQLCompletionCaches()
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
    expect(semanticCallCount(fetchMock)).toBe(1)
    expect(result?.from).toBe(15)
    expect(result?.options[0]).toMatchObject({
      label: 'widgets',
      apply: 'widgets',
      type: 'table',
      boost: 5080,
    })
  })

  it('issues the remote completion call for sqlite', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('completion-vocabulary')) {
        return vocabularyResponse()
      }
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
    const state = EditorState.create({ doc: 'SELECT x FROM wid' })
    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'sqlite',
    })
    const result = await source(new CompletionContext(state, state.doc.length, true))
    expect(semanticCallCount(fetchMock)).toBe(1)
    expect(result?.options[0]).toMatchObject({ label: 'widgets', type: 'table' })
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

  it('inserts a tab character at a bare cursor instead of moving browser focus', () => {
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
    expect(view.state.doc.toString()).toBe('SELECT 1\t')

    view.destroy()
  })

  it('indents the covered lines with Tab when there is a selection', () => {
    const parent = document.createElement('div')
    document.body.append(parent)
    const view = new EditorView({
      parent,
      state: EditorState.create({
        doc: 'SELECT 1',
        selection: { anchor: 0, head: 8 },
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
    expect(semanticCallCount(fetchMock)).toBe(0)
  })

  it('re-invokes the source as a word grows past the lexical threshold instead of filtering an object-only result', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('completion-index')) {
        return new Response(
          JSON.stringify({
            version: 'v1',
            default_schema: 'main',
            schemas: ['main'],
            objects: [{ schema: 'main', name: 'customers', kind: 'table' }],
            columns: [],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      if (String(input).includes('completion-vocabulary')) {
        return new Response(
          JSON.stringify({
            dialect: 'sqlite',
            version: 'threshold-test',
            suggestions: [{ label: 'ORDER', kind: 'keyword', score: 40 }],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      throw new Error(`unexpected fetch: ${String(input)}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'sqlite',
    })

    const short = EditorState.create({ doc: 'c' })
    const shortResult = await source(new CompletionContext(short, 1, false))
    expect(shortResult?.options.map((option) => option.label)).toEqual(['customers'])
    if (typeof shortResult?.validFor !== 'function') {
      throw new Error('expected a prefix-bound validFor function below the lexical threshold')
    }
    expect(
      shortResult.validFor('cu', shortResult.from, shortResult.to ?? shortResult.from, short),
    ).toBe(false)

    const settled = EditorState.create({ doc: 'or' })
    const settledResult = await source(new CompletionContext(settled, 2, false))
    expect(settledResult?.options.map((option) => option.label)).toContain('ORDER')
    expect(settledResult?.validFor).toBeInstanceOf(RegExp)
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
      expect(semanticCallCount(fetchMock)).toBe(1)
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
    expect(semanticCallCount(fetchMock)).toBe(1)
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
      expect(semanticCallCount(fetchMock)).toBe(1)
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
    expect(semanticCallCount(fetchMock)).toBe(0)

    const relationPosition = EditorState.create({ doc: 'SELECT * FROM ' })
    const result = await source(
      new CompletionContext(relationPosition, relationPosition.doc.length, false),
    )
    expect(result?.options.map((option) => option.label)).toContain('actor')
    expect(semanticCallCount(fetchMock)).toBe(1)
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
      await vi.waitFor(() => expect(semanticCallCount(fetchMock)).toBe(1))

      const finalState = EditorState.create({ doc: finalSQL })
      const finalResult = await source(
        new CompletionContext(finalState, finalState.doc.length, false),
      )

      expect(initialRequestAborted).toBe(true)
      expect(await initialResult).toBeNull()
      expect(finalResult?.options.map((option) => option.label)).toEqual(['very_long_table_name'])
      expect(semanticCallCount(fetchMock)).toBe(2)
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
      expect(semanticCallCount(fetchMock)).toBe(1)
    },
  )

  it.each([
    ['postgres', false],
    ['postgres', true],
    ['mysql', false],
    ['mysql', true],
  ] as const)(
    'requests standalone CTE columns for %s with explicit=%s and a commented outer query',
    async (driver, explicit) => {
      const marker = explicit ? '' : 'cust'
      const sql = `WITH customer_total_spent AS (
  SELECT
    ${marker}
  FROM payment
  GROUP BY customer_id
)
-- block
-- SELECT
-- FROM customer c
-- block`
      const cursorLine = `    ${marker}\n`
      const cursor = sql.indexOf(cursorLine) + cursorLine.length - 1
      const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
        expect(JSON.parse(String(init?.body))).toEqual({
          sql,
          cursor_offset: cursor,
          trigger_kind: explicit ? 'invoked' : 'automatic',
        })
        return semanticCompletionResponse([
          {
            label: 'customer_id',
            kind: 'column',
            insert_text: 'customer_id',
            replace_start: cursor - marker.length,
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

      const result = await source(new CompletionContext(state, cursor, explicit))

      expect(result?.options).toEqual(
        expect.arrayContaining([expect.objectContaining({ label: 'customer_id', type: 'column' })]),
      )
      expect(semanticCallCount(fetchMock)).toBe(1)
    },
  )

  it('calls semantic completion on a dot and preserves display labels', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
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
    expect(semanticCallCount(fetchMock)).toBe(1)
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
      await vi.waitFor(() => expect(semanticCallCount(fetchMock)).toBe(1))

      const finalState = EditorState.create({ doc: finalSQL })
      const finalResult = await source(new CompletionContext(finalState, finalSQL.length, false))

      expect(firstRequestAborted).toBe(true)
      expect(await firstResult).toBeNull()
      expect(finalResult?.options.map((option) => option.label)).toEqual(['actor_id'])
      expect(semanticCallCount(fetchMock)).toBe(2)
    },
  )

  it('serves a relation-position completion entirely from the local index with zero POST /completion calls', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('completion-index')) {
        return new Response(
          JSON.stringify({
            version: 'v1',
            default_schema: 'public',
            schemas: ['public'],
            objects: [{ schema: 'public', name: 'orders', kind: 'table' }],
            columns: [],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
      throw new Error(`unexpected fetch: ${String(input)}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const state = EditorState.create({ doc: 'SELECT id FROM ord' })
    const result = await source(new CompletionContext(state, 18, true))

    expect(result?.options.map((o) => o.label)).toContain('orders')
    expect(semanticCallCount(fetchMock)).toBe(0)
  })

  it('falls back to POST /completion for an unresolved qualified reference', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('completion-index')) {
        return new Response(
          JSON.stringify({
            version: 'v1',
            default_schema: 'public',
            schemas: [],
            objects: [],
            columns: [],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
      return new Response(
        JSON.stringify({
          suggestions: [
            {
              label: 'total',
              kind: 'column',
              insert_text: 'total',
              replace_start: 20,
              replace_end: 20,
              score: 90,
            },
          ],
          mode: 'persistent',
          metadata_available: true,
          metadata_status: 'ready',
          context: 'column',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    })
    vi.stubGlobal('fetch', fetchMock)

    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const state = EditorState.create({ doc: 'SELECT z. FROM orders o' })
    const result = await source(new CompletionContext(state, 9, true))

    expect(result?.options.map((o) => o.label)).toContain('total')
    expect(semanticCallCount(fetchMock)).toBe(1)
  })

  it('consults the backend and merges local rows for an explicit column completion on a warm index', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('completion-index')) {
        return new Response(
          JSON.stringify({
            version: 'v1',
            default_schema: 'public',
            schemas: ['public'],
            objects: [{ schema: 'public', name: 'orders', kind: 'table' }],
            columns: [
              { schema: 'public', table: 'orders', name: 'total', type: 'numeric' },
              { schema: 'public', table: 'orders', name: 'token', type: 'text' },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
      expect((JSON.parse(String(init?.body)) as { trigger_kind: string }).trigger_kind).toBe(
        'invoked',
      )
      return semanticCompletionResponse([
        {
          label: 'total_owed',
          kind: 'column',
          insert_text: 'total_owed',
          replace_start: 7,
          replace_end: 9,
          score: 100,
        },
      ])
    })
    vi.stubGlobal('fetch', fetchMock)

    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const state = EditorState.create({ doc: 'SELECT to FROM orders' })
    const result = await source(new CompletionContext(state, 9, true))

    const labels = result?.options.map((o) => o.label) ?? []
    expect(labels).toContain('total_owed')
    expect(labels).toContain('total')
    expect(semanticCallCount(fetchMock)).toBe(1)
  })

  it('consults the backend for an explicit qualified completion even when the alias resolves locally', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('completion-index')) {
        return new Response(
          JSON.stringify({
            version: 'v1',
            default_schema: 'public',
            schemas: ['public'],
            objects: [{ schema: 'public', name: 'orders', kind: 'table' }],
            columns: [{ schema: 'public', table: 'orders', name: 'total', type: 'numeric' }],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      if (String(input).includes('completion-vocabulary')) return vocabularyResponse()
      return semanticCompletionResponse([
        {
          label: 'discount',
          kind: 'column',
          insert_text: 'discount',
          replace_start: 9,
          replace_end: 9,
          score: 100,
        },
      ])
    })
    vi.stubGlobal('fetch', fetchMock)

    const source = remoteSQLCompletionSource({
      orgSlug: 'acme',
      workspaceId: 1,
      connectionId: 2,
      driver: 'postgres',
    })
    const state = EditorState.create({ doc: 'SELECT o. FROM orders o' })
    const result = await source(new CompletionContext(state, 9, true))

    const labels = result?.options.map((o) => o.label) ?? []
    expect(labels).toContain('discount')
    expect(semanticCallCount(fetchMock)).toBe(1)
  })

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

function semanticCallCount(fetchMock: ReturnType<typeof vi.fn>): number {
  return fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/completion')).length
}

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
