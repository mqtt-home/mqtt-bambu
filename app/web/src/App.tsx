import { useCallback, useEffect, useState } from 'react'
import { Loader2, LogOut, Printer, RefreshCw } from 'lucide-react'
import { useSSE } from './hooks/useSSE'
import { getAuthStatus, fetchDevices, logout, type AuthStatus } from './lib/api'
import { LANGS, useI18n } from './lib/i18n'
import type { DeviceSummary } from './types/status'
import { PrinterCard } from './components/PrinterCard'
import { LoginPage } from './components/LoginPage'

export function App() {
  const { t } = useI18n()
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [devices, setDevices] = useState<DeviceSummary[]>([])
  const [loading, setLoading] = useState(true)
  const { statuses, availabilities, isConnected } = useSSE()

  const loadDevices = useCallback(async () => {
    try {
      setDevices((await fetchDevices()) ?? [])
    } catch {
      // ignore; SSE will fill in
    }
  }, [])

  const refreshAuth = useCallback(async () => {
    try {
      const status = await getAuthStatus()
      setAuth(status)
      // LAN printers are bridged without an account, so always load devices.
      if (status.authenticated || !status.cloud_enabled) {
        await loadDevices()
      }
    } catch {
      setAuth({ cloud_enabled: true, authenticated: false })
    } finally {
      setLoading(false)
    }
  }, [loadDevices])

  useEffect(() => {
    refreshAuth()
  }, [refreshAuth])

  const handleLogout = async () => {
    await logout()
    setAuth({ cloud_enabled: true, authenticated: false })
    setDevices([])
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="size-6 animate-spin" style={{ color: 'var(--ink-3)' }} />
      </div>
    )
  }

  // Only the cloud needs a login; a LAN-only bridge goes straight to the status.
  if (auth?.cloud_enabled !== false && !auth?.authenticated) {
    return <LoginPage email={auth?.email} onSuccess={refreshAuth} />
  }

  return (
    <div className="min-h-screen">
      <header className="top-bar">
        <div className="mx-auto flex max-w-5xl items-center gap-3 px-safe py-2.5">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <span className="ico">
              <Printer />
            </span>
            <div className="min-w-0">
              <div className="truncate text-[17px] leading-tight font-semibold tracking-tight">{t('title')}</div>
              <div className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--ink-3)' }}>
                {isConnected ? (
                  'bambu'
                ) : (
                  <>
                    <span className="dot bad" /> {t('disconnected')}
                  </>
                )}
              </div>
            </div>
          </div>
          <LangSwitch />
          <div className="flex">
            <button onClick={loadDevices} className="icon-btn" title={t('reload')} aria-label={t('reload')}>
              <RefreshCw className="size-[18px]" />
            </button>
            {auth?.cloud_enabled && (
              <button onClick={handleLogout} className="icon-btn" title={t('logout')} aria-label={t('logout')}>
                <LogOut className="size-[18px]" />
              </button>
            )}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-safe pt-5 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
        {devices.length === 0 ? (
          <p className="py-16 text-center" style={{ color: 'var(--ink-3)' }}>
            {t('empty')}
          </p>
        ) : (
          <div className="grid items-start gap-4 lg:grid-cols-2">
            {devices.map((d) => (
              <PrinterCard
                key={d.slug}
                slug={d.slug}
                name={d.name}
                model={d.model}
                mode={d.mode}
                online={availabilities[d.slug] ?? d.online}
                status={statuses[d.slug] ?? d.status}
              />
            ))}
          </div>
        )}
      </main>
    </div>
  )
}

function LangSwitch() {
  const i = useI18n()
  return (
    <div className="seg" role="group" aria-label={i.t('language')}>
      {LANGS.map((l) => (
        <button key={l} aria-pressed={i.lang === l} onClick={() => i.setLang(l)}>
          {l.toUpperCase()}
        </button>
      ))}
    </div>
  )
}
