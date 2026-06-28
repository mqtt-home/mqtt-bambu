import { useEffect, useState, useCallback } from 'react';
import { Moon, Sun, LogOut, Loader2, Printer, RefreshCw } from 'lucide-react';
import { useTheme } from './contexts/ThemeContext';
import { useSSE } from './hooks/useSSE';
import { getAuthStatus, fetchDevices, logout, type AuthStatus } from './lib/api';
import type { DeviceSummary } from './types/status';
import { PrinterCard } from './components/PrinterCard';
import { LoginPage } from './components/LoginPage';

export function App() {
  const { theme, toggleTheme } = useTheme();
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const { statuses, availabilities, isConnected } = useSSE();

  const loadDevices = useCallback(async () => {
    try {
      setDevices(await fetchDevices());
    } catch {
      // ignore; SSE will fill in
    }
  }, []);

  const refreshAuth = useCallback(async () => {
    try {
      const status = await getAuthStatus();
      setAuth(status);
      if (status.authenticated) {
        await loadDevices();
      }
    } catch {
      setAuth({ authenticated: false });
    } finally {
      setLoading(false);
    }
  }, [loadDevices]);

  useEffect(() => {
    refreshAuth();
  }, [refreshAuth]);

  const handleLogout = async () => {
    await logout();
    setAuth({ authenticated: false });
    setDevices([]);
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!auth?.authenticated) {
    return <LoginPage email={auth?.email} onSuccess={refreshAuth} />;
  }

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-10 border-b bg-background/80 backdrop-blur">
        <div className="max-w-5xl mx-auto px-4 h-14 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Printer className="h-5 w-5 text-primary" />
            <span className="font-semibold">Bambu Monitor</span>
            <span
              className={`ml-2 h-2 w-2 rounded-full ${isConnected ? 'bg-green-500' : 'bg-red-500'}`}
              title={isConnected ? 'Live' : 'Disconnected'}
            />
          </div>
          <div className="flex items-center gap-1">
            <button onClick={loadDevices} className="p-2 rounded-md hover:bg-accent transition-colors touch-target" title="Reload">
              <RefreshCw className="h-4 w-4" />
            </button>
            <button onClick={toggleTheme} className="p-2 rounded-md hover:bg-accent transition-colors touch-target" title="Toggle theme">
              {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
            <button onClick={handleLogout} className="p-2 rounded-md hover:bg-accent transition-colors touch-target" title="Log out">
              <LogOut className="h-4 w-4" />
            </button>
          </div>
        </div>
      </header>

      <main className="max-w-5xl mx-auto px-4 py-6">
        {devices.length === 0 ? (
          <p className="text-center text-muted-foreground py-16">
            No printers found on this account.
          </p>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {devices.map((d) => (
              <PrinterCard
                key={d.slug}
                slug={d.slug}
                name={d.name}
                model={d.model}
                online={availabilities[d.slug] ?? d.online}
                status={statuses[d.slug] ?? d.status}
              />
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
