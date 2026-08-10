import type { WorkspaceFile } from '#/lib/api/types'
import type { AppIcon } from '#/lib/icons'

const ICON_BY_EXTENSION: Readonly<Record<string, AppIcon>> = {
  sql: 'database',
  csv: 'table',
  tsv: 'table',
  json: 'type-json',
  jsonl: 'type-json',
  ndjson: 'type-json',
  yaml: 'type-json',
  yml: 'type-json',
  toml: 'type-json',
  xml: 'type-json',
  md: 'book-open-02',
  mdx: 'book-open-02',
  sh: 'terminal',
  bash: 'terminal',
  zsh: 'terminal',
  ps1: 'terminal',
  txt: 'subject',
  log: 'subject',
  bin: 'type-binary',
}

const ICON_BY_FILE_KIND: Readonly<Record<string, AppIcon>> = {
  sql: 'database',
  csv: 'table',
  json: 'type-json',
  markdown: 'book-open-02',
  text: 'subject',
}

function filenameExtension(name: string): string | undefined {
  const basename = name.trim().toLowerCase()
  const dot = basename.lastIndexOf('.')
  if (dot <= 0 || dot === basename.length - 1) return undefined
  return basename.slice(dot + 1)
}

/** Returns the semantic icon used for a file in the workspace explorer. */
export function workspaceFileIcon(
  file: Pick<WorkspaceFile, 'name' | 'media_type' | 'file_kind'>,
): AppIcon {
  const extension = filenameExtension(file.name)
  if (extension && ICON_BY_EXTENSION[extension]) return ICON_BY_EXTENSION[extension]

  const kind = file.file_kind?.trim().toLowerCase()
  if (kind && ICON_BY_FILE_KIND[kind]) return ICON_BY_FILE_KIND[kind]

  const mediaType = file.media_type?.split(';', 1)[0]?.trim().toLowerCase()
  if (mediaType === 'text/csv' || mediaType === 'text/tab-separated-values') return 'table'
  if (mediaType === 'text/markdown') return 'book-open-02'
  if (mediaType?.includes('json')) return 'type-json'
  if (mediaType?.startsWith('audio/')) return 'audio-wave-01'

  return 'file-01'
}
