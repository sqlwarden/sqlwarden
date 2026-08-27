import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { Input } from '#/components/ui/input'

type SearchInputProps = {
  value: string
  onValueChange: (value: string) => void
  onClear: () => void
  placeholder: string
  className?: string
  /** 'sm' renders the compact icon/input/clear-button sizing used by dense
   *  IDE sidebar filters; 'default' is the standard page-toolbar search box. */
  size?: 'default' | 'sm'
  /** 'muted' fills the input with a subtle background instead of a border,
   *  which reads better inside an already-bordered sidebar pane. */
  variant?: 'default' | 'muted'
}

export function SearchInput({
  value,
  onValueChange,
  onClear,
  placeholder,
  className = 'max-w-md',
  size = 'default',
  variant = 'default',
}: SearchInputProps) {
  const isSm = size === 'sm'
  const isMuted = variant === 'muted'

  return (
    <div className={cn('relative', className)}>
      <Icon
        name="search-01"
        size={isSm ? 12 : 20}
        className={cn(
          'pointer-events-none absolute top-1/2 -translate-y-1/2 text-muted-foreground',
          isSm ? 'start-2 size-3' : 'start-3 size-4',
        )}
      />
      <Input
        value={value}
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
        className={cn(
          isSm ? 'h-7 pe-7 ps-7 text-xs' : 'pe-9 ps-9',
          isMuted &&
            'border-transparent bg-muted/60 focus-visible:bg-background dark:bg-muted/40 dark:focus-visible:bg-input/30',
        )}
      />
      {value ? (
        <button
          type="button"
          aria-label="Clear search"
          className={cn(
            'absolute top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground',
            isSm ? 'end-1.5' : 'end-3',
            isMuted && 'rounded hover:bg-muted',
          )}
          onClick={onClear}
        >
          <Icon name="cancel-01" size={isSm ? 10 : 20} className={isSm ? 'size-2.5' : 'size-4'} />
        </button>
      ) : null}
    </div>
  )
}
