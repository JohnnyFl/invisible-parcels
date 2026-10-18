package contract

import "time"

// Clock is the virtual clock: now = VirtualBase + (real - RealBase) * Speed. Speed 0 = paused.
type Clock struct {
	RealBase    time.Time
	VirtualBase time.Time
	Speed       float64
}

func (c Clock) Now(real time.Time) time.Time {
	return c.VirtualBase.Add(time.Duration(float64(real.Sub(c.RealBase)) * c.Speed))
}

// Rebase pins the current virtual time as the new base and switches speed.
func (c Clock) Rebase(real time.Time, speed float64) Clock {
	return Clock{RealBase: real, VirtualBase: c.Now(real), Speed: speed}
}
