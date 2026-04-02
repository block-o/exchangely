package lease

import "time"

type Lease struct {
	Name       string
	HolderID   string
	ExpiresAt  time.Time
	LastBeatAt time.Time
}
