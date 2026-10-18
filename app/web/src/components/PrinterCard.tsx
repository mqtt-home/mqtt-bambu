import type { CSSProperties, ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Fan, Gauge, Power, Printer, RefreshCw, Thermometer, Wifi, WifiOff, XCircle } from 'lucide-react'
import type { ConnectionMode, PrinterStatus, TrayStatus } from '@/types/status'
import { refreshDevice } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

interface PrinterCardProps {
  slug: string
  name: string
  model: string
  mode: ConnectionMode
  online: boolean
  status: PrinterStatus | null
}

// Colour only ever means state.
const STATE_CLASS: Record<string, string> = {
  printing: 'ok',
  finished: 'ok',
  paused: 'warn',
  prepare: 'info',
  slicing: 'info',
  failed: 'bad',
}

const ACTIVE = new Set(['printing', 'paused', 'prepare', 'slicing'])

export function PrinterCard({ slug, name, model, mode, online, status }: PrinterCardProps) {
  const i = useI18n()
  const state = status?.state ?? ''
  const stateClass = !online ? 'bad' : (STATE_CLASS[state] ?? '')
  const stateText = !online ? i.t('state.offline') : status ? i.label('state', state) : i.t('state.unknown')
  const active = online && ACTIVE.has(state)

  return (
    <section className="card">
      <div className="card-h">
        <span className="ico sm" style={{ '--c': active ? 'var(--accent)' : 'var(--idle)' } as CSSProperties}>
          <Printer />
        </span>
        <div className="min-w-0">
          <h3 className="truncate">{name}</h3>
          <div className="truncate text-xs" style={{ color: 'var(--ink-3)' }}>
            {model}
            {model && ' · '}
            {mode === 'lan' ? 'LAN' : 'Cloud'}
          </div>
        </div>
        <span className={`state ${stateClass} ml-auto`}>
          <span className={`dot ${state === 'printing' && online ? 'live' : ''} ${stateClass ? '' : 'idle'}`} />
          {stateText}
        </span>
      </div>

      <div className="card-b">
        {!status ? (
          <Note icon={online ? <Power /> : <WifiOff />} title={online ? i.t('waiting') : i.t('printer.offline')} />
        ) : (
          <>
            <Headline status={status} active={active} />

            {status.print_error !== 0 && (
              <div
                className="mt-3 flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium"
                style={{ color: 'var(--alert)', background: 'var(--alert-soft)' }}
              >
                <AlertTriangle className="size-4 shrink-0" />
                {i.t('error', { code: status.print_error })}
              </div>
            )}

            <div className="stats mt-4" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(128px, 1fr))' }}>
              <Temp icon={<Thermometer />} label={i.t('nozzle')} value={status.nozzle_temp} target={status.nozzle_target_temp} />
              <Temp icon={<Thermometer />} label={i.t('bed')} value={status.bed_temp} target={status.bed_target_temp} />
              {status.chamber_temp > 0 && <Temp icon={<Thermometer />} label={i.t('chamber')} value={status.chamber_temp} />}
              <Stat icon={<Gauge />} label={i.t('speed')}>
                {status.speed_label ? i.label('speed', status.speed_label) : `${i.num(status.speed_mag)} %`}
                {status.speed_label && <small>{i.num(status.speed_mag)} %</small>}
              </Stat>
              {active && (
                <Stat icon={<Fan />} label={i.t('fan')}>
                  {i.num(status.cooling_fan_percent)} %
                </Stat>
              )}
            </div>

            <Filament status={status} />

            <div
              className="mt-4 flex items-center gap-3 border-t pt-2 text-xs"
              style={{ color: 'var(--ink-3)' }}
            >
              <span className="num">{i.t('updated', { time: i.clock(status.updated_at) })}</span>
              {status.wifi_signal && (
                <span className="num hidden items-center gap-1 sm:inline-flex">
                  {online ? <Wifi className="size-3.5" /> : <WifiOff className="size-3.5" />}
                  {status.wifi_signal}
                </span>
              )}
              <button
                onClick={() => refreshDevice(slug).catch(() => {})}
                className="-mr-2 ml-auto inline-flex min-h-[44px] items-center gap-1.5 rounded-lg px-2 hover:text-[var(--ink)]"
                title={i.t('refresh.hint')}
              >
                <RefreshCw className="size-3.5" /> {i.t('refresh')}
              </button>
            </div>
          </>
        )}
      </div>
    </section>
  )
}

