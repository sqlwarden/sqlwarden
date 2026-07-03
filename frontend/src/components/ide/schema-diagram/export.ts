export type ExportFormat = 'png' | 'svg'

/** Pixel size of the export canvas for a diagram whose nodes span `bounds`.
 *  The image matches the diagram's aspect ratio (scaled by `scale` for
 *  resolution) and is clamped to `max` on its longest side so a huge diagram
 *  stays within browser canvas limits while keeping its proportions. */
export function exportDimensions(
  bounds: { width: number; height: number },
  scale = 2,
  max = 4096,
): { width: number; height: number } {
  const w = Math.max(1, bounds.width) * scale
  const h = Math.max(1, bounds.height) * scale
  const fit = Math.min(1, max / Math.max(w, h))
  return { width: Math.round(w * fit), height: Math.round(h * fit) }
}

/** Filename for a downloaded diagram, e.g. `public-users-diagram.png` for an
 *  object diagram or `public-diagram.svg` for a whole-namespace diagram.
 *  Non-filename-safe characters are collapsed to underscores. */
export function diagramFileName(namespace: string, tableName: string | undefined, ext: ExportFormat): string {
  const base = tableName ? `${namespace}-${tableName}` : namespace
  const safe = (base || 'schema').replace(/[^\w.-]+/g, '_').replace(/^_+|_+$/g, '')
  return `${safe || 'schema'}-diagram.${ext}`
}

/** Trigger a browser download of a data URL (PNG/SVG produced by html-to-image). */
export function downloadDataUrl(dataUrl: string, filename: string) {
  const anchor = document.createElement('a')
  anchor.href = dataUrl
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
}
