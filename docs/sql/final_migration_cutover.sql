-- Final user migration cutover (deadline: 2026-12-31T23:59:59Z).
--
-- The orchestrator Go scheduler performs the same steps automatically when
-- USER_MIGRATION_FINALIZE=true and the deadline has passed:
--   1. hard-delete every user with user_auth.EmailHash IS NULL (and all data);
--   2. ALTER user_auth.EmailHash to NOT NULL;
--   3. store 'users.email_hash_migration=finalized:<ts>' in app_config.
--
-- This script is a manual fallback/verification path for ops. It is NOT applied
-- by pkg/migrate. Run it only after confirming that non_migrated = 0.
-- It is safe to re-run.

-- 1. Preconditions ───────────────────────────────────────────────────────────
SELECT COUNT(*) AS non_migrated_users FROM `user_auth` WHERE `EmailHash` IS NULL;
-- If the value above is > 0, let the orchestrator purge finish first or delete
-- the remaining users explicitly:
--   SELECT u.Id, u.Name FROM users u JOIN user_auth a ON a.UserId = u.Id
--   WHERE a.EmailHash IS NULL;

SELECT COUNT(*) AS dry_run_migrations
FROM `schema_migrations` WHERE `dirty` = 1;
-- Must be 0.

-- 2. Finalize EmailHash ──────────────────────────────────────────────────────
ALTER TABLE `user_auth`
  MODIFY `EmailHash` VARCHAR(64) NOT NULL COLLATE 'utf8mb4_general_ci';

INSERT INTO `app_config` (`key`, `value`)
VALUES ('users.email_hash_migration', CONCAT('finalized:', DATE_FORMAT(UTC_TIMESTAMP(), '%Y-%m-%dT%H:%i:%sZ')))
ON DUPLICATE KEY UPDATE `value` = VALUES(`value`);

-- 3. Stop AUTO_INCREMENT for users.Id ───────────────────────────────────────
-- Requires IDGEN_ENABLED=true on all orchestrator replicas.
ALTER TABLE `users`
  MODIFY `Id` BIGINT UNSIGNED NOT NULL;

-- 4. Verification ───────────────────────────────────────────────────────────
SELECT `value` FROM `app_config` WHERE `key` = 'users.email_hash_migration';
SELECT COUNT(*) AS non_migrated_after FROM `user_auth` WHERE `EmailHash` IS NULL;
SELECT COUNT(*) AS legacy_users
FROM `users` WHERE `id_source` = 'legacy';
-- Legacy rows are allowed (their IDs are not rewritten), but no new users may be
-- created through the legacy path.
