import { describe, expect, it } from 'vitest'
import { DEFAULT_SUMMARY_PROFILE, normalizeSummaryProfile, SUMMARY_PROFILES } from './settings'

describe('summary profiles', () => {
  it('defines the seven supported profiles', () => {
    expect(SUMMARY_PROFILES).toEqual([
      'general_professional',
      'legal',
      'investment_analysis',
      'journalism',
      'negotiation',
      'decision_committee',
      'investigation',
    ])
  })

  it.each([undefined, null, '', 'unknown_profile', 1])(
    'uses General Professional for an absent or invalid profile: %j',
    (value) => {
      expect(normalizeSummaryProfile(value)).toBe(DEFAULT_SUMMARY_PROFILE)
    }
  )

  it.each(SUMMARY_PROFILES)('keeps a valid profile: %s', (profile) => {
    expect(normalizeSummaryProfile(profile)).toBe(profile)
  })
})
