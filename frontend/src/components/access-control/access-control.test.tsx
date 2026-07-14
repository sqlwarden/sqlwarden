// @vitest-environment jsdom

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PermissionDefinition, PolicyBinding } from '#/lib/api/types'
import { Table, TableBody } from '#/components/ui/table'
import { PermissionPicker, groupPermissionDetails } from './PermissionPicker'
import {
  PoliciesTableSkeleton,
  PolicySubjectCell,
  policySubjectDisplayName,
} from './PolicyTablePrimitives'
import { SearchComboboxField } from './SearchComboboxField'

const permissionDetails: PermissionDefinition[] = [
  {
    key: 'org:read',
    label: 'Read organization',
    description: 'View organization details',
    group: 'Organization',
  },
  {
    key: 'policy:modify',
    label: 'Modify policies',
    description: 'Manage role bindings',
    group: 'Policy',
  },
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

    fireEvent.change(screen.getByPlaceholderText('Filter permissions…'), {
      target: { value: 'bindings' },
    })
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
    fireEvent.change(await screen.findByPlaceholderText('Search roles'), {
      target: { value: 'admin' },
    })
    expect(onSearchChange).not.toHaveBeenCalled()
    await waitFor(() => expect(onSearchChange).toHaveBeenCalledWith('admin'))

    fireEvent.click(screen.getByText('Administrator'))
    expect(onChange).toHaveBeenCalledWith(
      '1',
      'Administrator',
      expect.objectContaining({ value: '1' }),
    )
    expect(onSearchChange).toHaveBeenLastCalledWith('')
  })
})

const policyBinding: PolicyBinding = {
  binding_kind: 'role',
  binding_id: 1,
  subject_id: 2,
  subject_type: 'org_members',
  subject_name: '',
  resource_id: 3,
  resource_type: 'org',
  resource_name: 'Acme',
  role_id: 4,
  role_name: 'Baseline Access',
  created_at: '2026-01-01T00:00:00Z',
}

describe('policy table primitives', () => {
  it('uses route-specific labels for aggregate subjects', () => {
    expect(policySubjectDisplayName(policyBinding)).toBe('All users')
    expect(policySubjectDisplayName(policyBinding, { org_members: 'All organization users' })).toBe(
      'All organization users',
    )

    render(<PolicySubjectCell binding={policyBinding} labels={{ org_members: 'All org users' }} />)
    expect(screen.getAllByText('All org users')).toHaveLength(2)
  })

  it('adds a resource placeholder only for workspace policy tables', () => {
    const { rerender } = render(
      <Table>
        <TableBody>
          <PoliciesTableSkeleton canModify={false} />
        </TableBody>
      </Table>,
    )
    expect(document.querySelectorAll('tbody tr')[0]?.children).toHaveLength(3)

    rerender(
      <Table>
        <TableBody>
          <PoliciesTableSkeleton canModify showResource />
        </TableBody>
      </Table>,
    )
    expect(document.querySelectorAll('tbody tr')[0]?.children).toHaveLength(5)
  })
})
