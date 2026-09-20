package auth

import "time"

// SetClockForTest moves the verifier's clock so cache ages can be tested.
func SetClockForTest(v *Verifier, now time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.nowFunc = func() time.Time { return now }
}
