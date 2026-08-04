import type { ComponentType } from 'react'
import { BrandFullLogo } from './BrandFullLogo'
import { BrandMark } from './BrandMark'

export type BrandLogoProps = { size?: number; className?: string }

export type BrandConfig = {
  productName: string
  LogoMark: ComponentType<BrandLogoProps>
  LogoLockup: ComponentType<BrandLogoProps>
  faviconPath: string
  manifestPath: string
}

export const defaultBrand: BrandConfig = {
  productName: 'SQLWarden',
  LogoMark: BrandMark,
  LogoLockup: BrandFullLogo,
  faviconPath: '/favicon.ico',
  manifestPath: '/manifest.json',
}

export function useBrand(): BrandConfig {
  return defaultBrand
}
