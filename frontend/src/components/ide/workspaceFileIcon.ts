import type { WorkspaceFile } from '#/lib/api/types'
import type { FileTypeIconName } from '#/lib/icons'

const ICON_BY_EXTENSION: Readonly<Record<string, FileTypeIconName>> = {
  sql: 'sql',
  csv: 'csv',
  tsv: 'csv',
  xlsx: 'excel',
  xls: 'excel',
  parquet: 'parquet',
  json: 'json',
  jsonl: 'json',
  ndjson: 'json',
  xml: 'xml',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'toml',
  md: 'markdown',
  mdx: 'markdown',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  ps1: 'shell',
  txt: 'text',
  log: 'log',
  bin: 'binary',
}

const ICON_BY_FILE_KIND: Readonly<Record<string, FileTypeIconName>> = {
  sql: 'sql',
  csv: 'csv',
  json: 'json',
  markdown: 'markdown',
  text: 'text',
}

function filenameExtension(name: string): string | undefined {
  const basename = name.trim().toLowerCase()
  const dot = basename.lastIndexOf('.')
  if (dot <= 0 || dot === basename.length - 1) return undefined
  return basename.slice(dot + 1)
}

/** Returns the file-type icon used for a file in the workspace explorer. */
export function workspaceFileIcon(
  file: Pick<WorkspaceFile, 'name' | 'media_type' | 'file_kind'>,
): FileTypeIconName {
  const extension = filenameExtension(file.name)
  if (extension && ICON_BY_EXTENSION[extension]) return ICON_BY_EXTENSION[extension]

  const kind = file.file_kind?.trim().toLowerCase()
  if (kind && ICON_BY_FILE_KIND[kind]) return ICON_BY_FILE_KIND[kind]

  const mediaType = file.media_type?.split(';', 1)[0]?.trim().toLowerCase()
  if (mediaType === 'text/csv' || mediaType === 'text/tab-separated-values') return 'csv'
  if (mediaType === 'text/markdown') return 'markdown'
  if (mediaType?.includes('json')) return 'json'
  if (mediaType?.startsWith('audio/')) return 'audio'

  return 'default'
}
