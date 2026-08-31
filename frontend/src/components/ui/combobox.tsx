'use client'

import { Combobox as ComboboxPrimitive } from '@base-ui/react/combobox'

import { cn } from '#/lib/utils.ts'
import { Icon } from '#/lib/icons'

const Combobox = ComboboxPrimitive.Root

/** Renders no element of its own — wrap it in a `<span>` for layout/truncation. */
function ComboboxValue(props: ComboboxPrimitive.Value.Props) {
  return <ComboboxPrimitive.Value {...props} />
}

function ComboboxIcon({ className, ...props }: ComboboxPrimitive.Icon.Props) {
  return (
    <ComboboxPrimitive.Icon
      data-slot="combobox-icon"
      className={cn('pointer-events-none shrink-0 text-muted-foreground', className)}
      {...props}
    >
      <Icon name="unfold-more" size={14} />
    </ComboboxPrimitive.Icon>
  )
}

function ComboboxTrigger({ className, ...props }: ComboboxPrimitive.Trigger.Props) {
  return (
    <ComboboxPrimitive.Trigger
      data-slot="combobox-trigger"
      className={cn(
        'flex items-center rounded-[calc(var(--radius-sm)+2px)] text-xs text-foreground transition-colors hover:bg-sidebar-accent/60',
        className,
      )}
      {...props}
    />
  )
}

function ComboboxInputGroup({ className, ...props }: ComboboxPrimitive.InputGroup.Props) {
  return (
    <ComboboxPrimitive.InputGroup
      data-slot="combobox-input-group"
      className={cn('relative', className)}
      {...props}
    />
  )
}

function ComboboxInput({ className, ...props }: ComboboxPrimitive.Input.Props) {
  return (
    <ComboboxPrimitive.Input
      data-slot="combobox-input"
      className={cn(
        'h-7 w-full rounded-md border border-transparent bg-muted/60 px-7 text-xs outline-none placeholder:text-muted-foreground focus-visible:bg-background dark:bg-muted/40 dark:focus-visible:bg-input/30',
        className,
      )}
      {...props}
    />
  )
}

function ComboboxPopup({
  className,
  align = 'start',
  alignOffset = 0,
  side = 'bottom',
  sideOffset = 4,
  children,
  ...props
}: ComboboxPrimitive.Popup.Props &
  Pick<ComboboxPrimitive.Positioner.Props, 'align' | 'alignOffset' | 'side' | 'sideOffset'>) {
  return (
    <ComboboxPrimitive.Portal>
      <ComboboxPrimitive.Positioner
        align={align}
        alignOffset={alignOffset}
        side={side}
        sideOffset={sideOffset}
        className="isolate z-50"
      >
        <ComboboxPrimitive.Popup
          data-slot="combobox-popup"
          className={cn(
            'z-50 flex max-h-(--available-height) w-(--anchor-width) min-w-64 origin-(--transform-origin) flex-col gap-1.5 overflow-x-hidden overflow-y-auto rounded-lg bg-popover p-1.5 text-xs text-popover-foreground shadow-md ring-1 ring-foreground/10 outline-hidden duration-100 data-[side=bottom]:slide-in-from-top-2 data-[side=inline-end]:slide-in-from-left-2 data-[side=inline-start]:slide-in-from-right-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95',
            className,
          )}
          {...props}
        >
          {children}
        </ComboboxPrimitive.Popup>
      </ComboboxPrimitive.Positioner>
    </ComboboxPrimitive.Portal>
  )
}

function ComboboxList({ className, ...props }: ComboboxPrimitive.List.Props) {
  return (
    <ComboboxPrimitive.List
      data-slot="combobox-list"
      className={cn('flex flex-col gap-0.5 empty:hidden', className)}
      {...props}
    />
  )
}

const ComboboxCollection = ComboboxPrimitive.Collection

function ComboboxItem({ className, children, ...props }: ComboboxPrimitive.Item.Props) {
  return (
    <ComboboxPrimitive.Item
      data-slot="combobox-item"
      className={cn(
        'relative flex h-8 w-full cursor-default items-center gap-2 rounded-md px-2 text-xs outline-hidden select-none data-highlighted:bg-sidebar-accent/60',
        className,
      )}
      {...props}
    >
      {children}
    </ComboboxPrimitive.Item>
  )
}

function ComboboxItemIndicator({ className, ...props }: ComboboxPrimitive.ItemIndicator.Props) {
  return (
    <ComboboxPrimitive.ItemIndicator
      data-slot="combobox-item-indicator"
      className={cn('shrink-0 text-muted-foreground', className)}
      {...props}
    >
      <Icon name="tick-02" size={14} />
    </ComboboxPrimitive.ItemIndicator>
  )
}

function ComboboxEmpty({ className, ...props }: ComboboxPrimitive.Empty.Props) {
  return (
    <ComboboxPrimitive.Empty
      data-slot="combobox-empty"
      className={cn('px-2 py-3 text-center text-muted-foreground empty:hidden', className)}
      {...props}
    />
  )
}

export {
  Combobox,
  ComboboxCollection,
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
}
