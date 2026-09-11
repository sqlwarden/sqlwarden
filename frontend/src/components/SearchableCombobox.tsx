import { useEffect, useRef } from 'react'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import {
  Combobox,
  ComboboxEmpty,
  ComboboxIcon,
  ComboboxInput,
  ComboboxInputGroup,
  ComboboxItem,
  ComboboxItemIndicator,
  ComboboxList,
  ComboboxPopup,
  ComboboxTrigger,
  ComboboxValue,
} from '#/components/ui/combobox'
import { Skeleton } from '#/components/ui/skeleton'

export interface SearchableComboboxItem {
  value: string
  label: string
  sublabel?: string
}

interface SearchableComboboxProps<T extends SearchableComboboxItem> {
  id?: string
  ariaLabel?: string
  placeholder: string
  searchPlaceholder?: string
  items: T[]
  /** The currently selected item. Only `value`/`label` are read, so callers that store the
   *  selection as an id/label pair (rather than the full list item) can pass that directly. */
  value: SearchableComboboxItem | null
  onValueChange: (item: T | null) => void
  isLoading?: boolean
  disabled?: boolean
  emptyMessage?: string
  /** Debounces and forwards the search query. Providing this switches the combobox into
   *  remote-search mode: `items` is treated as already server-filtered and the combobox's
   *  built-in client-side filtering is disabled. */
  onSearchChange?: (query: string) => void
  className?: string
}

const SEARCH_DEBOUNCE_MS = 300

/** Canonical searchable/filterable dropdown used across the app (role assignment, column type
 *  pickers, etc.) so every instance shares the same trigger, popup, and item styling. */
export function SearchableCombobox<T extends SearchableComboboxItem>({
  id,
  ariaLabel,
  placeholder,
  searchPlaceholder = 'Search…',
  items,
  value,
  onValueChange,
  isLoading = false,
  disabled,
  emptyMessage,
  onSearchChange,
  className,
}: SearchableComboboxProps<T>) {
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(
    () => () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    },
    [],
  )

  function handleInputValueChange(query: string) {
    if (!onSearchChange) return
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => onSearchChange(query), SEARCH_DEBOUNCE_MS)
  }

  return (
    <Combobox
      items={items}
      value={value as T | null}
      onValueChange={(item: T | null) => onValueChange(item)}
      onInputValueChange={handleInputValueChange}
      itemToStringLabel={(item: T) => item.label}
      isItemEqualToValue={(a: T, b: T) => a.value === b.value}
      filter={onSearchChange ? null : undefined}
      disabled={disabled}
    >
      <ComboboxTrigger
        id={id}
        aria-label={ariaLabel ?? placeholder}
        className={cn(
          'h-7 w-full justify-between gap-1.5 border border-input bg-input/20 px-2 py-1.5 hover:bg-input/30 focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30',
          className,
        )}
      >
        <span className="min-w-0 flex-1 truncate text-left">
          <ComboboxValue placeholder={placeholder} />
        </span>
        <ComboboxIcon />
      </ComboboxTrigger>
      <ComboboxPopup>
        <ComboboxInputGroup>
          <Icon
            name="search-01"
            size={12}
            className="pointer-events-none absolute start-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground"
          />
          <ComboboxInput placeholder={searchPlaceholder} className="ps-7" />
        </ComboboxInputGroup>
        {isLoading ? (
          <div className="flex flex-col gap-1 p-1">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-8 w-full rounded-md" />
            ))}
          </div>
        ) : (
          <>
            <ComboboxList>
              {(item: T) => (
                <ComboboxItem
                  key={item.value}
                  value={item}
                  className={item.sublabel ? 'h-auto min-h-8 py-1.5' : undefined}
                >
                  <span className="flex min-w-0 flex-1 flex-col items-start">
                    <span className="w-full truncate">{item.label}</span>
                    {item.sublabel ? (
                      <span className="w-full truncate text-[10px] text-muted-foreground">
                        {item.sublabel}
                      </span>
                    ) : null}
                  </span>
                  <ComboboxItemIndicator />
                </ComboboxItem>
              )}
            </ComboboxList>
            <ComboboxEmpty>{emptyMessage ?? 'No matches found.'}</ComboboxEmpty>
          </>
        )}
      </ComboboxPopup>
    </Combobox>
  )
}
