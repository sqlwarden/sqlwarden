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
import { FormField, PasswordInput, StoredSecretRow } from './ConnectionFormFields'

export type SshAuthMethod = 'password' | 'private_key'

export interface SshFormState {
  enabled: boolean
  host: string
  port: string
  user: string
  authMethod: SshAuthMethod
  password: string
  privateKeyPem: string
  passphrase: string
  knownHostsEntry: string
  fingerprint: string
  insecureSkipHostKey: boolean
  /** Edit mode: a password is already stored server-side. */
  passwordSet: boolean
  /** Edit mode: a private key is already stored server-side. */
  privateKeySet: boolean
  /** Edit mode: drop the stored password on save. */
  clearPassword: boolean
  /** Edit mode: drop the stored private key (and its passphrase) on save. */
  clearPrivateKey: boolean
}

export const emptySshState: SshFormState = {
  enabled: false,
  host: '',
  port: '22',
  user: '',
  authMethod: 'password',
  password: '',
  privateKeyPem: '',
  passphrase: '',
  knownHostsEntry: '',
  fingerprint: '',
  insecureSkipHostKey: true,
  passwordSet: false,
  privateKeySet: false,
  clearPassword: false,
  clearPrivateKey: false,
}

const AUTH_METHOD_LABELS: Record<SshAuthMethod, string> = {
  password: 'Password',
  private_key: 'Private key',
}

export function ConnectionSshFields({
  value,
  disabled,
  onChange,
}: {
  value: SshFormState
  disabled?: boolean
  onChange: (next: SshFormState) => void
}): JSX.Element {
  const set = <K extends keyof SshFormState>(key: K, v: SshFormState[K]) =>
    onChange({ ...value, [key]: v })
  const patch = (next: Partial<SshFormState>) => onChange({ ...value, ...next })

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
          <PasswordInput
            aria-label="SSH password"
            value={value.password}
            placeholder={
              value.clearPassword
                ? 'Will be removed on save'
                : value.passwordSet
                  ? 'Stored — leave blank to keep the existing password'
                  : undefined
            }
            disabled={fieldsDisabled || value.clearPassword}
            onChange={(next) => set('password', next)}
          />
          {value.passwordSet ? (
            <StoredSecretRow
              noun="password"
              cleared={value.clearPassword}
              disabled={fieldsDisabled}
              onClear={() => set('clearPassword', true)}
              onRestore={() => set('clearPassword', false)}
            />
          ) : null}
        </FormField>
      ) : (
        <>
          <FormField label="Private key (PEM)" disabled={fieldsDisabled}>
            <Textarea
              aria-label="Private key (PEM)"
              className="min-h-24 font-mono text-xs"
              spellCheck={false}
              value={value.privateKeyPem}
              placeholder={
                value.clearPrivateKey
                  ? 'Will be removed on save'
                  : value.privateKeySet
                    ? 'Stored — leave blank to keep the existing key'
                    : '-----BEGIN OPENSSH PRIVATE KEY-----'
              }
              disabled={fieldsDisabled || value.clearPrivateKey}
              onChange={(e) => set('privateKeyPem', e.target.value)}
            />
            {value.privateKeySet ? (
              <StoredSecretRow
                noun="key"
                cleared={value.clearPrivateKey}
                disabled={fieldsDisabled}
                onClear={() => patch({ clearPrivateKey: true })}
                onRestore={() => patch({ clearPrivateKey: false })}
              />
            ) : null}
          </FormField>

          <FormField label="Key passphrase (optional)" disabled={fieldsDisabled}>
            <PasswordInput
              aria-label="Key passphrase (optional)"
              value={value.passphrase}
              disabled={fieldsDisabled || value.clearPrivateKey}
              onChange={(next) => set('passphrase', next)}
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
