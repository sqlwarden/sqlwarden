import { useEffect, useRef, useState } from 'react'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { Label } from '#/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { ScrollArea } from '#/components/ui/scroll-area'
import { Skeleton } from '#/components/ui/skeleton'

export interface ComboboxItem {
  value: string
  label: string
  sublabel?: string
}

interface SearchComboboxFieldProps<T extends ComboboxItem> {
  label: string
  placeholder: string
  searchPlaceholder: string
  selectedValue: string
  selectedLabel: string
  items: T[]
  isLoading: boolean
  error?: string
  disabled: boolean
  emptyMessage?: string
  onChange: (value: string, label: string, item: T) => void
  onSearchChange: (query: string) => void
}

export function SearchComboboxField<T extends ComboboxItem>({
  label,
  placeholder,
  searchPlaceholder,
  selectedValue,
  selectedLabel,
  items,
  isLoading,
  error,
  disabled,
  emptyMessage,
  onChange,
  onSearchChange,
}: SearchComboboxFieldProps<T>) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
  }, [])

  function resetSearch() {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = null
    setSearch('')
    onSearchChange('')
  }

  function handleSearchChange(value: string) {
    setSearch(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => onSearchChange(value), 300)
  }

  function handleSelect(item: T) {
    onChange(item.value, item.label, item)
    setOpen(false)
    resetSearch()
  }

  function handleOpenChange(nextOpen: boolean) {
    setOpen(nextOpen)
    if (!nextOpen) resetSearch()
    else setTimeout(() => inputRef.current?.focus(), 0)
  }

  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger
          disabled={disabled}
          className={cn(
            'flex h-7 w-full items-center justify-between gap-1.5 rounded-md border border-input bg-input/20 px-2 py-1.5 text-xs/relaxed whitespace-nowrap transition-colors outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-50',
            error && 'border-destructive ring-2 ring-destructive/20',
            !selectedValue && 'text-muted-foreground',
          )}
        >
          <span className="truncate">{selectedValue ? selectedLabel : placeholder}</span>
          <svg className="size-3.5 shrink-0 text-muted-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </PopoverTrigger>
        <PopoverContent className="w-(--anchor-width) p-0" align="start" sideOffset={4}>
          <div className="flex items-center gap-2 border-b border-border px-2">
            <Icon name="search-01" size={20} className="size-3.5 shrink-0 text-muted-foreground" />
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(event) => handleSearchChange(event.target.value)}
              placeholder={searchPlaceholder}
              className="h-8 w-full bg-transparent text-xs outline-none placeholder:text-muted-foreground"
            />
            {search ? (
              <button type="button" onClick={resetSearch} className="shrink-0 text-muted-foreground hover:text-foreground">
                <Icon name="cancel-01" size={20} className="size-3" />
              </button>
            ) : null}
          </div>
          <ScrollArea className="max-h-52">
            <div className="flex flex-col p-1">
              {isLoading ? (
                <div className="flex flex-col gap-1 p-1">
                  {Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-7 w-full rounded-md" />)}
                </div>
              ) : items.length === 0 ? (
                <p className="py-5 text-center text-xs text-muted-foreground">
                  {search ? 'No matches found.' : (emptyMessage ?? `No ${label.toLowerCase()}s available.`)}
                </p>
              ) : items.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  onClick={() => handleSelect(item)}
                  className={cn(
                    'flex flex-col items-start rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent hover:text-accent-foreground',
                    item.value === selectedValue && 'bg-accent text-accent-foreground',
                  )}
                >
                  <span className="text-xs font-medium">{item.label}</span>
                  {item.sublabel ? <span className="text-[10px] text-muted-foreground">{item.sublabel}</span> : null}
                </button>
              ))}
            </div>
          </ScrollArea>
        </PopoverContent>
      </Popover>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
