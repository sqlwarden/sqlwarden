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

interface SaveFilePickerWindow extends Window {
  showSaveFilePicker?: (options?: {
    suggestedName?: string
  }) => Promise<{ createWritable: () => Promise<FileSystemWritableFileStream> }>
}

interface FileSystemWritableFileStream {
  write: (data: Blob) => Promise<void>
  close: () => Promise<void>
}

/**
 * "Save As" a blob to local disk, prompting the user to pick a location via the
 * File System Access API where supported (Chromium browsers). Falls back to the
 * plain anchor-download — straight to the default downloads folder, no prompt —
 * in browsers that don't implement it (Firefox, Safari) or when the user cancels.
 */
export async function saveBlobAsWithPicker(filename: string, blob: Blob) {
  const picker = (window as SaveFilePickerWindow).showSaveFilePicker
  if (!picker) {
    saveBlobAs(filename, blob)
    return
  }
  try {
    const handle = await picker({ suggestedName: filename })
    const writable = await handle.createWritable()
    await writable.write(blob)
    await writable.close()
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') return
    saveBlobAs(filename, blob)
  }
}

export async function saveTextAsWithPicker(filename: string, text: string) {
  await saveBlobAsWithPicker(filename, new Blob([text], { type: 'text/plain;charset=utf-8' }))
}
