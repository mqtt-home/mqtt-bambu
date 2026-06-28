package bambu

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// PublishedStatus is the comprehensive, derived status published to the home
// MQTT broker and served to the web UI. The remaining-duration fields are the
// headline values; the rest gives a full picture of the printer.
type PublishedStatus struct {
	// Identity.
	Name   string `json:"name"`
	Model  string `json:"model"`
	Serial string `json:"serial"`

	// Job / durations (the primary interest).
	State            string `json:"state"` // normalized: idle/prepare/printing/paused/finished/failed/...
	Printing         bool   `json:"printing"`
	Percent          int    `json:"percent"` // 0..100
	RemainingMinutes int    `json:"remaining_minutes"`
	RemainingText    string `json:"remaining_text,omitempty"` // "1h 23m"
	FinishTime       string `json:"finish_time,omitempty"`    // RFC3339, computed at report time
	LayerNum         int    `json:"layer_num"`
	TotalLayerNum    int    `json:"total_layer_num"`
	JobName          string `json:"job_name,omitempty"`
	GCodeFile        string `json:"gcode_file,omitempty"`
	Stage            string `json:"stage,omitempty"`

	// Temperatures (°C).
	NozzleTemp       float64 `json:"nozzle_temp"`
	NozzleTargetTemp float64 `json:"nozzle_target_temp"`
	BedTemp          float64 `json:"bed_temp"`
	BedTargetTemp    float64 `json:"bed_target_temp"`
	ChamberTemp      float64 `json:"chamber_temp"`

	// Fans (percent, best-effort from gear values).
	CoolingFanPercent int `json:"cooling_fan_percent"`
	AuxFanPercent     int `json:"aux_fan_percent"`
	ChamberFanPercent int `json:"chamber_fan_percent"`

	// Speed.
	SpeedLevel int    `json:"speed_level"`
	SpeedLabel string `json:"speed_label,omitempty"`
	SpeedMag   int    `json:"speed_mag"`

	// Filament / AMS.
	ActiveFilament string       `json:"active_filament,omitempty"`
	ActiveColor    string       `json:"active_color,omitempty"`
	Trays          []TrayStatus `json:"trays,omitempty"`

	// Connectivity / errors.
	WifiSignal string `json:"wifi_signal,omitempty"`
	PrintError int    `json:"print_error"`

	// Timestamp the status snapshot was assembled (local time).
	UpdatedAt string `json:"updated_at"`
}

type TrayStatus struct {
	Unit   int    `json:"unit"`
	Tray   int    `json:"tray"`
	Type   string `json:"type,omitempty"`
	Color  string `json:"color,omitempty"`
	Remain int    `json:"remain"`
	Active bool   `json:"active"`
}

var speedLabels = map[int]string{
	1: "silent",
	2: "standard",
	3: "sport",
	4: "ludicrous",
}

// buildStatus derives the published status from a merged report. now is the
// local time the report was received and is used to compute the finish ETA.
func buildStatus(name, model, serial string, r Report, now time.Time) PublishedStatus {
	state := normalizeState(r.GCodeState)
	printing := state == "printing"

	s := PublishedStatus{
		Name:              name,
		Model:             model,
		Serial:            serial,
		State:             state,
		Printing:          printing,
		Percent:           clampPercent(r.MCPercent),
		RemainingMinutes:  r.MCRemainingTime,
		LayerNum:          r.LayerNum,
		TotalLayerNum:     r.TotalLayerNum,
		JobName:           strings.TrimSuffix(r.SubtaskName, ".gcode"),
		GCodeFile:         r.GCodeFile,
		Stage:             r.MCPrintStage,
		NozzleTemp:        r.NozzleTemper,
		NozzleTargetTemp:  r.NozzleTargetTemper,
		BedTemp:           r.BedTemper,
		BedTargetTemp:     r.BedTargetTemper,
		ChamberTemp:       r.ChamberTemper,
		CoolingFanPercent: fanPercent(r.CoolingFanSpeed),
		AuxFanPercent:     fanPercent(r.BigFan1Speed),
		ChamberFanPercent: fanPercent(r.BigFan2Speed),
		SpeedLevel:        r.SpdLvl,
		SpeedLabel:        speedLabels[r.SpdLvl],
		SpeedMag:          r.SpdMag,
		WifiSignal:        r.WifiSignal,
		PrintError:        r.PrintError,
		UpdatedAt:         now.Format(time.RFC3339),
	}

	if r.MCRemainingTime > 0 {
		s.RemainingText = formatDuration(r.MCRemainingTime)
		// Only project a finish time while actively printing; when paused or idle
		// the remaining value is frozen and a wall-clock ETA would be misleading.
		if printing {
			s.FinishTime = now.Add(time.Duration(r.MCRemainingTime) * time.Minute).Format(time.RFC3339)
		}
	}

	s.Trays, s.ActiveFilament, s.ActiveColor = buildTrays(r.AMS)
	return s
}

func buildTrays(ams *AMSReport) ([]TrayStatus, string, string) {
	if ams == nil {
		return nil, "", ""
	}
	activeIdx := -1
	if v, err := strconv.Atoi(strings.TrimSpace(ams.TrayNow)); err == nil {
		activeIdx = v
	}

	var trays []TrayStatus
	var activeType, activeColor string
	for _, unit := range ams.AMS {
		unitID, _ := strconv.Atoi(unit.ID)
		for _, t := range unit.Tray {
			trayID, _ := strconv.Atoi(t.ID)
			global := unitID*4 + trayID
			active := global == activeIdx
			trays = append(trays, TrayStatus{
				Unit:   unitID,
				Tray:   trayID,
				Type:   t.TrayType,
				Color:  normalizeColor(t.TrayColor),
				Remain: t.Remain,
				Active: active,
			})
			if active {
				activeType = t.TrayType
				activeColor = normalizeColor(t.TrayColor)
			}
		}
	}
	return trays, activeType, activeColor
}

// normalizeState maps the printer's UPPERCASE gcode_state into stable lowercase
// values used across MQTT and the UI.
func normalizeState(gcodeState string) string {
	switch strings.ToUpper(strings.TrimSpace(gcodeState)) {
	case "RUNNING":
		return "printing"
	case "PAUSE":
		return "paused"
	case "PREPARE":
		return "prepare"
	case "SLICING":
		return "slicing"
	case "FINISH":
		return "finished"
	case "FAILED":
		return "failed"
	case "IDLE", "":
		return "idle"
	default:
		return strings.ToLower(gcodeState)
	}
}

func clampPercent(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// fanPercent converts a Bambu fan gear value (string, typically 0..15) into a
// rough percentage. Unparseable values yield 0.
func fanPercent(v string) int {
	g, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	if g <= 0 {
		return 0
	}
	pct := int(math.Round(float64(g) / 15.0 * 100.0))
	return min(pct, 100)
}

// normalizeColor trims the alpha channel from an RRGGBBAA hex string and returns
// a "#RRGGBB" CSS color. Empty / all-zero values yield "".
func normalizeColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" || strings.HasPrefix(strings.ToUpper(c), "00000000") {
		return ""
	}
	if len(c) >= 6 {
		return "#" + strings.ToUpper(c[:6])
	}
	return ""
}

// formatDuration renders a minute count as "1h 23m" / "45m".
func formatDuration(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	h := minutes / 60
	m := minutes % 60
	if h > 0 {
		return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
	}
	return strconv.Itoa(m) + "m"
}
