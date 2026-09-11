import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ResultColumn, ResultValue } from '#/lib/api/types'
import { ContextMenuProvider } from '#/components/ui/context-menu'
import { DataGrid } from './DataGrid'

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 29,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        start: index * 29,
        end: (index + 1) * 29,
        size: 29,
        key: index,
        lane: 0,
      })),
    scrollToIndex: vi.fn(),
  }),
}))

const columns: ResultColumn[] = [
  { name: 'id', type: 'integer', raw_type: 'integer', nullable: false },
  { name: 'name', type: 'text', raw_type: 'text', nullable: true },
]

function textValue(text: string): ResultValue {
  return { type: 'text', text }
}
function intValue(n: number): ResultValue {
  return { type: 'integer', integer: n }
}

const rows: ResultValue[][] = [
  [intValue(1), textValue('Ada')],
  [intValue(2), textValue('Grace')],
]

function renderGrid(props: Partial<React.ComponentProps<typeof DataGrid>> = {}) {
  return render(
    <ContextMenuProvider>
      <DataGrid columns={columns} rows={rows} {...props} />
    </ContextMenuProvider>,
  )
}

describe('DataGrid', () => {
  it('renders columns and cell values', () => {
    renderGrid()
    expect(screen.getByRole('columnheader', { name: 'id' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'name' })).toBeInTheDocument()
    expect(screen.getByText('Ada')).toBeInTheDocument()
    expect(screen.getByText('Grace')).toBeInTheDocument()
  })

  it('shows a custom empty message when there are no rows', () => {
    renderGrid({ rows: [], emptyMessage: 'Nothing here yet' })
    expect(screen.getByText('Nothing here yet')).toBeInTheDocument()
  })

  it('opens the cell detail panel and moves selection with arrow keys', () => {
    renderGrid()
    fireEvent.mouseDown(screen.getByText('Ada').closest('td')!)
    expect(screen.getByLabelText('Copy value')).toBeInTheDocument()

    // The value panel now also shows "Ada" alongside the grid cell.
    fireEvent.keyDown(screen.getAllByText('Ada')[0].closest('[data-cell]')!, { key: 'ArrowDown' })
    expect(screen.getAllByText('Grace').length).toBeGreaterThan(0)
  })

  it('copies the selected range with the keyboard shortcut', () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText')
    renderGrid()

    const cell = screen.getByText('Ada').closest('[data-cell]')!
    fireEvent.mouseDown(cell)
    fireEvent.keyDown(cell, { key: 'c', ctrlKey: true })

    expect(writeText).toHaveBeenCalledWith('Ada')
  })

  it('falls back to the default copy-only cell menu when no builder is passed', () => {
    renderGrid()
    fireEvent.contextMenu(screen.getByText('Ada').closest('td')!)
    expect(screen.getByRole('menuitem', { name: 'Copy' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Set NULL' })).not.toBeInTheDocument()
  })

  it('uses a passed-in cell menu builder instead of the default', () => {
    renderGrid({
      buildCellMenu: () => [
        { kind: 'action', id: 'custom', label: 'Custom action', onSelect: vi.fn() },
      ],
    })
    fireEvent.contextMenu(screen.getByText('Ada').closest('td')!)
    expect(screen.getByRole('menuitem', { name: 'Custom action' })).toBeInTheDocument()
  })

  it('calls onScrollNearEnd once the scroll container nears the bottom', () => {
    const onScrollNearEnd = vi.fn()
    renderGrid({ onScrollNearEnd })

    const scrollEl = screen.getByTestId('data-grid-scroll')
    Object.defineProperty(scrollEl, 'scrollHeight', { value: 1000, configurable: true })
    Object.defineProperty(scrollEl, 'clientHeight', { value: 400, configurable: true })
    fireEvent.scroll(scrollEl, { target: { scrollTop: 700 } })

    expect(onScrollNearEnd).toHaveBeenCalledTimes(1)
  })

  it('does not call onScrollNearEnd while isLoadingMore is true', () => {
    const onScrollNearEnd = vi.fn()
    renderGrid({ onScrollNearEnd, isLoadingMore: true })

    const scrollEl = screen.getByTestId('data-grid-scroll')
    Object.defineProperty(scrollEl, 'scrollHeight', { value: 1000, configurable: true })
    Object.defineProperty(scrollEl, 'clientHeight', { value: 400, configurable: true })
    fireEvent.scroll(scrollEl, { target: { scrollTop: 700 } })

    expect(onScrollNearEnd).not.toHaveBeenCalled()
  })

  it('shows the per-column type caption by default', () => {
    renderGrid()
    expect(screen.getByText('integer')).toBeInTheDocument()
    expect(screen.getByText('text')).toBeInTheDocument()
  })

  it('hides the per-column type caption when showColumnType is false', () => {
    renderGrid({ showColumnType: false })
    expect(screen.queryByText('integer')).not.toBeInTheDocument()
    expect(screen.queryByText('text')).not.toBeInTheDocument()
  })
})
