import { useMemo } from 'react'
import { createI18n, type I18n as BaseI18n } from '@mqtt-home/ui/i18n'

export { LANGS, LangSwitch, resolveText, type Lang, type Text } from '@mqtt-home/ui/i18n'

// German is the house language; English stays one tap away (per device).
const dict = {
  de: {
    title: '3D-Drucker',
    live: 'Live',
    disconnected: 'Keine Verbindung',
    reload: 'Neu laden',
    logout: 'Abmelden',
    'empty': 'Keine Drucker konfiguriert.',
    'state.idle': 'Bereit',
    'state.prepare': 'Bereitet vor',
    'state.slicing': 'Slicing',
    'state.printing': 'Druckt',
    'state.paused': 'Pausiert',
    'state.finished': 'Fertig',
    'state.failed': 'Fehlgeschlagen',
    'state.unknown': 'Unbekannt',
    'state.offline': 'Offline',
    online: 'Online',
    offline: 'Offline',
    waiting: 'Warte auf den ersten Bericht …',
    'printer.offline': 'Drucker ist offline',
    progress: 'Fortschritt',
    remaining: 'Restzeit',
    'eta.at': 'fertig um {time}',
    'finished.title': 'Druck fertig',
    'failed.title': 'Druck fehlgeschlagen',
    'idle.title': 'Kein aktiver Druck',
    'idle.last': 'Zuletzt: {job}',
    layer: 'Schicht {n} von {total}',
    job: 'Druck',
    nozzle: 'Düse',
    bed: 'Druckbett',
    chamber: 'Kammer',
    speed: 'Tempo',
    fan: 'Bauteillüfter',
    'speed.silent': 'Leise',
    'speed.standard': 'Standard',
    'speed.sport': 'Sport',
    'speed.ludicrous': 'Turbo',
    filament: 'Filament',
    'tray.empty': 'leer',
    'tray.label': 'Slot {n}',
    error: 'Druckfehler {code}',
    updated: 'Aktualisiert {time}',
    refresh: 'Aktualisieren',
    'refresh.hint': 'Neuen Statusbericht vom Drucker anfordern',
    wifi: 'WLAN {signal}',
    'login.title': 'Bambu-Cloud-Anmeldung',
    'login.verify': 'Konto {email} bestätigen',
    'login.hint': 'Mit einem Bestätigungscode per E-Mail anmelden',
    'login.send': 'Code per E-Mail senden',
    'login.resend': 'Code erneut senden',
    'login.code': 'Bestätigungscode',
    'login.submit': 'Anmelden',
    'login.requestFailed': 'Code konnte nicht angefordert werden',
    'login.failed': 'Anmeldung fehlgeschlagen',
  },
  en: {
    title: '3D printers',
    live: 'Live',
    disconnected: 'Disconnected',
    reload: 'Reload',
    logout: 'Log out',
    'empty': 'No printers configured.',
    'state.idle': 'Idle',
    'state.prepare': 'Preparing',
    'state.slicing': 'Slicing',
    'state.printing': 'Printing',
    'state.paused': 'Paused',
    'state.finished': 'Finished',
    'state.failed': 'Failed',
    'state.unknown': 'Unknown',
    'state.offline': 'Offline',
    online: 'Online',
    offline: 'Offline',
    waiting: 'Waiting for the first report …',
    'printer.offline': 'Printer is offline',
    progress: 'Progress',
    remaining: 'Remaining',
    'eta.at': 'done at {time}',
    'finished.title': 'Print finished',
    'failed.title': 'Print failed',
    'idle.title': 'No active print',
    'idle.last': 'Last: {job}',
    layer: 'Layer {n} of {total}',
    job: 'Job',
    nozzle: 'Nozzle',
    bed: 'Bed',
    chamber: 'Chamber',
    speed: 'Speed',
    fan: 'Part fan',
    'speed.silent': 'Silent',
    'speed.standard': 'Standard',
    'speed.sport': 'Sport',
    'speed.ludicrous': 'Ludicrous',
    filament: 'Filament',
    'tray.empty': 'empty',
    'tray.label': 'Slot {n}',
    error: 'Print error {code}',
    updated: 'Updated {time}',
    refresh: 'Refresh',
    'refresh.hint': 'Request a fresh status report from the printer',
    wifi: 'Wi-Fi {signal}',
    'login.title': 'Bambu Cloud login',
    'login.verify': 'Verify the account {email}',
    'login.hint': 'Sign in with an email verification code',
    'login.send': 'Email me a code',
    'login.resend': 'Resend code',
    'login.code': 'Verification code',
    'login.submit': 'Sign in',
    'login.requestFailed': 'Failed to request code',
    'login.failed': 'Login failed',
  },
}

export type Key = keyof (typeof dict)['de']

export interface I18n extends BaseI18n<Key> {
  /** The label of a key family member, falling back to the raw value ("state.idle", "speed.sport"). */
  label: (prefix: 'state' | 'speed', value: string) => string
  /** "1 h 21 min" from minutes. */
  mins: (m: number) => string
  /** 24h clock time, "14:05"; with the weekday when not today. */
  clock: (iso?: string) => string
}

const base = createI18n(dict)
export const I18nProvider = base.I18nProvider

export function useI18n(): I18n {
  const i = base.useI18n()
  return useMemo<I18n>(() => {
    const { lang, locale, t } = i
    const label: I18n['label'] = (prefix, value) => {
      const k = `${prefix}.${value}` as Key
      return k in dict[lang] ? t(k) : value
    }
    const mins: I18n['mins'] = (m) => {
      if (!isFinite(m) || m < 0) return '–'
      const h = Math.floor(m / 60)
      const r = Math.round(m % 60)
      if (h > 0) return r > 0 ? `${h} h ${r} min` : `${h} h`
      return `${r} min`
    }
    const clock: I18n['clock'] = (iso) => {
      if (!iso) return '–'
      const d = new Date(iso)
      if (Number.isNaN(d.getTime())) return '–'
      const time = d.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit', hour12: false })
      if (d.toDateString() === new Date().toDateString()) return time
      return `${d.toLocaleDateString(locale, { weekday: 'short' })} ${time}`
    }
    return { ...i, label, mins, clock }
  }, [i])
}
