import { Button } from '#/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'

const defaultPageSizeOptions = [10, 25, 50, 100]

type PaginationFooterProps = {
  itemLabel: string
  page: number
  pageCount: number
  pageSize: number
  total: number
  isFetching?: boolean
  pageSizeOptions?: number[]
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

export function PaginationFooter({
  itemLabel,
  page,
  pageCount,
  pageSize,
  total,
  isFetching = false,
  pageSizeOptions = defaultPageSizeOptions,
  onPageChange,
  onPageSizeChange,
}: PaginationFooterProps) {
  const options = pageSizeOptions.includes(pageSize)
    ? pageSizeOptions
    : [...pageSizeOptions, pageSize].sort((left, right) => left - right)

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-sm text-muted-foreground">
        {total === 0 ? `0 ${itemLabel}` : `${(page - 1) * pageSize + 1}-${Math.min(page * pageSize, total)} of ${total} ${itemLabel}`}
      </p>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Rows per page</span>
          <Select
            items={options.map((option) => ({ label: String(option), value: String(option) }))}
            value={String(pageSize)}
            onValueChange={(value) => {
              if (!value) {
                return
              }
              onPageSizeChange(Number(value))
            }}
            disabled={isFetching}
          >
            <SelectTrigger aria-label="Rows per page">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {options.map((option) => (
                  <SelectItem key={option} value={String(option)}>
                    {option}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={() => onPageChange(Math.max(1, page - 1))}
            disabled={page <= 1 || isFetching}
          >
            Previous
          </Button>
          <div className="min-w-20 text-center text-sm text-muted-foreground">
            Page {page} of {pageCount}
          </div>
          <Button
            variant="outline"
            onClick={() => onPageChange(page + 1)}
            disabled={page >= pageCount || isFetching}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  )
}
