import { useState } from 'react';
import { Mail, KeyRound, Loader2 } from 'lucide-react';
import { requestCode, loginWithCode } from '@/lib/api';

interface LoginPageProps {
  email?: string;
  onSuccess: () => void;
}

export function LoginPage({ email, onSuccess }: LoginPageProps) {
  const [code, setCode] = useState('');
  const [sending, setSending] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [codeSent, setCodeSent] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleRequestCode = async () => {
    setSending(true);
    setError(null);
    try {
      await requestCode();
      setCodeSent(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to request code');
    } finally {
      setSending(false);
    }
  };

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!code.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await loginWithCode(code.trim());
      onSuccess();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Login failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="w-full max-w-sm rounded-lg border bg-card text-card-foreground shadow-sm p-6 space-y-5">
        <div className="text-center space-y-1">
          <h1 className="text-xl font-bold">Bambu Cloud Login</h1>
          <p className="text-sm text-muted-foreground">
            {email ? <>Verify the account <span className="font-medium">{email}</span></> : 'Sign in with an email verification code'}
          </p>
        </div>

        <button
          onClick={handleRequestCode}
          disabled={sending}
          className="w-full flex items-center justify-center gap-2 rounded-md border px-4 py-2.5 text-sm font-medium hover:bg-accent transition-colors disabled:opacity-50 touch-target"
        >
          {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Mail className="h-4 w-4" />}
          {codeSent ? 'Resend code' : 'Email me a code'}
        </button>

        <form onSubmit={handleLogin} className="space-y-3">
          <div className="relative">
            <KeyRound className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <input
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="Verification code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              className="w-full rounded-md border bg-background pl-9 pr-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <button
            type="submit"
            disabled={submitting || !code.trim()}
            className="w-full flex items-center justify-center gap-2 rounded-md bg-primary text-primary-foreground px-4 py-2.5 text-sm font-medium hover:opacity-90 transition-opacity disabled:opacity-50 touch-target"
          >
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            Sign in
          </button>
        </form>

        {error && <p className="text-sm text-red-500 text-center">{error}</p>}
      </div>
    </div>
  );
}
