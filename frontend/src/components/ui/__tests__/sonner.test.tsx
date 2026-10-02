import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { Toaster } from '../sonner'

describe('Toaster', () => {
  it('renders without crashing', () => {
    const { container } = render(<Toaster />)
    expect(container).toBeDefined()
  })
})
