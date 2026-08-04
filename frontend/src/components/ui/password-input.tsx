import * as React from 'react'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils.ts'
import { Input } from './input'

function PasswordInput({ className, ...props }: Omit<React.ComponentProps<typeof Input>, 'type'>) {
  const [visible, setVisible] = React.useState(false)

  return (
    <div className="relative">
      <Input type={visible ? 'text' : 'password'} className={cn('pr-7', className)} {...props} />
      <button
        type="button"
        tabIndex={-1}
        onClick={() => setVisible((current) => !current)}
        className="absolute inset-y-0 right-0 flex w-7 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
        aria-label={visible ? 'Hide password' : 'Show password'}
      >
        <Icon name={visible ? 'eye-off' : 'eye'} size={14} />
      </button>
    </div>
  )
}

export { PasswordInput }
