import { cn } from '#/lib/utils'

export const SQLWARDEN_SOURCE_URL = 'https://github.com/sqlwarden/sqlwarden'

export function LegalNotice({ className }: { className?: string }) {
  return (
    <p className={cn('text-xs text-muted-foreground', className)}>
      SQLWarden Community is licensed under AGPLv3.{' '}
      <a
        className="cursor-pointer underline underline-offset-4 hover:text-foreground"
        href={SQLWARDEN_SOURCE_URL}
        target="_blank"
        rel="noreferrer"
      >
        Source code
      </a>
    </p>
  )
}
