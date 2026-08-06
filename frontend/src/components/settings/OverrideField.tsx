import { Checkbox } from '#/components/ui/checkbox'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldTitle,
} from '#/components/ui/field'

/** A single runtime-policy field that can inherit the instance default or be overridden. */
export function OverrideField({
  label,
  description,
  overridden,
  disabled,
  onOverrideChange,
  effectiveText,
  constraintText,
  error,
  children,
}: {
  label: string
  description: string
  overridden: boolean
  disabled?: boolean
  onOverrideChange: (overridden: boolean) => void
  effectiveText: string
  constraintText?: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <Field data-invalid={Boolean(error)} className="gap-3 rounded-md border border-border p-4">
      <div className="flex items-start justify-between gap-4">
        <FieldContent>
          <FieldTitle>{label}</FieldTitle>
          <FieldDescription>{description}</FieldDescription>
        </FieldContent>
        <div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
          <Checkbox
            aria-label={`Override ${label}`}
            checked={overridden}
            disabled={disabled}
            onCheckedChange={(checked) => onOverrideChange(checked === true)}
          />
          <span>Override</span>
        </div>
      </div>
      {overridden ? (
        <div className="flex flex-col gap-1.5">
          {children}
          <FieldError>{error}</FieldError>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          Inherits instance default: <span className="text-foreground">{effectiveText}</span>
        </p>
      )}
      {constraintText ? <p className="text-xs text-muted-foreground">{constraintText}</p> : null}
    </Field>
  )
}
