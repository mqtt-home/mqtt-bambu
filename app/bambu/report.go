package bambu

import (
	"encoding/json"
	"maps"
	"sync"
)

// reportEnvelope is the top-level shape of a message on device/{serial}/report.
// We only care about the "print" object; other roots (info, system, ...) are
// ignored for status purposes.
type reportEnvelope struct {
	Print json.RawMessage `json:"print"`
}

// stateCache accumulates the partial "print" deltas the printer sends into a
// single merged object. The printer only emits a full snapshot in response to a
// pushall request; every other message contains just the changed keys, so we
// merge incoming top-level keys over the cached object and decode the result.
type stateCache struct {
	mu    sync.Mutex
	print map[string]json.RawMessage
}

func newStateCache() *stateCache {
	return &stateCache{print: make(map[string]json.RawMessage)}
}

// merge applies a raw report payload. It returns true when the payload carried a
// "print" object (i.e. something status-relevant changed).
func (c *stateCache) merge(payload []byte) bool {
	var env reportEnvelope
	if err := json.Unmarshal(payload, &env); err != nil || len(env.Print) == 0 {
		return false
	}

	var delta map[string]json.RawMessage
	if err := json.Unmarshal(env.Print, &delta); err != nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	maps.Copy(c.print, delta)
	return true
}

// report decodes the merged state into a typed Report snapshot.
func (c *stateCache) report() Report {
	c.mu.Lock()
	merged, _ := json.Marshal(c.print)
	c.mu.Unlock()

	var r Report
	_ = json.Unmarshal(merged, &r)
	return r
}

// Report is the merged, typed view of the printer's "print" object. Field names
// mirror the Bambu cloud MQTT protocol. Fan speeds and several AMS values arrive
// as strings, so they are typed accordingly.
type Report struct {
	// Job progress / time.
	MCRemainingTime int    `json:"mc_remaining_time"` // minutes
	MCPercent       int    `json:"mc_percent"`        // 0..100
	LayerNum        int    `json:"layer_num"`
	TotalLayerNum   int    `json:"total_layer_num"`
	GCodeState      string `json:"gcode_state"` // IDLE/PREPARE/RUNNING/PAUSE/FINISH/FAILED/SLICING
	PrintError      int    `json:"print_error"`
	PrintType       string `json:"print_type"`
	SubtaskName     string `json:"subtask_name"`
	GCodeFile       string `json:"gcode_file"`
	MCPrintStage    string `json:"mc_print_stage"`
	StgCur          int    `json:"stg_cur"`

	// Temperatures (°C).
	NozzleTemper       float64 `json:"nozzle_temper"`
	NozzleTargetTemper float64 `json:"nozzle_target_temper"`
	BedTemper          float64 `json:"bed_temper"`
	BedTargetTemper    float64 `json:"bed_target_temper"`
	ChamberTemper      float64 `json:"chamber_temper"`

	// Fans (string percentages / gear values).
	CoolingFanSpeed   string `json:"cooling_fan_speed"`
	BigFan1Speed      string `json:"big_fan1_speed"`
	BigFan2Speed      string `json:"big_fan2_speed"`
	HeatbreakFanSpeed string `json:"heatbreak_fan_speed"`

	// Speed.
	SpdLvl int `json:"spd_lvl"`
	SpdMag int `json:"spd_mag"`

	// Misc.
	WifiSignal     string        `json:"wifi_signal"`
	LightsReport   []LightReport `json:"lights_report"`
	AMS            *AMSReport    `json:"ams"`
	NozzleDiameter string        `json:"nozzle_diameter"`
}

type LightReport struct {
	Node string `json:"node"`
	Mode string `json:"mode"`
}

// AMSReport is the top-level AMS object; it nests one entry per AMS unit.
type AMSReport struct {
	AMS     []AMSUnit `json:"ams"`
	TrayNow string    `json:"tray_now"`
	TrayTar string    `json:"tray_tar"`
}

type AMSUnit struct {
	ID       string    `json:"id"`
	Humidity string    `json:"humidity"`
	Temp     string    `json:"temp"`
	Tray     []AMSTray `json:"tray"`
}

type AMSTray struct {
	ID          string `json:"id"`
	TrayType    string `json:"tray_type"`
	TrayColor   string `json:"tray_color"` // RRGGBBAA hex
	TrayInfoIdx string `json:"tray_info_idx"`
	Remain      int    `json:"remain"` // percent (unreliable on AMS-lite)
}
