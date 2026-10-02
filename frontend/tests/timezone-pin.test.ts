// Guards the TZ pin in vitest.config.ts. tests/setup.ts imports the real i18n,
// so language detection runs for every test in the suite; without the pin the
// suite inherits the host timezone and a developer in a mapped zone (Jakarta,
// Shanghai, Berlin) boots every test in the wrong language.
//
// Verify the pin actually works by running this file with a hostile TZ:
//   TZ=Asia/Jakarta npx vitest run tests/timezone-pin.test.ts
// It must still report UTC.
describe('test host timezone', () => {
  it('is pinned to UTC regardless of the host', () => {
    expect(Intl.DateTimeFormat().resolvedOptions().timeZone).toBe('UTC')
  })
})
