import type { ExtensionModule } from '#/lib/extensions/module'

// The default build has no optional implementation. Shared product surfaces
// can still render public upgrade information from the optional-feature catalog.
export const extensionModule: ExtensionModule = {
  pages: {},
  slots: {},
}
