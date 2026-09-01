import { configureInMemoryAccessTokens, setAccessToken } from '#/lib/auth/access-token'

export interface DesktopIdentity {
  account_id: number
  org_id: number
  org_slug: string
  workspace_id?: number
}

export interface DesktopSession {
  access_token: string
  auth_session_id: string
  identity: DesktopIdentity
}

export interface DesktopPaths {
  config_dir?: string
  data_dir: string
  database: string
  files: string
  logs: string
  backups?: string
  cache?: string
  temp?: string
  config_file: string
}

export interface DesktopInfo {
  version: string
  paths: DesktopPaths
  secret_store?: 'keyring' | 'protected-file' | 'unavailable'
  startup_error?: string
}

export interface NativeTextFile {
  path: string
  name: string
  content: string
}

export interface NativeOpenRequests {
  files: NativeTextFile[]
  sqlite_files: string[]
}

interface DesktopBridge {
  StartSession(): Promise<DesktopSession>
  GetInfo(): Promise<DesktopInfo>
  RevealDataDirectory(): Promise<void>
  RevealLogDirectory(): Promise<void>
  RevealBackupDirectory?(): Promise<void>
  OpenSQLFile?(): Promise<NativeTextFile>
  SaveSQLFile?(suggestedName: string, content: string): Promise<string>
  SaveExportFile?(suggestedName: string, content: string): Promise<string>
  ChooseSQLiteFile?(): Promise<string>
  ChooseDirectory?(): Promise<string>
  OpenExternalURL?(url: string): Promise<void>
  OpenReleasePage?(): Promise<void>
  SaveDiagnostics?(): Promise<string>
  CreateBackup?(): Promise<string>
  RestoreBackup?(): Promise<string>
  SetUnsavedChanges?(unsaved: boolean): Promise<void>
  SetTheme?(theme: 'dark' | 'light' | 'system', resolvedTheme: 'dark' | 'light'): Promise<void>
  DrainOpenRequests?(): Promise<NativeOpenRequests>
}

declare global {
  interface Window {
    go?: {
      main?: {
        DesktopBridge?: DesktopBridge
      }
    }
  }
}

export function desktopBridge() {
  return window.go?.main?.DesktopBridge
}

export function isNativeDesktop() {
  return Boolean(desktopBridge())
}

export async function startDesktopSession() {
  const bridge = desktopBridge()
  if (!bridge) return undefined
  configureInMemoryAccessTokens()
  const session = await bridge.StartSession()
  setAccessToken(session.access_token)
  return session
}

export async function getDesktopInfo() {
  return desktopBridge()?.GetInfo()
}

export async function revealDesktopDataDirectory() {
  await desktopBridge()?.RevealDataDirectory()
}

export async function revealDesktopLogDirectory() {
  await desktopBridge()?.RevealLogDirectory()
}

export async function revealDesktopBackupDirectory() {
  await desktopBridge()?.RevealBackupDirectory?.()
}

export async function saveDesktopDiagnostics() {
  return desktopBridge()?.SaveDiagnostics?.()
}

export async function openDesktopReleasePage() {
  await desktopBridge()?.OpenReleasePage?.()
}

export async function createDesktopBackup() {
  return desktopBridge()?.CreateBackup?.()
}

export async function restoreDesktopBackup() {
  return desktopBridge()?.RestoreBackup?.()
}

export async function drainDesktopOpenRequests() {
  return desktopBridge()?.DrainOpenRequests?.()
}

export async function syncDesktopTheme(
  theme: 'dark' | 'light' | 'system',
  resolvedTheme: 'dark' | 'light',
) {
  await desktopBridge()?.SetTheme?.(theme, resolvedTheme)
}
