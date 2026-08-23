import { Clock, Layers, Thermometer, Gauge, RefreshCw, Wifi, WifiOff, CheckCircle2, AlertTriangle, Printer } from 'lucide-react';
import type { ConnectionMode, PrinterStatus } from '@/types/status';
import { stateLabel, formatFinishTime } from '@/types/status';
import { refreshDevice } from '@/lib/api';

interface PrinterCardProps {
  slug: string;
  name: string;
  model: string;
  mode: ConnectionMode;
  online: boolean;
  status: PrinterStatus | null;
}

const STATE_COLOR: Record<string, string> = {
  printing: 'var(--color-primary)',
  paused: 'hsl(38 92% 50%)',
  finished: 'hsl(142 70% 45%)',
  failed: 'hsl(0 84% 60%)',
};

export function PrinterCard({ slug, name, model, mode, online, status }: PrinterCardProps) {
  const accent = (status && STATE_COLOR[status.state]) || 'var(--color-muted-foreground)';

  return (
    <div className="rounded-lg border bg-card text-card-foreground shadow-sm p-5 flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <Printer className="h-5 w-5 shrink-0" style={{ color: accent }} />
          <div className="min-w-0">
            <h2 className="font-semibold truncate">{name}</h2>
            <p className="text-xs text-muted-foreground truncate">
              {model}
              {model && ' · '}
              <span className="uppercase tracking-wide">{mode}</span>
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span
            className="text-xs font-medium px-2 py-0.5 rounded-full border"
            style={{ color: accent, borderColor: accent }}
          >
            {status ? stateLabel(status.state) : 'Unknown'}
          </span>
          {online ? (
            <Wifi className="h-4 w-4 text-muted-foreground" aria-label="online" />
          ) : (
            <WifiOff className="h-4 w-4 text-muted-foreground" aria-label="offline" />
          )}
        </div>
      </div>

      {!status && (
        <p className="text-sm text-muted-foreground py-6 text-center">
          {online ? 'Waiting for first report…' : 'Printer offline'}
        </p>
      )}

      {status && (
        <>
          {/* Headline: remaining duration */}
          <RemainingBlock status={status} />

          {/* Progress bar */}
          <div className="space-y-1">
            <div className="flex justify-between text-xs text-muted-foreground">
              <span>{status.percent}%</span>
              {status.total_layer_num > 0 && (
                <span className="flex items-center gap-1">
                  <Layers className="h-3 w-3" />
                  {status.layer_num}/{status.total_layer_num}
                </span>
              )}
            </div>
            <div className="h-2 w-full rounded-full bg-muted overflow-hidden">
              <div
                className="h-full rounded-full transition-all"
                style={{ width: `${status.percent}%`, backgroundColor: accent }}
              />
            </div>
          </div>

          {status.job_name && (
            <p className="text-sm truncate" title={status.job_name}>
              <span className="text-muted-foreground">Job: </span>{status.job_name}
            </p>
          )}

          {/* Telemetry grid */}
          <div className="grid grid-cols-2 gap-3 text-sm">
            <Metric icon={<Thermometer className="h-4 w-4" />} label="Nozzle"
              value={`${status.nozzle_temp.toFixed(0)}°C`} sub={`→ ${status.nozzle_target_temp.toFixed(0)}°C`} />
            <Metric icon={<Thermometer className="h-4 w-4" />} label="Bed"
              value={`${status.bed_temp.toFixed(0)}°C`} sub={`→ ${status.bed_target_temp.toFixed(0)}°C`} />
            {status.chamber_temp > 0 && (
              <Metric icon={<Thermometer className="h-4 w-4" />} label="Chamber"
                value={`${status.chamber_temp.toFixed(0)}°C`} />
            )}
            <Metric icon={<Gauge className="h-4 w-4" />} label="Speed"
              value={status.speed_label ? status.speed_label : `${status.speed_mag}%`} />
          </div>

          {/* Active filament */}
          {status.active_filament && (
            <div className="flex items-center gap-2 text-sm">
              <span
                className="inline-block h-4 w-4 rounded-full border"
                style={{ backgroundColor: status.active_color || 'transparent' }}
              />
              <span className="text-muted-foreground">Filament:</span>
              <span>{status.active_filament}</span>
            </div>
          )}

          {status.print_error !== 0 && (
            <div className="flex items-center gap-2 text-sm text-red-500">
              <AlertTriangle className="h-4 w-4" />
              Print error {status.print_error}
            </div>
          )}

          {/* Footer */}
          <div className="flex items-center justify-between text-xs text-muted-foreground pt-1 border-t">
            <span>Updated {formatFinishTime(status.updated_at)}</span>
            <button
              onClick={() => refreshDevice(slug).catch(() => {})}
              className="flex items-center gap-1 hover:text-foreground transition-colors touch-target"
              title="Request a fresh snapshot"
            >
              <RefreshCw className="h-3.5 w-3.5" /> Refresh
            </button>
          </div>
        </>
      )}
    </div>
  );
}

function RemainingBlock({ status }: { status: PrinterStatus }) {
  if (status.state === 'finished') {
    return (
      <div className="flex items-center gap-3 rounded-md bg-muted/50 p-3">
        <CheckCircle2 className="h-6 w-6" style={{ color: 'hsl(142 70% 45%)' }} />
        <div>
          <p className="font-semibold">Print finished</p>
          <p className="text-xs text-muted-foreground">{status.job_name || ''}</p>
        </div>
      </div>
    );
  }

  const hasRemaining = status.remaining_minutes > 0 && (status.printing || status.state === 'paused');

  return (
    <div className="flex items-center gap-3 rounded-md bg-muted/50 p-3">
      <Clock className="h-6 w-6 shrink-0" style={{ color: 'var(--color-primary)' }} />
      <div className="min-w-0">
        {hasRemaining ? (
          <>
            <p className="text-2xl font-bold leading-tight tabular-nums">
              {status.remaining_text || `${status.remaining_minutes}m`}
            </p>
            <p className="text-xs text-muted-foreground">
              remaining
              {status.finish_time && ` · done ~${formatFinishTime(status.finish_time)}`}
            </p>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">No active print</p>
        )}
      </div>
    </div>
  );
}

function Metric({ icon, label, value, sub }: { icon: React.ReactNode; label: string; value: string; sub?: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-muted-foreground">{icon}</span>
      <div className="leading-tight">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="font-medium tabular-nums">
          {value}{sub && <span className="text-xs text-muted-foreground ml-1">{sub}</span>}
        </p>
      </div>
    </div>
  );
}
