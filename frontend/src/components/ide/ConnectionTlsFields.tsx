import { Input } from '#/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Textarea } from '#/components/ui/textarea'
import { FormField } from './ConnectionFormFields'
import type { EngineTlsSpec, TlsMode } from './engines/types'

export interface TlsFormState {
  mode: TlsMode
  serverName: string
  caPem: string
  clientCertPem: string
  clientKeyPem: string
  /** Edit mode: a client key is already stored server-side. */
  clientKeySet: boolean
}

export const emptyTlsState: TlsFormState = {
  mode: 'disable',
  serverName: '',
  caPem: '',
  clientCertPem: '',
  clientKeyPem: '',
  clientKeySet: false,
}

function PemArea({
  label,
  value,
  placeholder,
  disabled,
  onChange,
}: {
  label: string
  value: string
  placeholder?: string
  disabled: boolean
  onChange: (v: string) => void
}) {
  return (
    <FormField label={label} disabled={disabled}>
      <Textarea
        aria-label={label}
        className="min-h-24 font-mono text-xs"
        spellCheck={false}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
      />
    </FormField>
  )
}

export function ConnectionTlsFields({
  spec,
  value,
  disabled,
  onChange,
}: {
  spec: EngineTlsSpec | undefined
  value: TlsFormState
  disabled: boolean
  onChange: (next: TlsFormState) => void
}) {
  if (!spec) return null

  const set = (patch: Partial<TlsFormState>) => onChange({ ...value, ...patch })
  // Fields stay mounted when the mode is 'disable' so switching TLS off never
  // discards what the user typed; they are only disabled.
  const fieldsDisabled = disabled || value.mode === 'disable'
  const modeLabel = spec.modes.find((m) => m.value === value.mode)?.label ?? value.mode

  return (
    <div className="flex flex-col gap-3 overflow-x-clip">
      <FormField label="TLS mode">
        <Select
          value={value.mode}
          onValueChange={(v) => v && set({ mode: v as TlsMode })}
          disabled={disabled}
        >
          <SelectTrigger className="w-full" aria-label="TLS mode">
            <SelectValue>{modeLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {spec.modes.map((m) => (
              <SelectItem key={m.value} value={m.value}>
                {m.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>

      {spec.serverName ? (
        <FormField label="Server name (optional)" disabled={fieldsDisabled}>
          <Input
            aria-label="Server name"
            value={value.serverName}
            placeholder="Overrides the host for certificate checks"
            disabled={fieldsDisabled}
            onChange={(e) => set({ serverName: e.target.value })}
          />
        </FormField>
      ) : null}

      {spec.caBundle ? (
        <PemArea
          label="CA bundle (PEM)"
          value={value.caPem}
          placeholder="-----BEGIN CERTIFICATE-----"
          disabled={fieldsDisabled}
          onChange={(v) => set({ caPem: v })}
        />
      ) : null}

      {spec.clientCert ? (
        <>
          <PemArea
            label="Client certificate (PEM)"
            value={value.clientCertPem}
            placeholder="-----BEGIN CERTIFICATE-----"
            disabled={fieldsDisabled}
            onChange={(v) => set({ clientCertPem: v })}
          />
          <PemArea
            label="Client key (PEM)"
            value={value.clientKeyPem}
            placeholder={
              value.clientKeySet
                ? 'Stored — leave blank to keep the existing key'
                : '-----BEGIN PRIVATE KEY-----'
            }
            disabled={fieldsDisabled}
            onChange={(v) => set({ clientKeyPem: v })}
          />
        </>
      ) : null}
    </div>
  )
}
