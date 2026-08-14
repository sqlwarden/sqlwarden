import type { EditorView, Panel } from '@codemirror/view'
import { findPanelHost } from './findPanelBridge'

/**
 * CodeMirror search Panel whose contents are rendered by React. createFindPanel
 * only creates the host element and announces it through the findPanelHost facet;
 * the owning SqlEditor portals <FindPanel> into it (so it keeps app context).
 * Wired via search({ createPanel: createFindPanel }).
 */
export function createFindPanel(view: EditorView): Panel {
  const dom = document.createElement('div')
  dom.className = 'cm-sqlwarden-find'

  const notify = view.state.facet(findPanelHost)
  notify?.({ dom, view })

  return {
    dom,
    top: true,
    destroy() {
      notify?.(null)
    },
  }
}
