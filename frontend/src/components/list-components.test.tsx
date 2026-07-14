// @vitest-environment jsdom

import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { EmptyState } from './EmptyState'
import { PaginationFooter } from './PaginationFooter'
import { SearchInput } from './SearchInput'

describe('SearchInput', () => {
  it('reports input changes and clears a populated search', () => {
    const onValueChange = vi.fn()
    const onClear = vi.fn()
    render(
      <SearchInput
        value="orders"
        onValueChange={onValueChange}
        onClear={onClear}
        placeholder="Search"
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('Search'), { target: { value: 'customers' } })
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }))

    expect(onValueChange).toHaveBeenCalledWith('customers')
    expect(onClear).toHaveBeenCalledOnce()
  })
})

describe('PaginationFooter', () => {
  it('renders the current range and changes pages', () => {
    const onPageChange = vi.fn()
    render(
      <PaginationFooter
        itemLabel="users"
        page={2}
        pageCount={4}
        pageSize={10}
        total={35}
        onPageChange={onPageChange}
        onPageSizeChange={vi.fn()}
      />,
    )

    expect(screen.getByText('11-20 of 35 users')).toBeTruthy()
    expect(screen.getByText('Page 2 of 4')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(onPageChange).toHaveBeenNthCalledWith(1, 1)
    expect(onPageChange).toHaveBeenNthCalledWith(2, 3)
  })
})

describe('EmptyState', () => {
  it('renders its message and optional description', () => {
    render(<EmptyState message="No roles found" description="Create a role to get started." />)
    expect(screen.getByText('No roles found')).toBeTruthy()
    expect(screen.getByText('Create a role to get started.')).toBeTruthy()
  })
})
