# mqtt-bambu

A bridge between [Bambu Lab](https://bambulab.com) 3D printers and your home MQTT
broker. It reaches your printers over the Bambu **cloud** broker, directly over
your **LAN**, or both at once, merges the printer's status reports, and
republishes a clean, comprehensive status — with a focus on **remaining print
durations** — to your own broker. A small web UI shows live status and handles
the cloud login.

Built in the same style as the other `mqtt-*` bridges (`mqtt-huawei`,
`mqtt-lamarzocco`, `roborock-mqtt`): a Go backend using
[`philipparndt/go-logger`](https://github.com/philipparndt/go-logger) and
[`philipparndt/mqtt-gateway`](https://github.com/philipparndt/mqtt-gateway), a
React/Vite web UI, GoReleaser + Docker CI publishing to `pharndt/mqtt-bambu`.

## Features

- **Cloud connection** to one or more printers bound to your Bambu account
  (no LAN access code needed; works for printers in cloud/LAN-bound mode).
- **LAN mode** for printers switched to *LAN Only*, connecting straight to the
  printer with its access code — no account needed, and it keeps working while
  the cloud is down. **Cloud and LAN printers can run side by side**, so you can
  move one printer at a time.
- **Remaining durations first**: `remaining_minutes`, a human `remaining_text`
  (`"1h 23m"`), and a projected `finish_time` (computed at report time, only
  while actively printing).
- **Comprehensive status**: state, progress %, layers, temperatures
  (nozzle/bed/chamber), fan speeds, print speed, active filament/AMS trays,
  Wi-Fi signal, and error codes.
- **Per-printer MQTT topics** with retained status + availability.
- **Web UI** with live updates over Server-Sent Events, dark/light theme, and the
  email-verification-code login flow.
- **Resilient**: persists the cloud session to disk, re-requests full snapshots
  periodically (reports are otherwise partial deltas), and exposes a Kubernetes
  liveness probe with a grace window.

## Quick start

### Docker

```bash
cp production/config/config.example.json production/config/config.json
# edit config.json (or set BAMBU_EMAIL / BAMBU_PASSWORD env vars)
cd app
docker compose -f docker-compose.dev.yml up --build
```

### From source

```bash
cd app
make build      # builds the web UI then the Go binary into build/
make run        # runs build/mqtt-bambu ../production/config/config.json
```

Then open <http://localhost:8080>.

## Configuration

```json
{
  "mqtt": {
    "url": "tcp://localhost:1883",
    "topic": "home/bambu",
    "qos": 2,
    "retain": true
  },
  "bambu": {
    "region": "global",
    "email": "${BAMBU_EMAIL}",
    "password": "${BAMBU_PASSWORD}",
    "pushall_interval": 300,
    "session_file": "/var/lib/mqtt-bambu/.session/session.json",
    "lan": [
      {
        "name": "H2D",
        "model": "H2D",
        "host": "10.0.0.42",
        "serial": "0123456789ABCDE",
        "access_code": "12345678"
      }
    ]
  },
  "web": {
    "enabled": true,
    "port": 8080
  },
  "loglevel": "info"
}
```

| Key | Description |
| --- | --- |
| `mqtt` | Your home broker. `topic` is the base topic; status is published under `<topic>/<printer-slug>/…`. |
| `bambu.region` | `global` (api.bambulab.com / us.mqtt.bambulab.com) or `china` (api.bambulab.cn / cn.mqtt.bambulab.com). |
| `bambu.email` / `bambu.password` | Bambu Lab account credentials. `${VAR}` references environment variables. Leave `email` empty to disable the cloud entirely and run LAN-only. |
| `bambu.lan` | Printers in LAN Only mode (see below). Omit or leave empty for a cloud-only setup. |
| `bambu.pushall_interval` | Seconds between full-snapshot requests to each printer (default 300). |
| `bambu.session_file` | Where the cloud token + device list are cached so restarts don't re-authenticate. |
| `web.enabled` / `web.port` | The status web UI + login flow. |
| `web.liveness_grace_seconds` | How long the cloud connection may stay broken before `/api/livez` fails (default 240). LAN printers never affect it — a powered-off printer is normal, and a restart would not bring it back. |

### LAN mode

A printer switched to **LAN Only** mode is no longer reachable through the
cloud, so the bridge connects to the MQTT server the printer itself runs. Each
entry under `bambu.lan` needs:

| Key | Description |
| --- | --- |
| `name` | Display name; also derives the MQTT topic slug. Defaults to the serial. |
| `host` | The printer's IP address on your network. Give it a DHCP reservation. |
| `port` | The printer's MQTT port. Defaults to `8883`. |
| `serial` | The printer's serial number, which forms its MQTT topics. |
| `access_code` | The 8-character LAN access code. |
| `model` | Product name shown in the UI, e.g. `"H2D"`. Optional. |

Find the access code and serial on the printer: **Settings → Network → LAN Only
Mode**. The printer serves a self-signed certificate for its own serial, so the
bridge connects over TLS without certificate verification.

Cloud and LAN entries can be mixed freely. If a serial appears both in
`bambu.lan` and on your Bambu account, the LAN connection wins and the cloud
duplicate is dropped — in LAN mode the cloud broker no longer carries that
printer's reports.

### Authentication & the verification code

Most Bambu accounts require an **email verification code** rather than (or in
addition to) a password. On startup the bridge:

1. Reuses a cached session from `session_file` if still valid.
2. Otherwise attempts a password login. If the account needs an emailed code (or
   TOTP two-factor), it logs a notice and waits.
3. Open the web UI, click **Email me a code**, enter the code, and sign in. The
   session is then persisted and the bridge connects to your printers.

## MQTT topics

For a printer named *"Bambu X1C"* (slug `bambu-x1c`) with base topic `home/bambu`:

| Topic | Payload |
| --- | --- |
| `home/bambu/bambu-x1c/status` | Retained JSON status (see below). |
| `home/bambu/bambu-x1c/availability` | `online` / `offline`. |

### Status payload (the interesting bits)

```json
{
  "name": "Bambu X1C",
  "model": "X1 Carbon",
  "serial": "00M00A000000000",
  "state": "printing",
  "printing": true,
  "percent": 42,
  "remaining_minutes": 83,
  "remaining_text": "1h 23m",
  "finish_time": "2026-06-28T13:23:00+02:00",
  "layer_num": 50,
  "total_layer_num": 120,
  "job_name": "calibration",
  "nozzle_temp": 220.5,
  "nozzle_target_temp": 220,
  "bed_temp": 60,
  "bed_target_temp": 60,
  "chamber_temp": 28,
  "cooling_fan_percent": 100,
  "speed_level": 2,
  "speed_label": "standard",
  "active_filament": "PLA",
  "active_color": "#FF8800",
  "wifi_signal": "-45dBm",
  "print_error": 0,
  "updated_at": "2026-06-28T12:00:00+02:00"
}
```

`state` is normalized to `idle` / `prepare` / `slicing` / `printing` / `paused` /
`finished` / `failed`. `finish_time` is only set while actively printing.

## Development

| Command | Description |
| --- | --- |
| `make build` | Build the web UI and Go binary. |
| `make test` | Run the Go tests. |
| `make dev-frontend` | Vite dev server (proxies the API at `localhost:8080`). |
| `make dev-backend` | Build + run the backend against `production/config/config.json`. |

The Go module lives in `app/`; the web UI in `app/web/`.

## How it works

Bambu printers publish status on `device/<serial>/report`, and accept commands
on `device/<serial>/request`. Only the broker and its credentials differ between
the two modes:

| | Broker | Username | Password |
| --- | --- | --- | --- |
| Cloud | `us.mqtt.bambulab.com:8883` | `u_<uid>`, derived from the access-token JWT | the account access token |
| LAN | `<printer-ip>:8883` | `bblp` | the printer's LAN access code |

Reports are **partial deltas**, so the bridge merges them into a cached state
object and periodically publishes a `pushall` request to force a full snapshot.
The merged state is derived into the comprehensive status above and published to
your home broker.

LAN printers connect at startup and are independent of the cloud: they keep
publishing while the account session is expired, being re-authenticated, or not
configured at all.
