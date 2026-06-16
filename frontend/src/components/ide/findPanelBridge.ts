import { Facet } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'

/** The CodeMirror search panel's host element plus the view that owns it. */
export type FindPanelHost = { dom: HTMLElement; view: EditorView }

/** Called by createFindPanel when the panel opens (host) and closes (null). */
export type SetFindPanelHost = (host: FindPanelHost | null) => void

/**
 * Per-editor channel that lets the search panel (created imperatively by
 * CodeMirror) hand its host element back to the React tree, so <FindPanel> can
 * be rendered into it via a portal and inherit the app's providers (icons,
 * theme, …). Each SqlEditor supplies its own setter through this facet.
 */
export const findPanelHost = Facet.define<SetFindPanelHost, SetFindPanelHost | null>({
  combine: (values) => values[0] ?? null,
})
