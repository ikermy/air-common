-- ResetFlagsEvent calls _scheduler_ResetUserFlags, which is absent from the
-- schema dump and refers to columns that no longer exist. Disable it until the
-- original logic is recovered from production and moved to a Go cron job.
ALTER EVENT `ResetFlagsEvent` DISABLE;
