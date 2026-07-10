/**
 * Save a blob to the user's local disk under the given filename. On the web this
 * triggers a browser download; the desktop build is expected to swap this for a
 * native "Save As" dialog while keeping the same call sites.
 */
export function saveBlobAs(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function saveTextAs(filename: string, text: string) {
  saveBlobAs(filename, new Blob([text], { type: 'text/plain;charset=utf-8' }))
}
