import { StateEffect, StateField } from '@codemirror/state'
import { Decoration, EditorView, type DecorationSet } from '@codemirror/view'

/** Sets or clears the subtle background shown behind the statement that the
 *  hovered Run/Explain hint would act on. `null` clears it. */
export const setStatementPreview = StateEffect.define<{ from: number; to: number } | null>()

const previewMark = Decoration.mark({ class: 'cm-statement-preview' })

const statementPreviewField = StateField.define<DecorationSet>({
  create() {
    return Decoration.none
  },
  update(deco, tr) {
    for (const effect of tr.effects) {
      if (effect.is(setStatementPreview)) {
        return effect.value
          ? Decoration.set([previewMark.range(effect.value.from, effect.value.to)])
          : Decoration.none
      }
    }
    return tr.docChanged ? Decoration.none : deco
  },
  provide: (field) => EditorView.decorations.from(field),
})

/** Renders the subtle preview background; register once per editor instance. */
export function statementPreviewHighlightExtension() {
  return statementPreviewField
}
