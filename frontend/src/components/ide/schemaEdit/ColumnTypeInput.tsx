import { useId, useMemo, useState } from 'react'
import { SearchableCombobox } from '#/components/SearchableCombobox'
import { Input } from '#/components/ui/input'
import { Field, FieldError, FieldLabel } from '#/components/ui/field'
import type { ParameterizedColumnType } from '#/lib/api/types'
import { canonicalColumnType } from './columnTypes'

const CUSTOM_VALUE = '__custom_column_type__'

type TypeOption = { value: string; label: string }

export function ColumnTypeInput({
  value,
  onChange,
  columnTypes,
  rules = [],
  allowCustomTypes = false,
  disabled,
  id,
  label = 'Column type',
}: {
  value: string
  onChange: (value: string) => void
  columnTypes: string[]
  rules?: ParameterizedColumnType[]
  allowCustomTypes?: boolean
  disabled?: boolean
  id?: string
  label?: string
}) {
  const customId = useId()
  const descriptionId = useId()
  const fixed = columnTypes.find((type) => type.toUpperCase() === value.trim().toUpperCase())
  const showCustomOption = rules.length > 0 || allowCustomTypes
  const [custom, setCustom] = useState(
    () =>
      showCustomOption &&
      !fixed &&
      canonicalColumnType(value, columnTypes, rules, allowCustomTypes) !== null,
  )
  // The column's starting type may fall outside columnTypes (e.g. a MySQL
  // "smallint(5) unsigned" the closed list doesn't spell out). Pin it for the
  // life of this input so switching to another type and back doesn't strand the
  // user without a way to restore the original value.
  const [originalValue] = useState(value)

  const options = useMemo<TypeOption[]>(() => {
    const list = columnTypes.map((type) => ({ value: type, label: type }))
    if (
      originalValue &&
      !columnTypes.some((type) => type.toUpperCase() === originalValue.toUpperCase())
    ) {
      list.push({ value: originalValue, label: originalValue })
    }
    if (showCustomOption) list.push({ value: CUSTOM_VALUE, label: 'Custom type…' })
    return list
  }, [columnTypes, originalValue, showCustomOption])

  const selected = custom
    ? (options.find((option) => option.value === CUSTOM_VALUE) ?? null)
    : (options.find((option) => option.value.toUpperCase() === value.trim().toUpperCase()) ?? null)

  const valid = canonicalColumnType(value, columnTypes, rules, allowCustomTypes) !== null
  const matchingRule = rules.find(
    (rule) => rule.name.toUpperCase() === value.split('(')[0].trim().toUpperCase(),
  )

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <SearchableCombobox
        id={id}
        ariaLabel={label}
        placeholder="Select a type"
        searchPlaceholder="Filter types..."
        items={options}
        value={selected}
        onValueChange={(option) => {
          if (!option) return
          setCustom(option.value === CUSTOM_VALUE)
          if (option.value !== CUSTOM_VALUE) onChange(option.value)
        }}
        emptyMessage="No matching type."
        disabled={disabled}
      />
      {custom && (
        <Field data-invalid={!valid} data-disabled={disabled}>
          <FieldLabel htmlFor={customId}>Custom column type</FieldLabel>
          <Input
            id={customId}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            disabled={disabled}
            aria-invalid={!valid}
            aria-describedby={descriptionId}
            autoComplete="off"
          />
          {!valid && (
            <FieldError id={descriptionId}>
              {matchingRule
                ? matchingRule.parameters
                    .map(
                      (parameter) =>
                        `${parameter.name}: ${parameter.min} to ${parameter.max}${parameter.optional ? ' (optional)' : ''}`,
                    )
                    .join('; ')
                : 'Enter a supported data type.'}
            </FieldError>
          )}
        </Field>
      )}
    </div>
  )
}
