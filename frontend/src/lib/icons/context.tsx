import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { addCollection } from '@iconify/react'
import type { AppIcon } from './registry'

export type IconPackName = 'hugeicons' | 'lucide' | 'remix'

type IconMap = Record<AppIcon, string>

type IconPackContextValue = {
  iconMap: IconMap | null
  packName: IconPackName
  setPackName: (name: IconPackName) => void
}

const IconPackContext = createContext<IconPackContextValue>({
  iconMap: null,
  packName: 'hugeicons',
  setPackName: () => {},
})

const STORAGE_KEY = 'sqlwarden.preference.icon_pack'

const packLoaders: Record<IconPackName, () => Promise<IconMap>> = {
  hugeicons: async () => {
    const [{ default: data }, { default: map }] = await Promise.all([
      import('@iconify-json/hugeicons/icons.json'),
      import('./packs/hugeicons'),
    ])
    addCollection(data as Parameters<typeof addCollection>[0])
    return map
  },
  lucide: async () => {
    const [{ default: data }, { default: map }] = await Promise.all([
      import('@iconify-json/lucide/icons.json'),
      import('./packs/lucide'),
    ])
    addCollection(data as Parameters<typeof addCollection>[0])
    return map
  },
  remix: async () => {
    const [{ default: data }, { default: map }] = await Promise.all([
      import('@iconify-json/ri/icons.json'),
      import('./packs/remix'),
    ])
    addCollection(data as Parameters<typeof addCollection>[0])
    return map
  },
}

function readStoredPack(): IconPackName {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'lucide' || stored === 'remix') return stored
  return 'hugeicons'
}

export function IconPackProvider({ children }: { children: ReactNode }) {
  const [packName, setPackNameState] = useState<IconPackName>(readStoredPack)
  const [iconMap, setIconMap] = useState<IconMap | null>(null)

  function setPackName(name: IconPackName) {
    localStorage.setItem(STORAGE_KEY, name)
    setPackNameState(name)
  }

  useEffect(() => {
    let cancelled = false
    setIconMap(null)
    packLoaders[packName]().then((map) => {
      if (!cancelled) setIconMap(map)
    })
    return () => { cancelled = true }
  }, [packName])

  return (
    <IconPackContext.Provider value={{ iconMap, packName, setPackName }}>
      {children}
    </IconPackContext.Provider>
  )
}

export function useIconPack() {
  return useContext(IconPackContext)
}
