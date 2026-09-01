import { desktopBridge, isNativeDesktop, type NativeTextFile } from '#/lib/desktop/runtime'

export interface PlatformService {
  native: boolean
  openSQLFile(): Promise<NativeTextFile | undefined>
  saveSQLFile(name: string, content: string): Promise<string | undefined>
  saveExport(name: string, content: string): Promise<string | undefined>
  chooseSQLiteFile(): Promise<string | undefined>
  chooseDirectory(): Promise<string | undefined>
  openExternalURL(url: string): Promise<void>
}

function download(name: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: 'text/plain;charset=utf-8' }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.click()
  URL.revokeObjectURL(url)
}

const browserService: PlatformService = {
  native: false,
  async openSQLFile() {
    return new Promise((resolve) => {
      const input = document.createElement('input')
      input.type = 'file'
      input.accept = '.sql,text/plain'
      input.onchange = async () => {
        const file = input.files?.[0]
        if (!file) return resolve(undefined)
        resolve({ path: '', name: file.name, content: await file.text() })
      }
      input.click()
    })
  },
  async saveSQLFile(name, content) {
    download(name, content)
    return undefined
  },
  async saveExport(name, content) {
    download(name, content)
    return undefined
  },
  async chooseSQLiteFile() {
    return undefined
  },
  async chooseDirectory() {
    return undefined
  },
  async openExternalURL(url) {
    window.open(url, '_blank', 'noopener,noreferrer')
  },
}

const nativeService: PlatformService = {
  native: true,
  openSQLFile: async () => desktopBridge()?.OpenSQLFile?.(),
  saveSQLFile: async (name, content) => desktopBridge()?.SaveSQLFile?.(name, content),
  saveExport: async (name, content) => desktopBridge()?.SaveExportFile?.(name, content),
  chooseSQLiteFile: async () => desktopBridge()?.ChooseSQLiteFile?.(),
  chooseDirectory: async () => desktopBridge()?.ChooseDirectory?.(),
  openExternalURL: async (url) => desktopBridge()?.OpenExternalURL?.(url),
}

export function platformService(): PlatformService {
  return isNativeDesktop() ? nativeService : browserService
}
