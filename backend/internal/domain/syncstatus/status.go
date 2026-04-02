package syncstatus

type PairSyncStatus struct {
	Pair              string `json:"pair"`
	BackfillCompleted bool   `json:"backfill_completed"`
	LastSyncedUnix    int64  `json:"last_synced_unix"`
	NextTargetUnix    int64  `json:"next_target_unix"`
}

type Snapshot struct {
	PlannerLeader string           `json:"planner_leader"`
	Pairs         []PairSyncStatus `json:"pairs"`
}
