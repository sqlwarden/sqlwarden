import { useId } from 'react'
import { Input } from '#/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'

interface UnitOption<Unit extends string> {
  value: Unit
  label: string
}

/** Paired numeric amount + unit selector, e.g. "500 MB" or "30 minutes". */
export function UnitInputField<Unit extends string>({
  amount,
  unit,
  options,
  disabled,
  min = 0,
  max,
  label,
  error,
  onAmountChange,
  onUnitChange,
}: {
  amount: number
  unit: Unit
  options: UnitOption<Unit>[]
  disabled?: boolean
  min?: number
  max?: number
  label: string
  error?: boolean
  onAmountChange: (amount: number) => void
  onUnitChange: (unit: Unit) => void
}) {
  const generatedId = useId()
  const amountId = `${generatedId}-amount`
  const unitId = `${generatedId}-unit`
  const selectedLabel = options.find((option) => option.value === unit)?.label ?? unit

  return (
    <div className="flex gap-2">
      <Input
        type="number"
        id={amountId}
        aria-label={label}
        aria-invalid={error || undefined}
        min={min}
        max={max}
        step="any"
        value={Number.isFinite(amount) ? amount : ''}
        disabled={disabled}
        onChange={(event) => {
          const next = event.target.valueAsNumber
          onAmountChange(Number.isFinite(next) ? next : 0)
        }}
        className="flex-1"
      />
      <Select
        value={unit}
        onValueChange={(value) => onUnitChange(value as Unit)}
        disabled={disabled}
      >
        <SelectTrigger id={unitId} aria-label={`${label} unit`} className="w-28">
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
