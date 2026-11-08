DROP TABLE IF EXISTS `audit_log`;

ALTER TABLE `users`
  DROP KEY `idx_users_status_deleted`,
  DROP COLUMN `deleted_at`,
  DROP COLUMN `status`;
