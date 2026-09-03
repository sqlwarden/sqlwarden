/** Illustrative snippet for the Appearance editor-theme preview. Not executed;
 *  chosen to exercise keywords, strings, numbers, comments, and functions so
 *  every theme's token colors are visible. */
export const SAMPLE_QUERY = `-- Recent high-value orders by customer
SELECT
  c.id,
  c.name,
  count(o.id) AS order_count,
  sum(o.total_cents) / 100.0 AS revenue
FROM customers AS c
JOIN orders AS o ON o.customer_id = c.id
WHERE o.created_at >= now() - interval '30 days'
  AND o.status = 'completed'
GROUP BY c.id, c.name
HAVING sum(o.total_cents) > 100000
ORDER BY revenue DESC
LIMIT 25;
`
