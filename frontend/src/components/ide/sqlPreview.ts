export function isExpandableSql(sqlText: string): boolean {
  return sqlText.includes('\n') || sqlText.length > 80
}

export function flattenSql(sqlText: string): string {
  return sqlText.replace(/\s+/g, ' ').trim()
}
