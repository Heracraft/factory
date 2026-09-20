package db

import "time"

// nowFunc is swapped by tests that need partitions for another month.
var nowFunc = time.Now