function Headline({ status, active }: { status: PrinterStatus; active: boolean }) {
  const i = useI18n()

  if (status.state === 'finished') {
    return (
      <Note
        icon={<CheckCircle2 />}
        color="var(--ok)"
        title={i.t('finished.title')}
        sub={status.job_name}
      />
    )
  }
  if (status.state === 'failed') {
    return <Note icon={<XCircle />} color="var(--alert)" title={i.t('failed.title')} sub={status.job_name} />
  }
  if (!active) {
    return (
      <Note
        icon={<Power />}
        title={i.t('idle.title')}
        sub={status.job_name ? i.t('idle.last', { job: status.job_name }) : undefined}
      />
    )
  }

  const pct = Math.max(0, Math.min(100, status.percent))
  const paused = status.state === 'paused'
  const hasRemaining = status.remaining_minutes > 0

  return (
    <div className="mt-2">
      <div className="flex items-center gap-5">
        <div
          className={`pring ${paused ? 'warn' : ''}`}
          style={{ '--p': pct } as CSSProperties}
          role="progressbar"
          aria-label={i.t('progress')}
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <span className="pct">{i.num(pct)} %</span>
        </div>
        <div className="min-w-0">
          <div className="kicker">{i.t('remaining')}</div>
          <div className="big mt-1.5">{hasRemaining ? i.mins(status.remaining_minutes) : '–'}</div>
          {hasRemaining && status.finish_time && (
            <div className="num mt-1.5 text-sm" style={{ color: 'var(--ink-3)' }}>
              {i.t('eta.at', { time: i.clock(status.finish_time) })}
            </div>
          )}
        </div>
      </div>
      {(status.job_name || status.total_layer_num > 0) && (
        <div className="mt-4 flex flex-wrap items-baseline gap-x-2 text-sm" style={{ color: 'var(--ink-3)' }}>
          {status.job_name && (
            <>
              <span>{i.t('job')}</span>
              <span className="min-w-0 truncate font-medium" style={{ color: 'var(--ink)' }} title={status.job_name}>
                {status.job_name}
              </span>
            </>
          )}
          {status.total_layer_num > 0 && (
            <span className="num">
              {status.job_name && '· '}
              {i.t('layer', { n: i.num(status.layer_num), total: i.num(status.total_layer_num) })}
            </span>
          )}
        </div>
      )}
    </div>
  )
}

function Filament({ status }: { status: PrinterStatus }) {
  const i = useI18n()
  const trays = status.trays ?? []

  if (trays.length > 0) {
    return (
      <div className="mt-4">
        <div className="kicker mb-2">{i.t('filament')}</div>
        <div className="flex flex-wrap gap-2">
          {trays.map((tr: TrayStatus) => (
            <span
              key={`${tr.unit}-${tr.tray}`}
              className={`tray ${tr.active ? 'active' : ''}`}
              title={i.t('tray.label', { n: tr.unit * 4 + tr.tray + 1 })}
            >
              <span
                className={`swatch ${tr.type ? '' : 'empty'}`}
                style={tr.type ? { background: tr.color || 'transparent' } : undefined}
              />
              {tr.type || i.t('tray.empty')}
              {tr.type && tr.remain >= 0 && <span className="num" style={{ color: 'var(--ink-3)' }}>{i.num(tr.remain)} %</span>}
            </span>
          ))}
        </div>
      </div>
    )
  }

  if (!status.active_filament) return null
  return (
    <div className="mt-4 flex items-center gap-2 text-sm">
      <span className="swatch" style={{ background: status.active_color || 'transparent' }} />
      <span style={{ color: 'var(--ink-3)' }}>{i.t('filament')}</span>
      <span className="font-medium">{status.active_filament}</span>
    </div>
  )
}

function Note({ icon, title, sub, color }: { icon: ReactNode; title: string; sub?: string; color?: string }) {
  return (
    <div className="flex items-center gap-4 py-3">
      <span className="ico" style={{ '--c': color ?? 'var(--idle)' } as CSSProperties}>
        {icon}
      </span>
      <div className="min-w-0">
        <div className="font-medium">{title}</div>
        {sub && (
          <div className="truncate text-sm" style={{ color: 'var(--ink-3)' }}>
            {sub}
          </div>
        )}
      </div>
    </div>
  )
}

function Stat({ icon, label, children }: { icon: ReactNode; label: string; children: ReactNode }) {
  return (
    <div className="stat">
      <div className="k">
        {icon}
        {label}
      </div>
      <div className="v">{children}</div>
    </div>
  )
}

function Temp({ icon, label, value, target }: { icon: ReactNode; label: string; value: number; target?: number }) {
  const i = useI18n()
  return (
    <Stat icon={icon} label={label}>
      {i.num(value)} °C
      {target !== undefined && target > 0 && <small>→ {i.num(target)} °C</small>}
    </Stat>
  )
}
