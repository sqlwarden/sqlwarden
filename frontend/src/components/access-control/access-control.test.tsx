// @vitest-environment jsdom

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PermissionDefinition } from '#/lib/api/types'
import { PermissionPicker, groupPermissionDetails } from './PermissionPicker'
import { SearchComboboxField } from './SearchComboboxField'

const permissionDetails: PermissionDefinition[] = [
  { key: 'org:read', label: 'Read organization', description: 'View organization details', group: 'Organization' },
  { key: 'policy:modify', label: 'Modify policies', description: 'Manage role bindings', group: 'Policy' },
]

describe('PermissionPicker', () => {
  it('groups permissions in catalog order', () => {
    expect(groupPermissionDetails(permissionDetails)).toEqual([
      { name: 'Organization', permissions: [permissionDetails[0]] },
      { name: 'Policy', permissions: [permissionDetails[1]] },
    ])
  })

  it('filters by permission metadata and reports selection', () => {
    const onPermissionChecked = vi.fn()
    render(
      <PermissionPicker
        description="Choose permissions"
        idPrefix="test"
        selectedPermissions={new Set()}
        permissionDetails={permissionDetails}
        permissionDefinitions={new Map(permissionDetails.map((item) => [item.key, item]))}
        disabled={false}
        onPermissionChecked={onPermissionChecked}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('Filter permissions…'), { target: { value: 'bindings' } })
    expect(screen.queryByText('Read organization')).toBeNull()
    fireEvent.click(screen.getByText('Modify policies'))
    expect(onPermissionChecked).toHaveBeenCalledWith('policy:modify', true)
  })
})

describe('SearchComboboxField', () => {
  it('debounces remote search and returns the selected item', async () => {
    const onChange = vi.fn()
    const onSearchChange = vi.fn()
    render(
      <SearchComboboxField
        label="Role"
        placeholder="Select a role"
        searchPlaceholder="Search roles"
        selectedValue=""
        selectedLabel=""
        items={[{ value: '1', label: 'Administrator', sublabel: 'Built in' }]}
        isLoading={false}
        disabled={false}
        onChange={onChange}
        onSearchChange={onSearchChange}
      />,
    )

    fireEvent.click(screen.getByText('Select a role'))
    fireEvent.change(await screen.findByPlaceholderText('Search roles'), { target: { value: 'admin' } })
    expect(onSearchChange).not.toHaveBeenCalled()
    await waitFor(() => expect(onSearchChange).toHaveBeenCalledWith('admin'))

    fireEvent.click(screen.getByText('Administrator'))
    expect(onChange).toHaveBeenCalledWith('1', 'Administrator', expect.objectContaining({ value: '1' }))
    expect(onSearchChange).toHaveBeenLastCalledWith('')
  })
})
