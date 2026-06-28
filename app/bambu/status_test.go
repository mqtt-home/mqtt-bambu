package bambu

import (
	"testing"
	"time"
)

func TestStateCacheMergePartialReports(t *testing.T) {
	c := newStateCache()

	// Full snapshot.
	if !c.merge([]byte(`{"print":{"gcode_state":"RUNNING","mc_remaining_time":90,"mc_percent":10,"subtask_name":"benchy.gcode"}}`)) {
		t.Fatal("expected first merge to report a print payload")
	}
	// Partial delta: only the remaining time changes.
	if !c.merge([]byte(`{"print":{"mc_remaining_time":75,"mc_percent":20}}`)) {
		t.Fatal("expected second merge to report a print payload")
	}
	// A non-print message must be ignored.
	if c.merge([]byte(`{"info":{"command":"get_version"}}`)) {
		t.Fatal("non-print payload should not merge")
	}

	r := c.report()
	if r.MCRemainingTime != 75 {
		t.Errorf("remaining: got %d want 75", r.MCRemainingTime)
	}
	if r.MCPercent != 20 {
		t.Errorf("percent: got %d want 20", r.MCPercent)
	}
	// Field not present in the delta must persist from the snapshot.
	if r.GCodeState != "RUNNING" {
		t.Errorf("gcode_state: got %q want RUNNING", r.GCodeState)
	}
	if r.SubtaskName != "benchy.gcode" {
		t.Errorf("subtask_name: got %q want benchy.gcode", r.SubtaskName)
	}
}

func TestBuildStatusRemainingAndETA(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	r := Report{
		GCodeState:      "RUNNING",
		MCRemainingTime: 83,
		MCPercent:       42,
		LayerNum:        50,
		TotalLayerNum:   120,
		SubtaskName:     "calibration.gcode",
		NozzleTemper:    220.5,
		CoolingFanSpeed: "15",
		SpdLvl:          2,
	}

	s := buildStatus("X1C", "X1 Carbon", "SERIAL123", r, now)

	if s.State != "printing" || !s.Printing {
		t.Errorf("state: got %q/%v want printing/true", s.State, s.Printing)
	}
	if s.RemainingMinutes != 83 {
		t.Errorf("remaining minutes: got %d want 83", s.RemainingMinutes)
	}
	if s.RemainingText != "1h 23m" {
		t.Errorf("remaining text: got %q want 1h 23m", s.RemainingText)
	}
	wantFinish := now.Add(83 * time.Minute).Format(time.RFC3339)
	if s.FinishTime != wantFinish {
		t.Errorf("finish time: got %q want %q", s.FinishTime, wantFinish)
	}
	if s.JobName != "calibration" {
		t.Errorf("job name: got %q want calibration", s.JobName)
	}
	if s.SpeedLabel != "standard" {
		t.Errorf("speed label: got %q want standard", s.SpeedLabel)
	}
	if s.CoolingFanPercent != 100 {
		t.Errorf("cooling fan percent: got %d want 100", s.CoolingFanPercent)
	}
}

func TestBuildStatusNoETAWhenPaused(t *testing.T) {
	now := time.Now()
	s := buildStatus("X1C", "X1 Carbon", "S", Report{GCodeState: "PAUSE", MCRemainingTime: 30}, now)
	if s.State != "paused" {
		t.Errorf("state: got %q want paused", s.State)
	}
	if s.FinishTime != "" {
		t.Errorf("expected no finish time while paused, got %q", s.FinishTime)
	}
	if s.RemainingText != "30m" {
		t.Errorf("remaining text: got %q want 30m", s.RemainingText)
	}
}

func TestNormalizeColor(t *testing.T) {
	cases := map[string]string{
		"FF8800FF": "#FF8800",
		"00000000": "",
		"":         "",
		"AABBCC":   "#AABBCC",
	}
	for in, want := range cases {
		if got := normalizeColor(in); got != want {
			t.Errorf("normalizeColor(%q): got %q want %q", in, got, want)
		}
	}
}
