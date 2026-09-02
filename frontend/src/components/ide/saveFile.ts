import { platformService } from '#/lib/platform/service'

/**
 * Save a blob to the user's local disk under the given filename. On the web this
 * triggers a browser download; the desktop build uses the platform service's
 * native Save dialog while keeping the same call sites.
 */
export function saveBlobAs(filename: string, blob: Blob) {
  const service = platformService()
  if (service.native) {
    void blob.text().then((content) => service.saveExport(filename, content))
    return
  }
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function saveTextAs(filename: string, text: string) {
  const service = platformService()
  if (service.native) {
    if (filename.toLowerCase().endsWith('.sql')) {
      void service.saveSQLFile(filename, text)
    } else {
      void service.saveExport(filename, text)
    }
    return
  }
  saveBlobAs(filename, new Blob([text], { type: 'text/plain;charset=utf-8' }))
}
