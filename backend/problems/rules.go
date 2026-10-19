package problems

import "time"

// Thresholds (spec §6). Tuned for the demo; real values should come from route delay data.
const (
	OverdueAfter   = 2 * time.Hour  // after ETA
	StuckAfter     = 12 * time.Hour // without any event
	UnloadGrace    = 1 * time.Hour  // after a trip arrives, before a missing unload scan counts as a broken chain
	EscalateOrange = 6 * time.Hour  // orange -> red
	EscalateYellow = 24 * time.Hour // yellow -> orange
)
