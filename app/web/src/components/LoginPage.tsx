import { useState } from 'react'
import { KeyRound, Loader2, Mail, Printer } from 'lucide-react'
import { loginWithCode, requestCode } from '@/lib/api'
import { LANGS, useI18n } from '@/lib/i18n'

interface LoginPageProps {
  email?: string
  onSuccess: () => void
}

export function LoginPage({ email, onSuccess }: LoginPageProps) {
  const i = useI18n()
  const [code, setCode] = useState('')
  const [sending, setSending] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [codeSent, setCodeSent] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleRequestCode = async () => {
    setSending(true)
    setError(null)
    try {
      await requestCode()
      setCodeSent(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : i.t('login.requestFailed'))
    } finally {
      setSending(false)
    }
  }

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!code.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      await loginWithCode(code.trim())
      onSuccess()
    } catch (e) {
      setError(e instanceof Error ? e.message : i.t('login.failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 px-safe py-6">
      <div className="card w-full max-w-sm">
        <div className="card-h">
          <span className="ico sm">
            <Printer />
          </span>
          <h3>{i.t('login.title')}</h3>
        </div>
        <div className="card-b space-y-4">
          <p className="text-sm" style={{ color: 'var(--ink-3)' }}>
            {email ? i.t('login.verify', { email }) : i.t('login.hint')}
          </p>

          <button onClick={handleRequestCode} disabled={sending} className="btn w-full">
            {sending ? <Loader2 className="size-4 animate-spin" /> : <Mail className="size-4" />}
            {codeSent ? i.t('login.resend') : i.t('login.send')}
          </button>

          <form onSubmit={handleLogin} className="space-y-3">
            <label className="field">
              <KeyRound className="size-4 shrink-0" style={{ color: 'var(--ink-3)' }} />
              <input
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder={i.t('login.code')}
                aria-label={i.t('login.code')}
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </label>
            <button type="submit" disabled={submitting || !code.trim()} className="btn primary w-full">
              {submitting && <Loader2 className="size-4 animate-spin" />}
              {i.t('login.submit')}
            </button>
          </form>

          {error && (
            <p className="text-center text-sm" style={{ color: 'var(--alert)' }}>
              {error}
            </p>
          )}
        </div>
      </div>
      <div className="seg" role="group" aria-label={i.t('language')}>
        {LANGS.map((l) => (
          <button key={l} aria-pressed={i.lang === l} onClick={() => i.setLang(l)}>
            {l.toUpperCase()}
          </button>
        ))}
      </div>
    </div>
  )
}
