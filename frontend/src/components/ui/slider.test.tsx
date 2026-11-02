import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Slider } from './slider'

describe('Slider', () => {
  it('names the thumb, which is the element a screen reader announces', () => {
    render(<Slider aria-label="Seek" defaultValue={[3]} max={10} />)
    expect(screen.getByRole('slider', { name: 'Seek' })).toBeInTheDocument()
  })
})
