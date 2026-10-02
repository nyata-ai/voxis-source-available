import { describe, it, expect } from 'vitest'
import { statusTone } from './status-tone'

describe('statusTone', () => {
  it('maps in-flight statuses to live', () => {
    expect(statusTone('pending')).toBe('live')
    expect(statusTone('encrypting')).toBe('live')
    expect(statusTone('submitted')).toBe('live')
    expect(statusTone('processing')).toBe('live')
    expect(statusTone('scan_pending')).toBe('live')
  })

  it('maps finished statuses to neutral, never success', () => {
    expect(statusTone('ready')).toBe('neutral')
    expect(statusTone('completed')).toBe('neutral')
    expect(statusTone('deleted')).toBe('neutral')
    expect(statusTone('skipped')).toBe('neutral')
    expect(statusTone('ready')).not.toBe('success')
    expect(statusTone('completed')).not.toBe('success')
  })

  it('maps failure statuses to error', () => {
    expect(statusTone('failed')).toBe('error')
    // The audio-integrity findings spell it `fail`; both are the same tone.
    expect(statusTone('fail')).toBe('error')
    expect(statusTone('scan_infected')).toBe('error')
    expect(statusTone('scan_error')).toBe('error')
  })

  it('maps clean/pass statuses to success', () => {
    expect(statusTone('scan_clean')).toBe('success')
    expect(statusTone('pass')).toBe('success')
  })

  it('maps warn to warning', () => {
    expect(statusTone('warn')).toBe('warning')
  })

  it('falls back to neutral for unknown statuses', () => {
    expect(statusTone('some_unknown_status')).toBe('neutral')
  })
})
