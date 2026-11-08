-- Phase 2 (expand): canonical channel model. Legacy `channels` is kept until
-- dual-write and backfill are complete.

CREATE TABLE `user_channels` (
  `id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `channel_type` VARCHAR(32) NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `external_id` VARCHAR(255) NULL,
  `created_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_user_channel` (`user_id`, `channel_type`),
  KEY `idx_type_enabled` (`channel_type`, `enabled`),
  KEY `idx_external` (`channel_type`, `external_id`),
  CONSTRAINT `fk_user_channels_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE `user_channel_configs` (
  `channel_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `config_json` JSON NULL,
  `secret_ciphertext` MEDIUMTEXT NULL,
  `encrypted` TINYINT(1) NOT NULL DEFAULT 0,
  `key_version` SMALLINT UNSIGNED NULL,
  `updated_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`channel_id`),
  KEY `idx_channel_cfg_user` (`user_id`),
  CONSTRAINT `fk_channel_cfg_channel` FOREIGN KEY (`channel_id`) REFERENCES `user_channels` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_channel_cfg_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
