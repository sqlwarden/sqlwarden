export type FileType = {
  kind: string       // file_kind value sent to the API
  mediaType: string  // media_type value sent to the API
  extension: string  // filename extension including the dot, e.g. '.sql'
  label: string      // display label shown in the UI
}

export const FILE_TYPES: readonly FileType[] = [
  { kind: 'sql', mediaType: 'text/plain', extension: '.sql', label: 'SQL' },
]

export const DEFAULT_FILE_TYPE = FILE_TYPES[0]

/** Returns the final filename: appends extension unless the name already ends with it. */
export function buildFilename(basename: string, fileType: FileType): string {
  const trimmed = basename.trim()
  if (trimmed.toLowerCase().endsWith(fileType.extension)) return trimmed
  return trimmed + fileType.extension
}
