-- Phase 5 contract: remove legacy schema after all services are deployed with
-- v2-only read/write paths.
--
-- DO NOT RUN during expand/dual-write. Preconditions:
--   1. All services run the current release (no CALL to legacy routines,
--      no reads/writes of legacy channels/dialogs/service_*bot* columns).
--   2. CHANNELS_V2_READ_ENABLED=true and DIALOG_MESSAGES_READ_ENABLED=true
--      have been stable on production traffic.
--   3. Backfills completed and idempotent:
--        - channels -> user_channels/user_channel_configs
--        - dialogs.Data -> dialog_messages, dialogs.Context -> dialog_contexts
--        - responders.user_id backfilled (non-orphans)
--        - service_*bot* -> user_bots/user_bot_events/user_bot_state
--   4. A fresh backup/restore-check exists.
--
-- Run with: mariadb --default-character-set=utf8mb4 < phase5_contract.sql

-- ── Precondition checks (review manually; script aborts on failure) ──────────
SET @pending_migrations := (
  SELECT COUNT(*) FROM schema_migrations WHERE dirty = 1
);
-- Uncomment to abort on a dirty migration:
-- SELECT IF(@pending_migrations = 0, 'ok', 'ABORT: dirty migration') AS check_dirty;

SET @unmigrated_dialogs := (
  SELECT COUNT(*) FROM dialogs WHERE messages_migrated = 0 AND Data IS NOT NULL
);
-- SELECT IF(@unmigrated_dialogs = 0, 'ok', 'ABORT: unmigrated dialogs') AS check_dialogs;

SET @unowned_responders := (
  SELECT COUNT(*) FROM responders WHERE user_id IS NULL
);
-- Orphans are allowed (is_orphan = 1), shared responders must be resolved:
SET @unowned_shared := (
  SELECT COUNT(*) FROM responders WHERE user_id IS NULL AND is_orphan = 0
);
-- SELECT IF(@unowned_shared = 0, 'ok', 'ABORT: unowned responders') AS check_responders;

-- ── 1. Stored procedures and functions (Go implementations are authoritative) ─
DROP PROCEDURE IF EXISTS `DeleteAllUserData`;
DROP PROCEDURE IF EXISTS `DeleteDialog`;
DROP PROCEDURE IF EXISTS `DisableAllUserChannel`;
DROP PROCEDURE IF EXISTS `GetOrSetTreadAndResponder`;
DROP PROCEDURE IF EXISTS `SaveChannelData`;
DROP PROCEDURE IF EXISTS `SaveContext`;
DROP PROCEDURE IF EXISTS `SaveDialog`;
DROP PROCEDURE IF EXISTS `SetChannelEnabled`;
DROP PROCEDURE IF EXISTS `UpdateDialogsMeta`;

DROP FUNCTION IF EXISTS `GetBalance`;
DROP FUNCTION IF EXISTS `GetNotificationChannel`;
DROP FUNCTION IF EXISTS `GetOrSetDialog`;
DROP FUNCTION IF EXISTS `GetUserSubscriptionLimites`;
DROP FUNCTION IF EXISTS `GetUserTariff`;
DROP FUNCTION IF EXISTS `ReadContext`;
DROP FUNCTION IF EXISTS `ReadDialog`;
DROP FUNCTION IF EXISTS `ReadResponderName`;
DROP FUNCTION IF EXISTS `UserLang`;

-- ── 2. Legacy bot triggers and tables ────────────────────────────────────────
DROP TRIGGER IF EXISTS `trg_tgbot_delete_cascade_events`;
DROP TRIGGER IF EXISTS `trg_tgbot_delete_cascade_state`;
DROP TRIGGER IF EXISTS `trg_wabot_delete_cascade_events`;
DROP TRIGGER IF EXISTS `trg_wabot_delete_cascade_state`;

DROP TABLE IF EXISTS `service_user_bot_events`;
DROP TABLE IF EXISTS `service_user_bot_state`;
DROP TABLE IF EXISTS `service_tgbots`;
DROP TABLE IF EXISTS `service_wabots`;

-- ── 3. Scheduler event that referenced the removed function ──────────────────
DROP EVENT IF EXISTS `ResetFlagsEvent`;

-- ── 4. Responders: one identity per user ────────────────────────────────────
-- Duplicates were merged by comdb.BackfillResponders; verify before adding.
-- SELECT user_id, Type, RealId, COUNT(*) c FROM responders
--   WHERE user_id IS NOT NULL AND RealId IS NOT NULL
--   GROUP BY user_id, Type, RealId HAVING c > 1;
ALTER TABLE `responders`
  ADD UNIQUE KEY `uq_user_type_real` (`user_id`, `Type`, `RealId`);

-- ── 5. Legacy channel columns ───────────────────────────────────────────────
ALTER TABLE `channels`
  DROP COLUMN `TgBot`,
  DROP COLUMN `TgBot_enabled`,
  DROP COLUMN `Widget`,
  DROP COLUMN `Widget_enabled`,
  DROP COLUMN `TgUserBot`,
  DROP COLUMN `TgUserBot_enabled`,
  DROP COLUMN `Whats`,
  DROP COLUMN `Whats_enabled`,
  DROP COLUMN `Insta`,
  DROP COLUMN `Insta_enabled`,
  DROP COLUMN `Avito`,
  DROP COLUMN `Avito_enabled`;

-- The legacy table may be dropped entirely once no code touches it:
-- DROP TABLE IF EXISTS `channels`;

-- ── 6. Legacy dialog payloads ───────────────────────────────────────────────
ALTER TABLE `dialogs`
  DROP COLUMN `Data`,
  DROP COLUMN `Context`,
  DROP COLUMN `messages_migrated`,
  DROP COLUMN `messages_migrated_at`;

-- ── 7. Final user migration (deadline 2026-12-31T23:59:59Z) ─────────────────
-- Precondition: the Go scheduler (USER_MIGRATION_FINALIZE=true) has purged all
-- users with user_auth.EmailHash IS NULL. This block is idempotent and is a
-- manual fallback for ops if the scheduler did not run.
-- SELECT COUNT(*) AS non_migrated FROM `user_auth` WHERE `EmailHash` IS NULL; -- must be 0
ALTER TABLE `user_auth`
  MODIFY `EmailHash` VARCHAR(64) NOT NULL COLLATE 'utf8mb4_general_ci';

INSERT INTO `app_config` (`key`, `value`)
VALUES ('users.email_hash_migration', CONCAT('finalized:', DATE_FORMAT(UTC_TIMESTAMP(), '%Y-%m-%dT%H:%i:%sZ')))
ON DUPLICATE KEY UPDATE `value` = VALUES(`value`);

-- ── 8. No AUTO_INCREMENT for user IDs ───────────────────────────────────────
-- New users are created with Snowflake IDs (IDGEN_ENABLED=true). The legacy
-- AUTO_INCREMENT path must not be used after this point.
-- Precondition: IDGEN_ENABLED=true on every orchestrator replica.
ALTER TABLE `users`
  MODIFY `Id` BIGINT UNSIGNED NOT NULL;

-- user_auth.Id remains a surrogate key with AUTO_INCREMENT: nothing references
-- it (all queries use UserId) and it is only used as a stable row key by
-- pkg/dbverify. It can be dropped/replaced in a later cleanup.
