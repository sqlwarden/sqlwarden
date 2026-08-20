import type { ComponentType } from 'react'
import { BrandColorLockup, BrandFullLogo } from './BrandFullLogo'
import { BrandMark } from './BrandMark'

export type BrandLogoProps = { size?: number; className?: string }

export type BrandConfig = {
  productName: string
  LogoMark: ComponentType<BrandLogoProps>
  LogoLockup: ComponentType<BrandLogoProps>
  LogoColorLockup: ComponentType<BrandLogoProps>
  faviconPath: string
  manifestPath: string
}

export const defaultBrand: BrandConfig = {
  productName: 'SQLWarden',
  LogoMark: BrandMark,
  LogoLockup: BrandFullLogo,
  LogoColorLockup: BrandColorLockup,
  faviconPath: '/favicon.ico',
  manifestPath: '/manifest.json',
}

export function useBrand(): BrandConfig {
  return defaultBrand
}
