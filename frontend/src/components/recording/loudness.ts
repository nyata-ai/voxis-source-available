/** Speech sits roughly between these levels; quieter reads as silence. */
const FLOOR_DB = -60
const CEILING_DB = -10

/**
 * Loudness of one analyser frame as 0–1: the RMS of the time-domain samples
 * (`getFloatTimeDomainData`, -1…1) on a -60…-10 dBFS scale, so anything at or
 * under -60 dBFS reads zero and normal speech fills most of the tape's height.
 * The float samples matter: the byte variant is floored to 8 bits, whose own
 * noise sits near -45 dBFS, so every quiet input drew the same 30% bar.
 */
export function loudnessFromTimeDomain(samples: Float32Array): number {
  if (samples.length === 0) return 0
  let sum = 0
  for (let i = 0; i < samples.length; i++) {
    sum += samples[i]! * samples[i]!
  }
  const rms = Math.sqrt(sum / samples.length)
  if (rms <= 0) return 0
  const db = 20 * Math.log10(rms)
  return Math.min(1, Math.max(0, (db - FLOOR_DB) / (CEILING_DB - FLOOR_DB)))
}
