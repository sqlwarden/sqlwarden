import { setAccessToken } from '#/lib/auth/access-token'

export interface DesktopIdentity {
  account_id: number
  org_id: number
  org_slug: string
  workspace_id: number
}

export interface DesktopSession {
  access_token: string
  auth_session_id: string
  identity: DesktopIdentity
}

export interface DesktopPaths {
  data_dir: string
  database: string
  files: string
  logs: string
  config_file: string
}

export interface DesktopInfo {
  version: string
  paths: DesktopPaths
  startup_error?: string
}

interface DesktopBridge {
  StartSession(): Promise<DesktopSession>
  GetInfo(): Promise<DesktopInfo>
  RevealDataDirectory(): Promise<void>
  RevealLogDirectory(): Promise<void>
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
