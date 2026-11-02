/**
 * How the audio came in, as a colour: peach = live recording, mint = uploaded
 * file, cream = public link. The Desk tiles, the capture dialog tabs, the
 * Library rows and its filter chips all read from this one map so the three
 * channels stay the same colour everywhere.
 *
 * Fixed hex fills (same in both themes). Contrast of the icon on each disc:
 * ink on peach 9.1:1, ink on mint 10.7:1, cream-ink on cream 5.2:1. Cream is
 * nearly invisible on white (1.2:1), so its disc carries a cream-hair ring;
 * the icon, not the fill, carries the meaning.
 *
 * The Library cannot yet tell a live recording from an upload (both are media
 * rows; `recording_mode` only says regular vs privilege), so audio rows use
 * `upload` until the API says which channel a file came through.
 */
export type IntakeChannel = 'record' | 'upload' | 'link'

export const INTAKE_DISC_CLASS: Record<IntakeChannel, string> = {
  record: 'bg-peach text-peach-foreground',
  upload: 'bg-mint text-mint-foreground',
  link: 'bg-cream text-cream-ink ring-1 ring-inset ring-cream-hair',
}
