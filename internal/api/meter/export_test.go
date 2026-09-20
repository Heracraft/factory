package meter

import "time"

// SetNow fixes the ingest clock for tests.
func (i *Ingest) SetNow(f func() time.Time) { i.nowFunc = f }
