import { Cancel01Icon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { cn } from '#/lib/utils'
import { Input } from '#/components/ui/input'

type SearchInputProps = {
  value: string
  onValueChange: (value: string) => void
  onClear: () => void
  placeholder: string
  className?: string
}

export function SearchInput({ value, onValueChange, onClear, placeholder, className = 'max-w-md' }: SearchInputProps) {
  return (
    <div className={cn('relative', className)}>
      <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        value={value}
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
        className="pe-9 ps-9"
      />
      {value ? (
        <button
          type="button"
          aria-label="Clear search"
          className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
          onClick={onClear}
        >
          <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-4" />
        </button>
      ) : null}
    </div>
  )
}
