export interface TrayStatus {
  unit: number;
  tray: number;
  type?: string;
  color?: string;
  remain: number;
  active: boolean;
}

// PrinterStatus mirrors bambu.PublishedStatus on the backend.
export interface PrinterStatus {
  name: string;
  model: string;
  serial: string;

  state: string;
  printing: boolean;
  percent: number;
  remaining_minutes: number;
  remaining_text?: string;
  finish_time?: string;
  layer_num: number;
  total_layer_num: number;
  job_name?: string;
  gcode_file?: string;
  stage?: string;

  nozzle_temp: number;
  nozzle_target_temp: number;
  bed_temp: number;
  bed_target_temp: number;
  chamber_temp: number;

  cooling_fan_percent: number;
  aux_fan_percent: number;
  chamber_fan_percent: number;

  speed_level: number;
  speed_label?: string;
  speed_mag: number;

  active_filament?: string;
  active_color?: string;
  trays?: TrayStatus[];

  wifi_signal?: string;
  print_error: number;

  updated_at: string;
}

export type ConnectionMode = 'cloud' | 'lan';

export interface DeviceSummary {
  slug: string;
  name: string;
  model: string;
  serial: string;
  mode: ConnectionMode;
  online: boolean;
  status: PrinterStatus | null;
}

export interface StatusSSEEvent extends PrinterStatus {
  type: 'status';
  slug: string;
}

export interface AvailabilitySSEEvent {
  type: 'availability';
  slug: string;
  online: boolean;
}

export type SSEEvent = StatusSSEEvent | AvailabilitySSEEvent;

const STATE_LABELS: Record<string, string> = {
  idle: 'Idle',
  prepare: 'Preparing',
  slicing: 'Slicing',
  printing: 'Printing',
  paused: 'Paused',
  finished: 'Finished',
  failed: 'Failed',
};

export function stateLabel(state: string): string {
  return STATE_LABELS[state] ?? state;
}

export function formatFinishTime(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  return sameDay ? time : `${d.toLocaleDateString([], { weekday: 'short' })} ${time}`;
}
