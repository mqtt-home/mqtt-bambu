import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App.tsx'
import { ErrorBoundary } from './components/ErrorBoundary'
import { I18nProvider } from './lib/i18n'
import './index.css'

// Follow the system appearance live (index.html sets it before first paint).
const dark = matchMedia('(prefers-color-scheme: dark)')
dark.addEventListener('change', (e) => {
  // Embedded in the Wall Display panel: stay dark over its gradient.
  const on = e.matches || document.documentElement.dataset.embed === 'wall-display'
  document.documentElement.classList.toggle('dark', on)
  document.documentElement.dataset.theme = on ? 'dark' : 'light'
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </I18nProvider>
  </React.StrictMode>,
)
