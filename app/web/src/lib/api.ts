import type { DeviceSummary } from '@/types/status';

export const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

export interface AuthStatus {
  /** False when no Bambu account is configured; the bridge then runs LAN-only. */
  cloud_enabled: boolean;
  authenticated: boolean;
  email?: string;
  devices?: number;
}

export async function getAuthStatus(): Promise<AuthStatus> {
  const response = await fetch(`${API_BASE}/auth/status`);
  if (!response.ok) throw new Error('Failed to get auth status');
  return response.json();
}

export async function requestCode(): Promise<void> {
  const response = await fetch(`${API_BASE}/auth/request-code`, { method: 'POST' });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || 'Failed to request code');
  }
}

export async function loginWithCode(code: string): Promise<AuthStatus> {
  const response = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || 'Login failed');
  return data;
}

export async function logout(): Promise<void> {
  await fetch(`${API_BASE}/auth/logout`, { method: 'POST' });
}

export async function fetchDevices(): Promise<DeviceSummary[]> {
  const response = await fetch(`${API_BASE}/devices`);
  if (!response.ok) throw new Error('Failed to fetch devices');
  return response.json();
}

export async function refreshDevice(slug: string): Promise<void> {
  const response = await fetch(`${API_BASE}/devices/${slug}/refresh`, { method: 'POST' });
  if (!response.ok) throw new Error('Failed to refresh');
}
