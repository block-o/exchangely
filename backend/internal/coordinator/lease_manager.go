package coordinator

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
)

type LeaseManager struct {
	instanceID string
	name       string
	ttl        time.Duration
}

func NewLeaseManager(instanceID, name string, ttl time.Duration) *LeaseManager {
	return &LeaseManager{
		instanceID: instanceID,
		name:       name,
		ttl:        ttl,
	}
}

func (m *LeaseManager) CurrentLease() lease.Lease {
	now := time.Now().UTC()
	return lease.Lease{
		Name:       m.name,
		HolderID:   m.instanceID,
		ExpiresAt:  now.Add(m.ttl),
		LastBeatAt: now,
	}
}

func (m *LeaseManager) Heartbeat(_ context.Context) lease.Lease {
	return m.CurrentLease()
}
