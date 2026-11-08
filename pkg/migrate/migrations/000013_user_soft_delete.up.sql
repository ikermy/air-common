-- Phase 4.3: soft-delete users, audit trail and background purge.

ALTER TABLE `users`
  ADD COLUMN `status` ENUM('active','deleted') NOT NULL DEFAULT 'active' AFTER `id_source`,
  ADD COLUMN `deleted_at` TIMESTAMP(3) NULL DEFAULT NULL AFTER `status`,
  ADD KEY `idx_users_status_deleted` (`status`, `deleted_at`);

CREATE TABLE IF NOT EXISTS `audit_log` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NULL,
  `actor_id` BIGINT UNSIGNED NULL,
  `action` VARCHAR(64) NOT NULL,
  `details` JSON NULL,
  `created_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_audit_user` (`user_id`),
  KEY `idx_audit_action` (`action`),
  KEY `idx_audit_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
