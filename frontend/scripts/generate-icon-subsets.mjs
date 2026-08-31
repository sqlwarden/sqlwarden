// Generates trimmed Iconify collections containing only the icons the app
// actually references, instead of shipping the full multi-megabyte sets.
//
// Source of truth is each pack map in src/lib/icons/packs/<pack>.ts whose
// values are "<prefix>:<name>" strings. We read the full @iconify-json
// collection, keep only the referenced icons (resolving aliases), and write
// src/lib/icons/packs/<pack>.subset.json which context.tsx loads at runtime.
//
// Run via: bun run icons:generate

import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { getIcons } from '@iconify/utils'

const here = dirname(fileURLToPath(import.meta.url))
const packsDir = resolve(here, '../src/lib/icons/packs')

/** @type {Record<string, string>} pack file -> @iconify-json collection module */
const packs = {
  hugeicons: '@iconify-json/hugeicons/icons.json',
  lucide: '@iconify-json/lucide/icons.json',
  remix: '@iconify-json/ri/icons.json',
  'vscode-file-icons': '@iconify-json/vscode-icons/icons.json',
}

// Some upstream icon colors clash with our theme (e.g. vscode-icons' SQL disc
// is bright yellow, unreadable on light backgrounds). Recolor those specific
// icons post-fetch rather than forking the whole upstream collection.
/** @type {Record<string, Record<string, Record<string, string>>>} */
const recolors = {
  'vscode-file-icons': {
    'file-type-sql': { '#ffda44': '#0f5fa8' },
  },
}

// Pull the "<prefix>:<name>" string literals out of a pack .ts map.
function readPackNames(packFile) {
  const src = readFileSync(resolve(packsDir, `${packFile}.ts`), 'utf8')
  const names = new Set()
  for (const match of src.matchAll(/(?:'[^']+'|\w+)\s*:\s*'([^':]+):([^']+)'/g)) {
    names.add(match[2])
  }
  return [...names]
}

for (const [packFile, collectionModule] of Object.entries(packs)) {
  const collectionPath = fileURLToPath(import.meta.resolve(collectionModule))
  const collection = JSON.parse(readFileSync(collectionPath, 'utf8'))
  const names = readPackNames(packFile)

  const subset = getIcons(collection, names)
  if (!subset) throw new Error(`getIcons returned nothing for ${packFile}`)

  const missing = names.filter((n) => !subset.icons[n] && !subset.aliases?.[n])
  if (missing.length) {
    throw new Error(`${packFile}: icons not found in ${collectionModule}: ${missing.join(', ')}`)
  }

  for (const [iconName, colorMap] of Object.entries(recolors[packFile] ?? {})) {
    const icon = subset.icons[iconName]
    if (!icon) throw new Error(`${packFile}: recolor target not found: ${iconName}`)
    for (const [from, to] of Object.entries(colorMap)) {
      icon.body = icon.body.replaceAll(from, to)
    }
  }

  const outPath = resolve(packsDir, `${packFile}.subset.json`)
  writeFileSync(outPath, JSON.stringify(subset))
  const kept = Object.keys(subset.icons).length
  console.log(`${packFile}: ${kept} icons -> ${outPath}`)
}
