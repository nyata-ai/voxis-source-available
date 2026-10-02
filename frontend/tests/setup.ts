import '../src/i18n'
import '@testing-library/jest-dom'

// Radix UI components (Slider, etc.) rely on ResizeObserver which jsdom doesn't provide.
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// Radix UI Select uses pointer capture APIs which jsdom doesn't provide.
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
}
if (!Element.prototype.setPointerCapture) {
  Element.prototype.setPointerCapture = () => {}
}
if (!Element.prototype.releasePointerCapture) {
  Element.prototype.releasePointerCapture = () => {}
}

// Radix UI Select also uses scrollIntoView which jsdom doesn't implement.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {}
}
