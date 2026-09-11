import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { SearchableCombobox, type SearchableComboboxItem } from './SearchableCombobox'

const items: SearchableComboboxItem[] = [
  { value: '1', label: 'Administrator', sublabel: 'Built in' },
  { value: '2', label: 'Viewer' },
]

describe('SearchableCombobox', () => {
  it('opens a popup listing every item', async () => {
    const user = userEvent.setup()
    render(
      <SearchableCombobox
        placeholder="Select a role"
        items={items}
        value={null}
        onValueChange={vi.fn()}
      />,
    )

    await user.click(await screen.findByRole('combobox', { name: 'Select a role' }))

    expect(await screen.findByRole('option', { name: /Administrator/ })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /Viewer/ })).toBeInTheDocument()
  })

  it('calls onValueChange with the selected item and closes the popup', async () => {
    const onValueChange = vi.fn()
    const user = userEvent.setup()
    render(
      <SearchableCombobox
        placeholder="Select a role"
        items={items}
        value={null}
        onValueChange={onValueChange}
      />,
    )

    await user.click(await screen.findByRole('combobox', { name: 'Select a role' }))
    await user.click(await screen.findByRole('option', { name: /Administrator/ }))

    expect(onValueChange).toHaveBeenCalledWith(
      expect.objectContaining({ value: '1', label: 'Administrator' }),
    )
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
  })

  it('filters items client-side by default', async () => {
    const user = userEvent.setup()
    render(
      <SearchableCombobox
        placeholder="Select a role"
        searchPlaceholder="Search roles"
        items={items}
        value={null}
        onValueChange={vi.fn()}
      />,
    )

    await user.click(await screen.findByRole('combobox', { name: 'Select a role' }))
    await screen.findByRole('option', { name: /Viewer/ })

    await user.type(screen.getByPlaceholderText('Search roles'), 'admin')

    expect(screen.getByRole('option', { name: /Administrator/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Viewer/ })).not.toBeInTheDocument()
  })

  it('debounces onSearchChange in remote-search mode instead of filtering locally', async () => {
    const onSearchChange = vi.fn()
    const user = userEvent.setup()
    render(
      <SearchableCombobox
        placeholder="Select a role"
        searchPlaceholder="Search roles"
        items={items}
        value={null}
        onValueChange={vi.fn()}
        onSearchChange={onSearchChange}
      />,
    )

    await user.click(await screen.findByRole('combobox', { name: 'Select a role' }))
    await user.type(await screen.findByPlaceholderText('Search roles'), 'admin')

    expect(onSearchChange).not.toHaveBeenCalled()
    await waitFor(() => expect(onSearchChange).toHaveBeenCalledWith('admin'))
    // Remote-search mode disables the built-in client-side filter: both items stay visible
    // since the caller (not the combobox) is responsible for narrowing `items`.
    expect(screen.getByRole('option', { name: /Viewer/ })).toBeInTheDocument()
  })

  it('shows a loading skeleton instead of the item list while isLoading', async () => {
    const user = userEvent.setup()
    render(
      <SearchableCombobox
        placeholder="Select a role"
        items={items}
        value={null}
        onValueChange={vi.fn()}
        isLoading
      />,
    )

    await user.click(await screen.findByRole('combobox', { name: 'Select a role' }))

    expect(screen.queryByRole('option')).not.toBeInTheDocument()
  })

  it('accepts a selected value that is not part of the current items list', async () => {
    render(
      <SearchableCombobox
        placeholder="Select a role"
        items={items}
        value={{ value: '99', label: 'Stale role' }}
        onValueChange={vi.fn()}
      />,
    )

    expect(await screen.findByRole('combobox', { name: 'Select a role' })).toHaveTextContent(
      'Stale role',
    )
  })
})
