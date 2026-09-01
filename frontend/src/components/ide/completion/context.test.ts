import { expect, it } from 'vitest'
import { classifyCursorContext, scanSQLTriggerPrefix } from './context'

const at = (sql: string) => classifyCursorContext(sql, sql.length)

it('classifies a relation position after FROM', () => {
  const ctx = at('SELECT id FROM ')
  expect(ctx.positionClass).toBe('relation')
  expect(ctx.protectedRegion).toBe(false)
})

it('classifies a relation position after JOIN', () => {
  expect(at('SELECT * FROM orders o JOIN ').positionClass).toBe('relation')
})

it('classifies a column position inside the SELECT list and captures FROM refs', () => {
  const ctx = classifyCursorContext('SELECT  FROM public.orders o, customers c', 'SELECT '.length)
  expect(ctx.positionClass).toBe('column')
  expect(ctx.fromRefs).toEqual([
    { table: 'orders', schema: 'public', alias: 'o' },
    { table: 'customers', alias: 'c' },
  ])
})

it('classifies a qualified position after a dot and reports the qualifier', () => {
  const sql = 'SELECT o. FROM orders o'
  const ctx = classifyCursorContext(sql, 'SELECT o.'.length)
  expect(ctx.positionClass).toBe('qualified')
  expect(ctx.qualifier).toBe('o')
})

it('classifies a bare keyword slot', () => {
  expect(at('SELECT id FROM orders ').positionClass).toBe('keyword')
})

it('captures the identifier prefix being typed', () => {
  expect(at('SELECT id FROM ord').prefix).toBe('ord')
  expect(classifyCursorContext('SELECT o.to FROM orders o', 'SELECT o.to'.length).prefix).toBe('to')
})

it('marks comments and unterminated strings as protected', () => {
  expect(at('SELECT id FROM orders -- note ').protectedRegion).toBe(true)
  expect(at("SELECT 'unterminated ").protectedRegion).toBe(true)
})

it('does not leak FROM refs across a statement boundary', () => {
  const ctx = classifyCursorContext(
    'SELECT * FROM a; SELECT  FROM b',
    'SELECT * FROM a; SELECT '.length,
  )
  expect(ctx.fromRefs).toEqual([{ table: 'b' }])
})

it('collects CTE names bound by a leading WITH clause', () => {
  const ctx = at('WITH recent_orders AS (SELECT id FROM orders) SELECT * FROM ')
  expect(ctx.cteNames).toEqual(new Set(['recent_orders']))
})

it('collects multiple comma-separated CTE names, including a column list', () => {
  const ctx = at('WITH a AS (SELECT 1), b (x, y) AS (SELECT 1, 2) SELECT * FROM ')
  expect(ctx.cteNames).toEqual(new Set(['a', 'b']))
})

it('does not treat a subquery-only statement as defining a CTE', () => {
  const ctx = at('SELECT * FROM (SELECT id FROM orders) t')
  expect(ctx.cteNames).toEqual(new Set())
})

it('scopes FROM refs to the CTE body when the cursor sits inside its parens', () => {
  const sql =
    'WITH recent_orders AS (SELECT id FROM orders o JOIN customers c ON c.id = o.customer_id WHERE ) SELECT * FROM recent_orders'
  const ctx = classifyCursorContext(sql, sql.indexOf('WHERE ') + 'WHERE '.length)
  expect(ctx.fromRefs).toEqual([
    { table: 'orders', alias: 'o' },
    { table: 'customers', alias: 'c' },
  ])
})

it('scopes FROM refs to the outer query, ignoring a closed CTE body', () => {
  const sql =
    'WITH recent_orders AS (SELECT id FROM orders o JOIN customers c ON c.id = o.customer_id) SELECT  FROM recent_orders'
  const ctx = classifyCursorContext(sql, sql.indexOf('SELECT  ') + 'SELECT '.length)
  expect(ctx.positionClass).toBe('column')
  expect(ctx.fromRefs).toEqual([{ table: 'recent_orders' }])
})

it('classifies an INSERT column list as a column position targeting the insert table', () => {
  const sql = 'INSERT INTO users (id, '
  const ctx = classifyCursorContext(sql, sql.length)
  expect(ctx.positionClass).toBe('column')
  expect(ctx.fromRefs).toEqual([{ table: 'users' }])
})

it('keeps the schema on a schema-qualified INSERT column list target', () => {
  const sql = 'INSERT INTO app.users ('
  const ctx = classifyCursorContext(sql, sql.length)
  expect(ctx.positionClass).toBe('column')
  expect(ctx.fromRefs).toEqual([{ table: 'users', schema: 'app' }])
})

it('classifies an INSERT VALUES slot as a value position, not a column list', () => {
  const sql = 'INSERT INTO users (id) VALUES ('
  expect(classifyCursorContext(sql, sql.length).positionClass).toBe('value')
})

it('collects CTE names from a WITH nested inside a subquery', () => {
  const ctx = at('SELECT * FROM (WITH inner_cte AS (SELECT 1) SELECT * FROM ')
  expect(ctx.cteNames).toEqual(new Set(['inner_cte']))
})

it('collects CTE names from both an outer and a nested WITH clause', () => {
  const ctx = at(
    'WITH outer_cte AS (SELECT 1) SELECT * FROM (WITH inner_cte AS (SELECT 2) SELECT * FROM ',
  )
  expect(ctx.cteNames).toEqual(new Set(['outer_cte', 'inner_cte']))
})

it('does not emit a phantom FROM ref for a derived table alias', () => {
  const ctx = classifyCursorContext('SELECT  FROM (SELECT id FROM t) d', 'SELECT '.length)
  expect(ctx.fromRefs).toEqual([])
})

it('still captures a real table joined after a derived table', () => {
  const sql = 'SELECT  FROM (SELECT id FROM t) d JOIN real_table r ON true'
  const ctx = classifyCursorContext(sql, 'SELECT '.length)
  expect(ctx.fromRefs).toEqual([{ table: 'real_table', alias: 'r' }])
})

it('does not reset the token scan on a semicolon inside parentheses', () => {
  const scan = scanSQLTriggerPrefix('SELECT f(a ; b) ')
  expect(scan.tokens.some((t) => t.kind === 'word' && t.text === 'SELECT')).toBe(true)
})
