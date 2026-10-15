import type { JSX } from 'react'

import { Checkbox } from '#/components/ui/checkbox'
import { Input } from '#/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Textarea } from '#/components/ui/textarea'
import { cn } from '#/lib/utils'
import { FormField } from './ConnectionFormFields'
import { SecretField } from './SecretField'
import type { BindSecret } from './useSecretFields'

export type SshAuthMethod = 'password' | 'private_key'

export interface SshFormState {
  enabled: boolean
  host: string
  port: string
  user: string
  authMethod: SshAuthMethod
  knownHostsEntry: string
  fingerprint: string
  insecureSkipHostKey: boolean
}

export const emptySshState: SshFormState = {
  enabled: false,
  host: '',
  port: '22',
  user: '',
  authMethod: 'password',
  knownHostsEntry: '',
  fingerprint: '',
  insecureSkipHostKey: true,
}

const AUTH_METHOD_LABELS: Record<SshAuthMethod, string> = {
  password: 'Password',
  private_key: 'Private key',
}

export function ConnectionSshFields({
  value,
  disabled,
  bindSecret,
  onChange,
}: {
  value: SshFormState
  disabled?: boolean
  bindSecret: BindSecret
  onChange: (next: SshFormState) => void
}): JSX.Element {
  const set = <K extends keyof SshFormState>(key: K, v: SshFormState[K]) =>
    onChange({ ...value, [key]: v })

  // Fields stay mounted when the tunnel is off so toggling it never discards
  // what the user typed; they are only disabled.
  const fieldsDisabled = disabled || !value.enabled

  return (
    <div className="flex flex-col gap-3 overflow-x-clip">
      <label className="flex cursor-pointer items-center gap-3 py-1">
        <Checkbox
          aria-label="Use SSH tunnel"
          checked={value.enabled}
          disabled={disabled}
          onCheckedChange={(checked) => set('enabled', checked === true)}
        />
        <span className="text-xs font-medium text-foreground">Use SSH tunnel</span>
      </label>

      <FormField label="SSH host" disabled={fieldsDisabled}>
        <Input
          aria-label="SSH host"
          value={value.host}
          placeholder="bastion.internal"
          disabled={fieldsDisabled}
          onChange={(e) => set('host', e.target.value)}
        />
      </FormField>

      <FormField label="SSH port" disabled={fieldsDisabled}>
        <Input
          aria-label="SSH port"
          inputMode="numeric"
          value={value.port}
          placeholder="22"
          disabled={fieldsDisabled}
          onChange={(e) => set('port', e.target.value)}
        />
      </FormField>

      <FormField label="SSH user" disabled={fieldsDisabled}>
        <Input
          aria-label="SSH user"
          value={value.user}
          placeholder="jump"
          disabled={fieldsDisabled}
          onChange={(e) => set('user', e.target.value)}
        />
      </FormField>

      <FormField label="Authentication" disabled={fieldsDisabled}>
        <Select
          value={value.authMethod}
          onValueChange={(v) => v && set('authMethod', v as SshAuthMethod)}
          disabled={fieldsDisabled}
        >
          <SelectTrigger className="w-full" aria-label="Authentication">
            <SelectValue>{AUTH_METHOD_LABELS[value.authMethod]}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="password">Password</SelectItem>
            <SelectItem value="private_key">Private key</SelectItem>
          </SelectContent>
        </Select>
      </FormField>

      {value.authMethod === 'password' ? (
        <FormField label="SSH password" disabled={fieldsDisabled}>
          <SecretField
            binding={bindSecret('ssh_password')}
            noun="password"
            label="SSH password"
            disabled={fieldsDisabled}
          />
        </FormField>
      ) : (
        <>
          <FormField label="Private key (PEM)" disabled={fieldsDisabled}>
            <SecretField
              binding={bindSecret('ssh_private_key')}
              noun="key"
              label="Private key (PEM)"
              multiline
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              disabled={fieldsDisabled}
            />
          </FormField>

          <FormField label="Key passphrase (optional)" disabled={fieldsDisabled}>
            <SecretField
              binding={bindSecret('ssh_passphrase')}
              noun="passphrase"
              label="Key passphrase (optional)"
              disabled={fieldsDisabled}
            />
          </FormField>
        </>
      )}

      <label className="flex cursor-pointer items-center gap-3 py-1">
        <Checkbox
          aria-label="Do not verify host key"
          checked={value.insecureSkipHostKey}
          disabled={fieldsDisabled}
          onCheckedChange={(checked) => set('insecureSkipHostKey', checked === true)}
        />
        <span className={cn('text-xs font-medium text-foreground', fieldsDisabled && 'opacity-50')}>
          Do not verify host key
        </span>
      </label>

      {!value.insecureSkipHostKey ? (
        <>
          <FormField label="known_hosts entry" disabled={fieldsDisabled}>
            <Textarea
              aria-label="known_hosts entry"
              className="min-h-16 font-mono text-xs"
              spellCheck={false}
              value={value.knownHostsEntry}
              placeholder="bastion.internal ssh-ed25519 AAAAC3Nz..."
              disabled={fieldsDisabled}
              onChange={(e) => set('knownHostsEntry', e.target.value)}
            />
          </FormField>

          <FormField label="or SHA256 fingerprint" disabled={fieldsDisabled}>
            <Input
              aria-label="or SHA256 fingerprint"
              value={value.fingerprint}
              placeholder="SHA256:..."
              disabled={fieldsDisabled}
              onChange={(e) => set('fingerprint', e.target.value)}
            />
          </FormField>
        </>
      ) : null}
    </div>
  )
}
