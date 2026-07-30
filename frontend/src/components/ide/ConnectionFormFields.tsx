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
  return (
    <Input
      type={field.type === 'number' ? 'number' : field.type === 'password' ? 'password' : 'text'}
      value={value}
      placeholder={field.placeholder}
      disabled={disabled}
      aria-invalid={invalid ? true : undefined}
      onChange={(e) => onChange(field.key, e.target.value)}
    />
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
  children,
}: {
  label: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
