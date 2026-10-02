import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/globals.css'
import './styles/oss.css'
import './i18n'
import App from './App'

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Root element not found. Ensure index.html contains <div id="root"></div>')
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>
)
