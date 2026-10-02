import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { UsageBar } from './usage-bar'

describe('UsageBar', () => {
  it('renders bg-primary when usage is low', () => {
    const { getByRole } = render(<UsageBar used={10} total={100} />)
    const fill = getByRole('progressbar').firstChild as HTMLElement
    expect(fill.className).toContain('bg-primary')
  })

  it('renders bg-warning when usage exceeds 80%', () => {
    const { getByRole } = render(<UsageBar used={85} total={100} />)
    const fill = getByRole('progressbar').firstChild as HTMLElement
    expect(fill.className).toContain('bg-warning')
  })

  it('renders bg-destructive when usage exceeds 90%', () => {
    const { getByRole } = render(<UsageBar used={95} total={100} />)
    const fill = getByRole('progressbar').firstChild as HTMLElement
    expect(fill.className).toContain('bg-destructive')
  })
})
