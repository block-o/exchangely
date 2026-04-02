package syncstatus

type PairSyncStatus struct {
	Pair                    string `json:"pair"`
	BackfillCompleted       bool   `json:"backfill_completed"`
	LastSyncedUnix          int64  `json:"last_synced_unix"`
	NextTargetUnix          int64  `json:"next_target_unix"`
	HourlyBackfillCompleted bool   `json:"hourly_backfill_completed"`
	DailyBackfillCompleted  bool   `json:"daily_backfill_completed"`
	HourlySyncedUnix        int64  `json:"hourly_synced_unix"`
	DailySyncedUnix         int64  `json:"daily_synced_unix"`
	NextHourlyTargetUnix    int64  `json:"next_hourly_target_unix"`
	NextDailyTargetUnix     int64  `json:"next_daily_target_unix"`
}

type Snapshot struct {
	PlannerLeader string           `json:"planner_leader"`
	Pairs         []PairSyncStatus `json:"pairs"`
}
