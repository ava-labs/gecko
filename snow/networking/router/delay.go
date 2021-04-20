package router

import "time"

// Delay tracks the amount of time to delay future messages.
type Delay struct {
	waitUntil time.Time
}

func (d *Delay) Delay(delay time.Duration) {
	d.waitUntil = time.Now().Add(delay)
}
