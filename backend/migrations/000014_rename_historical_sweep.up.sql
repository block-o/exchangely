UPDATE tasks SET task_type = 'historical_backfill' WHERE task_type = 'historical_sweep';
