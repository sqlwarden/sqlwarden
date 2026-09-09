import { useId, useState } from 'react'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Input } from '#/components/ui/input'
import { Field, FieldDescription, FieldError, FieldLabel } from '#/components/ui/field'
import type { ParameterizedColumnType } from '#/lib/api/types'
import { canonicalColumnType } from './columnTypes'

export function ColumnTypeInput({
  value,
  onChange,
  columnTypes,
  rules = [],
  disabled,
  id,
  label = 'Column type',
}: {
  value: string
  onChange: (value: string) => void
  columnTypes: string[]
  rules?: ParameterizedColumnType[]
  disabled?: boolean
  id?: string
  label?: string
}) {
  const customId = useId()
  const descriptionId = useId()
  const fixed = columnTypes.find((type) => type.toUpperCase() === value.trim().toUpperCase())
  const [custom, setCustom] = useState(
    () => rules.length > 0 && !fixed && canonicalColumnType(value, columnTypes, rules) !== null,
  )
  const customValue = '__custom_column_type__'
  const items = columnTypes.map((type) => ({ label: type, value: type }))
  // Preserve an existing database type outside the editable palette when the
  // user is changing another property of the column.
  if (!custom && !fixed && value) items.push({ label: value, value })
  if (rules.length > 0) items.push({ label: 'Custom type…', value: customValue })
  const valid = canonicalColumnType(value, columnTypes, rules) !== null
  const matchingRule = rules.find(
    (rule) => rule.name.toUpperCase() === value.split('(')[0].trim().toUpperCase(),
  )
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Select
        items={items}
        value={custom ? customValue : (fixed ?? value)}
        onValueChange={(next) => {
          if (!next) return
          setCustom(next === customValue)
          if (next !== customValue) onChange(next)
        }}
        disabled={disabled}
      >
        <SelectTrigger id={id} aria-label={label} className="w-full min-w-0">
          <SelectValue placeholder="Select a type" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {items.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
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
          {valid ? (
            <FieldDescription id={descriptionId}>
              Enter a type with its parameters.
            </FieldDescription>
          ) : (
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
