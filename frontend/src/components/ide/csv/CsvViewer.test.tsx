import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { CsvViewer } from './CsvViewer'

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 28,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        start: index * 28,
        end: (index + 1) * 28,
        size: 28,
        key: index,
        lane: 0,
      })),
    scrollToIndex: vi.fn(),
  }),
}))

function docWithContent(content: string): Y.Doc {
  const doc = new Y.Doc()
  doc.getText('content').insert(0, content)
  return doc
}

describe('CsvViewer', () => {
  it('renders headers, rows, and counts', () => {
    const doc = docWithContent('id,name\n1,Ada\n2,Grace\n')
    render(<CsvViewer doc={doc} />)

    expect(screen.getByRole('columnheader', { name: 'id' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'name' })).toBeInTheDocument()
    expect(screen.getByText('Ada')).toBeInTheDocument()
    expect(screen.getByText('Grace')).toBeInTheDocument()
    expect(screen.getByText('2 rows')).toBeInTheDocument()
    expect(screen.getByText('2 columns')).toBeInTheDocument()
  })

  it('generates fallback names for missing or extra headers', () => {
    const doc = docWithContent('a,\n1,2,3\n')
    render(<CsvViewer doc={doc} />)

    expect(screen.getByRole('columnheader', { name: 'a' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Column 2' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Column 3' })).toBeInTheDocument()
  })

  it('resizes columns by dragging their header handles', () => {
    const doc = docWithContent('id,name\n1,Ada\n')
    render(<CsvViewer doc={doc} />)

    const idHeader = screen.getByRole('columnheader', { name: 'id' })
    const nameHeader = screen.getByRole('columnheader', { name: 'name' })
    const cell = screen.getByText('Ada').closest('td')!

    // Verify unresized column defaults (name-based widths from columnWidthFromName)
    // 'id' column: 2 * 7 + 56 = 70px
    expect(idHeader).toHaveStyle({ width: '70px' })
    // 'name' column: 4 * 7 + 56 = 84px
    expect(nameHeader).toHaveStyle({ width: '84px' })

    // Drag the 'name' column's resize handle 70px to the right (clientX: 100 -> 170)
    // Expected new width: 84 + 70 = 154px (via useColumnResize math: startWidth + moveEvent.clientX - startX)
    const separator = screen.getByRole('separator', { name: 'Resize name column' })

    act(() => {
      fireEvent.mouseDown(separator, { clientX: 100, bubbles: true } as React.MouseEvent)
    })
    act(() => {
      window.dispatchEvent(new MouseEvent('mousemove', { clientX: 170, bubbles: true }))
    })
    act(() => {
      window.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    })

    // Verify the resize was triggered - width should have changed from 84px to 154px
    expect(screen.getByRole('columnheader', { name: 'name' })).toHaveStyle({ width: '154px' })
    expect(cell.parentElement).toBeInTheDocument()
  })

  it('filters rows case-insensitively and updates counts', async () => {
    const doc = docWithContent('id,name\n1,Ada\n2,Grace\n3,ada again\n')
    const user = userEvent.setup()
    render(<CsvViewer doc={doc} />)

    await user.type(screen.getByLabelText('Search CSV rows'), 'ADA')
    expect(await screen.findByText('2 of 3 rows')).toBeInTheDocument()
    expect(screen.getByText('Ada')).toBeInTheDocument()
    expect(screen.getByText('ada again')).toBeInTheDocument()
    expect(screen.queryByText('Grace')).not.toBeInTheDocument()

    await user.click(screen.getByLabelText('Clear search'))
    expect(await screen.findByText('3 rows')).toBeInTheDocument()
  })

  it('shows a no-matching-rows state when search matches nothing', async () => {
    const doc = docWithContent('id,name\n1,Ada\n')
    const user = userEvent.setup()
    render(<CsvViewer doc={doc} />)

    await user.type(screen.getByLabelText('Search CSV rows'), 'zzz')
    expect(await screen.findByText('No matching rows')).toBeInTheDocument()
  })

  it('shows an empty-file state', () => {
    const doc = docWithContent('')
    render(<CsvViewer doc={doc} />)
    expect(screen.getByText('This file is empty')).toBeInTheDocument()
  })

  it('shows a header-only state', () => {
    const doc = docWithContent('id,name\n')
    render(<CsvViewer doc={doc} />)
    expect(screen.getByText('No rows to show')).toBeInTheDocument()
  })

  it('shows a parse error state for malformed CSV', () => {
    const doc = docWithContent('id,name\n1,"unfinished')
    render(<CsvViewer doc={doc} />)
    expect(screen.getByText('Could not parse CSV')).toBeInTheDocument()
    expect(screen.getByText(/Unterminated quoted field/)).toBeInTheDocument()
  })

  it('switches to the unparsed raw source and back to the table', async () => {
    const source = 'id,notes\n1,"hello, team"\n'
    const doc = docWithContent(source)
    const user = userEvent.setup()
    render(<CsvViewer doc={doc} />)

    expect(screen.getByRole('grid', { name: 'CSV data' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Raw view' }))

    expect(screen.getByRole('textbox', { name: 'Raw CSV' })).toHaveValue(source)
    expect(screen.queryByRole('grid', { name: 'CSV data' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Search CSV rows')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Table view' }))
    expect(screen.getByRole('grid', { name: 'CSV data' })).toBeInTheDocument()
  })

  it('allows malformed CSV to be inspected in raw mode', async () => {
    const source = 'id,name\n1,"unfinished'
    const doc = docWithContent(source)
    const user = userEvent.setup()
    render(<CsvViewer doc={doc} />)

    expect(screen.getByText('Could not parse CSV')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Raw view' }))

    expect(screen.getByRole('textbox', { name: 'Raw CSV' })).toHaveValue(source)
    expect(screen.queryByText('Could not parse CSV')).not.toBeInTheDocument()
  })

  it('re-renders when the Y.Doc content changes', () => {
    const doc = docWithContent('id\n1\n')
    render(<CsvViewer doc={doc} />)
    expect(screen.getByText('1 row')).toBeInTheDocument()

    act(() => {
      doc.getText('content').insert(doc.getText('content').length, '2\n')
    })
    expect(screen.getByText('2 rows')).toBeInTheDocument()
  })
})
