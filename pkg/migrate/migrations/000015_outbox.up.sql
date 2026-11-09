-- Phase 8: transactional outbox for billing/CRM/notification events.
-- Rows are written in the same transaction as the domain change and dispatched
-- asynchronously with at-least-once semantics.
CREATE TABLE IF NOT EXISTS `outbox` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `aggregate_type` VARCHAR(64) NOT NULL,
  `aggregate_id` VARCHAR(128) NOT NULL,
  `event_type` VARCHAR(64) NOT NULL,
  `payload` JSON NOT NULL,
  `status` ENUM('pending','processing','done','failed') NOT NULL DEFAULT 'pending',
  `attempts` INT UNSIGNED NOT NULL DEFAULT 0,
  `available_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `locked_until` TIMESTAMP(3) NULL DEFAULT NULL,
  `last_error` TEXT NULL,
  `created_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_outbox_event` (`aggregate_type`, `aggregate_id`, `event_type`),
  KEY `idx_outbox_ready` (`status`, `available_at`),
  KEY `idx_outbox_locked` (`status`, `locked_until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
