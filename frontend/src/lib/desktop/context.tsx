import { createContext, useContext } from 'react'
import type { DesktopSession } from './runtime'

export interface DesktopRuntimeState {
  native: boolean
  session?: DesktopSession
}

export const DesktopRuntimeContext = createContext<DesktopRuntimeState>({ native: false })

export function useDesktopRuntime() {
  return useContext(DesktopRuntimeContext)
}
