-- Phase 3 (expand): normalize dialog history and contexts.
-- Legacy `dialogs`.`Data`/`Context` stay in place until the contract phase.

ALTER TABLE `dialogs`
  ADD COLUMN `messages_migrated` TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN `messages_migrated_at` TIMESTAMP(3) NULL;

CREATE TABLE IF NOT EXISTS `dialog_messages` (
  `id` BIGINT UNSIGNED NOT NULL,
  `dialog_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `seq` BIGINT UNSIGNED NOT NULL,
  `role` ENUM('user','assistant','system','tool') NOT NULL,
  `content_type` ENUM('text','audio','image','file','json') NOT NULL DEFAULT 'text',
  `content` MEDIUMTEXT NULL,
  `content_ciphertext` MEDIUMTEXT NULL,
  `content_json` JSON NULL,
  `provider` VARCHAR(32) NULL,
  `model` VARCHAR(128) NULL,
  `tokens_in` INT UNSIGNED NULL,
  `tokens_out` INT UNSIGNED NULL,
  `created_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_dialog_seq` (`dialog_id`, `seq`),
  KEY `idx_user_created` (`user_id`, `created_at`),
  KEY `idx_dialog_created` (`dialog_id`, `created_at`),
  CONSTRAINT `fk_messages_dialog` FOREIGN KEY (`dialog_id`) REFERENCES `dialogs` (`Id`) ON DELETE CASCADE,
  CONSTRAINT `fk_messages_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE IF NOT EXISTS `dialog_contexts` (
  `dialog_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `provider` VARCHAR(32) NOT NULL,
  `context_json` JSON NULL,
  `updated_at` TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`dialog_id`, `provider`),
  KEY `idx_context_user` (`user_id`),
  CONSTRAINT `fk_context_dialog` FOREIGN KEY (`dialog_id`) REFERENCES `dialogs` (`Id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
