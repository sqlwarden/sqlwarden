import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PasswordInput } from './password-input'

describe('PasswordInput', () => {
  it('masks the value by default and toggles visibility on click', () => {
    render(<PasswordInput value="hunter2" onChange={() => {}} />)

    const field = screen.getByDisplayValue('hunter2') as HTMLInputElement
    expect(field.type).toBe('password')

    fireEvent.click(screen.getByRole('button', { name: 'Show password' }))
    expect(field.type).toBe('text')
    expect(screen.getByRole('button', { name: 'Hide password' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Hide password' }))
    expect(field.type).toBe('password')
  })
})
