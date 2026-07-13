import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
