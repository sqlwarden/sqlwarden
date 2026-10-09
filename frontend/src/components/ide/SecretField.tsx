import { useEffect, useState } from 'react'
import { Input } from '#/components/ui/input'
import { Textarea } from '#/components/ui/textarea'
import { Icon } from '#/lib/icons'
import type { SecretBinding } from './useSecretFields'
import { useSecretReveal } from './useSecretReveal'

const linkButtonClass =
  'font-medium underline underline-offset-2 hover:text-foreground disabled:opacity-50'

export function SecretField({
  binding,
  noun,
  label,
  multiline = false,
  placeholder,
  disabled = false,
  invalid = false,
}: {
  binding: SecretBinding
  /** Lower-case name used in action text, for example "password" or "client key". */
  noun: string
  /** Accessible name of the input. */
  label: string
  multiline?: boolean
  placeholder?: string
  disabled?: boolean
  invalid?: boolean
}) {
  const { state, dispatch } = binding
  const revealer = useSecretReveal(binding.reveal)
  const [visible, setVisible] = useState(false)
  const { hide } = revealer

  useEffect(() => {
    if (state.kind !== 'saved') hide()
  }, [state.kind, hide])

  if (state.kind === 'managed') {
    return (
      <div className="flex flex-col gap-1.5">
        <Input aria-label={label} value="Managed externally" readOnly disabled />
      </div>
    )
  }

  if (state.kind === 'cleared') {
    return (
      <div className="flex flex-col gap-1.5">
        <SecretInput
          label={label}
          multiline={multiline}
          value=""
          placeholder="Will be removed on save"
          disabled
          masked={false}
          onChange={() => {}}
        />
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
          <span className="text-destructive">Saved {noun} will be removed on save.</span>
          <button
            type="button"
            className={linkButtonClass}
            disabled={disabled}
            onClick={() => dispatch({ type: 'restore' })}
          >
            Keep it
          </button>
        </div>
      </div>
    )
  }

  const typed = state.kind === 'replaced'
  const saved = state.kind === 'saved'
  const revealed = saved ? revealer.value : null
  const hasStoredValue = saved || (typed && state.previous.kind === 'saved')
  const canReveal = state.kind === 'saved' && state.revealable && binding.reveal !== undefined

  function toggleReveal() {
    if (revealed === null) void revealer.show()
    else revealer.hide()
  }

  function edit(value: string) {
    hide()
    dispatch({ type: 'edit', value })
  }

  return (
    <div className="flex flex-col gap-1.5">
      <SecretInput
        label={label}
        multiline={multiline}
        value={typed ? state.value : (revealed ?? '')}
        placeholder={saved ? 'Saved' : placeholder}
        disabled={disabled}
        invalid={invalid}
        masked={!multiline && !visible && revealed === null}
        onChange={edit}
        trailing={
          !multiline && !saved ? (
            <button
              type="button"
              aria-label={visible ? `Hide ${noun}` : `Show ${noun}`}
              className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
              onClick={() => setVisible((current) => !current)}
            >
              <Icon name={visible ? 'eye-off' : 'eye'} size={20} className="size-4" />
            </button>
          ) : null
        }
      />
      {hasStoredValue || canReveal ? (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
          {canReveal ? (
            <button
              type="button"
              className={linkButtonClass}
              disabled={disabled || revealer.pending}
              onClick={toggleReveal}
            >
              {revealed === null ? `Show saved ${noun}` : `Hide saved ${noun}`}
            </button>
          ) : null}
          {hasStoredValue ? (
            <button
              type="button"
              className={`${linkButtonClass} text-destructive hover:text-destructive`}
              disabled={disabled}
              onClick={() => dispatch({ type: 'clear' })}
            >
              Remove saved {noun}
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function SecretInput({
  label,
  multiline,
  value,
  placeholder,
  disabled,
  invalid,
  masked,
  onChange,
  trailing,
}: {
  label: string
  multiline: boolean
  value: string
  placeholder?: string
  disabled: boolean
  invalid?: boolean
  masked: boolean
  onChange: (value: string) => void
  trailing?: React.ReactNode
}) {
  if (multiline) {
    return (
      <Textarea
        aria-label={label}
        className="min-h-24 font-mono text-xs"
        spellCheck={false}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        aria-invalid={invalid ? true : undefined}
        onChange={(event) => onChange(event.target.value)}
      />
    )
  }
  return (
    <div className="relative">
      <Input
        type={masked ? 'password' : 'text'}
        aria-label={label}
        autoComplete="off"
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        aria-invalid={invalid ? true : undefined}
        className="pe-9"
        onChange={(event) => onChange(event.target.value)}
      />
      {trailing}
    </div>
  )
}
