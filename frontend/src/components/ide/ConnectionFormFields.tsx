import { useState } from 'react'
import { Icon } from '#/lib/icons'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import type { ScopePath } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import type { DriverDef, FieldDef } from './connection-drivers/index'
import { scopeSegmentName, type ScopeDiscovery } from './useConnectionForm'

// Fields, sections, and layout all come from the driver definition, so adding a
// new database means writing one driver def — no form changes.

const SPAN_CLASS: Record<NonNullable<FieldDef['span']>, string> = {
  full: 'col-span-6',
  wide: 'col-span-4',
  half: 'col-span-3',
  compact: 'col-span-2',
}

const NO_DEFAULT_SCOPE = '__sqlwarden_no_default_scope__'

export function DriverFields({
  driver,
  values,
  errors,
  disabled,
  onChange,
  scopeDiscovery,
  defaultScope,
  onDatabaseChange,
  onSchemaChange,
}: {
  driver: DriverDef
  values: Record<string, string>
  errors: Record<string, string>
  disabled: boolean
  onChange: (key: string, value: string) => void
  scopeDiscovery?: ScopeDiscovery
  defaultScope?: ScopePath
  onDatabaseChange?: (database: string) => void
  onSchemaChange?: (schema: string) => void
}) {
  const nodes: React.ReactNode[] = []
  let lastSection: string | undefined

  for (const field of driver.fields) {
    if (field.section && field.section !== lastSection) {
      lastSection = field.section
      nodes.push(
        <div key={`section:${field.section}`} className="col-span-6">
          <SectionDivider label={field.section} />
        </div>,
      )
    }
    if (
      field.key === 'database' &&
      scopeDiscovery &&
      onDatabaseChange &&
      onSchemaChange &&
      discoveredDatabases(scopeDiscovery).length
    ) {
      nodes.push(
        <ScopeFields
          key="database"
          discovery={scopeDiscovery}
          defaultScope={defaultScope ?? []}
          disabled={disabled}
          onDatabaseChange={onDatabaseChange}
          onSchemaChange={onSchemaChange}
        />,
      )
      continue
    }
    nodes.push(
      <div key={field.key} className={SPAN_CLASS[field.span ?? 'full']}>
        <FormField label={field.label} error={errors[field.key]}>
          <DriverFieldControl
            field={field}
            value={values[field.key] ?? ''}
            invalid={Boolean(errors[field.key])}
            disabled={disabled}
            onChange={onChange}
          />
        </FormField>
      </div>,
    )
  }

  return <>{nodes}</>
}

