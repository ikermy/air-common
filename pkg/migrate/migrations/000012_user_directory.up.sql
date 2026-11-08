-- Phase 6: global user directory for slot-based shard routing.
-- Populated by comdb.BackfillUserDirectory because slot = xxhash64(user_id) & 1023
-- is computed in Go.

CREATE TABLE `user_directory` (
  `user_id` BIGINT UNSIGNED NOT NULL,
  `email_hash` VARBINARY(32) NOT NULL,
  `slot` SMALLINT UNSIGNED NOT NULL,
  `shard_id` SMALLINT UNSIGNED NOT NULL,
  `created_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`user_id`),
  UNIQUE KEY `uq_email_hash` (`email_hash`),
  KEY `idx_slot` (`slot`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
