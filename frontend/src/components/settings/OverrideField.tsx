import { Button } from '#/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldTitle,
} from '#/components/ui/field'
import { Tooltip, TooltipContent, TooltipTrigger } from '#/components/ui/tooltip'
import { Icon } from '#/lib/icons'

/** A single runtime-policy field that can inherit the instance default or be overridden. */
export function OverrideField({
  label,
  description,
  overridden,
  disabled,
  onReset,
  limitText,
  limitDescription,
  error,
  children,
}: {
  label: string
  description: string
  overridden: boolean
  disabled?: boolean
  onReset: () => void
  limitText: string
  limitDescription: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <Field data-invalid={Boolean(error)} className="gap-3 py-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <FieldContent className="min-w-0">
          <FieldTitle>{label}</FieldTitle>
          <FieldDescription>{description}</FieldDescription>
        </FieldContent>
        {overridden ? (
          <Button type="button" variant="ghost" size="xs" disabled={disabled} onClick={onReset}>
            Use instance setting
          </Button>
        ) : null}
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <div className="min-w-32 flex-1">{children}</div>
          <span aria-hidden="true" className="text-muted-foreground">
            /
          </span>
          <span className="whitespace-nowrap font-medium text-foreground">{limitText}</span>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`About the ${label} instance setting`}
                />
              }
            >
              <Icon name="information-circle" />
            </TooltipTrigger>
            <TooltipContent>{limitDescription}</TooltipContent>
          </Tooltip>
        </div>
        <FieldError>{error}</FieldError>
      </div>
    </Field>
  )
}