function ScopeFields({
  discovery,
  defaultScope,
  disabled,
  onDatabaseChange,
  onSchemaChange,
}: {
  discovery: ScopeDiscovery
  defaultScope: ScopePath
  disabled: boolean
  onDatabaseChange: (database: string) => void
  onSchemaChange: (schema: string) => void
}) {
  const database = scopeSegmentName(defaultScope, 'database') ?? ''
  const schema = scopeSegmentName(defaultScope, 'schema') ?? ''
  const databases = discoveredDatabases(discovery)
  const schemas = uniqueNames(
    discovery.scopes
      .filter((scope) => scopeSegmentName(scope, 'database') === database)
      .map((scope) => scopeSegmentName(scope, 'schema')),
  )

  return (
    <>
      <div className={cn('col-span-6', schemas.length > 0 && 'sm:col-span-3')}>
        <FormField label="Database">
          <Select
            value={database || NO_DEFAULT_SCOPE}
            onValueChange={(value) => {
              if (value) onDatabaseChange(value === NO_DEFAULT_SCOPE ? '' : value)
            }}
            disabled={disabled}
          >
            <SelectTrigger className="w-full" aria-label="Default database">
              <SelectValue>{database || 'No default database'}</SelectValue>
            </SelectTrigger>
            <SelectContent className="min-w-[180px]">
              <SelectGroup>
                <SelectItem value={NO_DEFAULT_SCOPE}>No default database</SelectItem>
                {databases.map((name) => (
                  <SelectItem key={name} value={name}>
                    {name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </FormField>
      </div>
      {schemas.length > 0 ? (
        <div className="col-span-6 sm:col-span-3">
          <FormField label="Schema">
            <Select
              value={schema || NO_DEFAULT_SCOPE}
              onValueChange={(value) => {
                if (value) onSchemaChange(value === NO_DEFAULT_SCOPE ? '' : value)
              }}
              disabled={disabled}
            >
              <SelectTrigger className="w-full" aria-label="Default schema">
                <SelectValue>{schema || 'Use database default'}</SelectValue>
              </SelectTrigger>
              <SelectContent className="min-w-[180px]">
                <SelectGroup>
                  <SelectItem value={NO_DEFAULT_SCOPE}>Use database default</SelectItem>
                  {schemas.map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
        </div>
      ) : null}
    </>
  )
}

function discoveredDatabases(discovery: ScopeDiscovery): string[] {
  return uniqueNames([
    ...discovery.scopes
      .filter((scope) => scope.length === 1)
      .map((scope) => scopeSegmentName(scope, 'database')),
    scopeSegmentName(discovery.current, 'database'),
  ])
}

function uniqueNames(names: (string | undefined)[]): string[] {
  return [...new Set(names.filter((name): name is string => Boolean(name)))].sort((a, b) =>
    a.localeCompare(b),
  )
}

function DriverFieldControl({
  field,
  value,
  invalid,
  disabled,
  onChange,
}: {
  field: FieldDef
  value: string
  invalid: boolean
  disabled: boolean
  onChange: (key: string, value: string) => void
}) {
  if (field.type === 'select') {
    const selectedLabel =
      field.options?.find((o) => o.value === (value || field.default))?.label ?? value
    return (
      <Select
        value={value || field.default || ''}
        onValueChange={(v) => {
          if (v) onChange(field.key, v)
        }}
        disabled={disabled}
      >
        <SelectTrigger className="w-full">
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent className="min-w-[120px]">
          {(field.options ?? []).map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }
  if (field.type === 'password') {
    return (
      <PasswordFieldControl
        field={field}
        value={value}
        invalid={invalid}
        disabled={disabled}
        onChange={onChange}
      />
    )
  }
  return (
    <Input
      type={field.type === 'number' ? 'number' : 'text'}
      value={value}
      placeholder={field.placeholder}
      disabled={disabled}
      aria-invalid={invalid ? true : undefined}
      onChange={(e) => onChange(field.key, e.target.value)}
    />
  )
}

function PasswordFieldControl({
  field,
  value,
  invalid,
  disabled,
  onChange,
}: {
  field: FieldDef
  value: string
  invalid: boolean
  disabled: boolean
  onChange: (key: string, value: string) => void
}) {
  return (
    <PasswordInput
      value={value}
      placeholder={field.placeholder}
      invalid={invalid}
      disabled={disabled}
      onChange={(next) => onChange(field.key, next)}
    />
  )
}

export function PasswordInput({
  value,
  placeholder,
  invalid,
  disabled,
  onChange,
  'aria-label': ariaLabel,
}: {
  value: string
  placeholder?: string
  invalid?: boolean
  disabled?: boolean
  onChange: (value: string) => void
  'aria-label'?: string
}) {
  const [visible, setVisible] = useState(false)

  return (
    <div className="relative">
      <Input
        type={visible ? 'text' : 'password'}
        aria-label={ariaLabel}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        aria-invalid={invalid ? true : undefined}
        className="pe-9"
        onChange={(e) => onChange(e.target.value)}
      />
      <button
        type="button"
        aria-label={visible ? 'Hide password' : 'Show password'}
        className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
        onClick={() => setVisible((current) => !current)}
      >
        <Icon name={visible ? 'eye-off' : 'eye'} size={20} className="size-4" />
      </button>
    </div>
  )
}

/** Edit-form control for a secret that is already stored server-side: the input
 *  stays visible so a replacement can be typed, and this row toggles an explicit
 *  "drop the stored value on save" request. */
export function StoredSecretRow({
  noun,
  cleared,
  disabled,
  onClear,
  onRestore,
}: {
  noun: string
  cleared: boolean
  disabled?: boolean
  onClear: () => void
  onRestore: () => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
      {cleared ? (
        <>
          <span className="text-destructive">Stored {noun} will be removed on save.</span>
          <button
            type="button"
            className="font-medium underline underline-offset-2 hover:text-foreground disabled:opacity-50"
            disabled={disabled}
            onClick={onRestore}
          >
            Keep it
          </button>
        </>
      ) : (
        <>
          <span>A {noun} is stored. Leave blank to keep it.</span>
          <button
            type="button"
            className="font-medium text-destructive underline underline-offset-2 hover:opacity-80 disabled:opacity-50"
            disabled={disabled}
            onClick={onClear}
          >
            Remove stored {noun}
          </button>
        </>
      )}
    </div>
  )
}

export function SectionDivider({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 pt-1">
      <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}

export function FormField({
  label,
  error,
  disabled,
  children,
}: {
  label: string
  error?: string
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <div className="group/field flex flex-col gap-1.5" data-disabled={disabled || undefined}>
      <Label className="group-data-[disabled]/field:opacity-50">{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
