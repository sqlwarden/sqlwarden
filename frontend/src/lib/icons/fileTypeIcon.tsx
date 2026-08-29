import { addCollection, Icon as IconifyIcon } from '@iconify/react'
import { useEffect, useState } from 'react'
import iconMap from './packs/vscode-file-icons'
import type { FileTypeIconName } from './packs/vscode-file-icons'

export type { FileTypeIconName }

// Fixed-color file-type icons (vscode-icons) are a separate collection from
// the theme-swappable AppIcon set: unlike Icon/useIconPack, there's no pack
// switching here, so the collection is loaded once and cached module-wide
// rather than through IconPackProvider.
let loaded: Promise<void> | null = null

function ensureLoaded(): Promise<void> {
  if (!loaded) {
    loaded = import('./packs/vscode-file-icons.subset.json').then(({ default: data }) => {
      addCollection(data as Parameters<typeof addCollection>[0])
    })
  }
  return loaded
}

type FileTypeIconProps = {
  name: FileTypeIconName
  size?: number
  className?: string
}

/** Renders a fixed-color, file-type-specific icon (SQL, CSV, Parquet, ...). */
export function FileTypeIcon({ name, size = 20, className }: FileTypeIconProps) {
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let cancelled = false
    ensureLoaded().then(() => {
      if (!cancelled) setReady(true)
    })
    return () => {
      cancelled = true
    }
  }, [])

  if (!ready) {
    return <span style={{ display: 'inline-block', width: size, height: size }} />
  }

  return <IconifyIcon icon={iconMap[name]} width={size} height={size} className={className} />
}
