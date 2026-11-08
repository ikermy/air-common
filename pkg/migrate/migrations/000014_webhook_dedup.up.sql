-- Webhook idempotency: Telegram retries deliveries when an ack is lost.

CREATE TABLE IF NOT EXISTS `webhook_dedup` (
  `source` VARCHAR(32) NOT NULL,
  `event_id` VARCHAR(128) NOT NULL,
  `created_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`source`, `event_id`),
  KEY `idx_webhook_dedup_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
