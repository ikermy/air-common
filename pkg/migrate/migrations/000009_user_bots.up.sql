-- Phase 4.2: user-scoped bots, events and state.
-- Replaces service_tgbots/service_wabots and service_user_bot_events/state.

CREATE TABLE IF NOT EXISTS `user_bots` (
  `id`              BIGINT UNSIGNED NOT NULL,
  `user_id`         BIGINT UNSIGNED NOT NULL,
  `bot_type`        ENUM('telegram','whatsapp') NOT NULL,
  `external_bot_id` BIGINT NOT NULL,
  `alias`           VARCHAR(100) NOT NULL,
  `auth_ciphertext` MEDIUMTEXT NOT NULL,
  `is_active`       TINYINT(1) NOT NULL DEFAULT 1,
  `created_at`      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_user_bot` (`user_id`, `bot_type`, `external_bot_id`),
  UNIQUE KEY `uq_user_alias` (`user_id`, `bot_type`, `alias`),
  KEY `idx_user_active` (`user_id`, `is_active`),
  CONSTRAINT `fk_user_bots_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE IF NOT EXISTS `user_bot_events` (
  `id`            BIGINT UNSIGNED NOT NULL,
  `user_id`       BIGINT UNSIGNED NOT NULL,
  `user_bot_id`   BIGINT UNSIGNED NOT NULL,
  `event_type`    ENUM('state','error','warning','info') NOT NULL,
  `cause_group`   ENUM('successes','auth_key','session','phone','password','api','proxy','user_block','chat_access','content','media','rate_limit','retry','other') NULL,
  `cause_code`    VARCHAR(64) NULL,
  `description`   VARCHAR(255) NULL,
  `context_json`  JSON NULL,
  `severity`      ENUM('fatal','start_block','send_block','recoverable') NOT NULL DEFAULT 'recoverable',
  `wait_until`    TIMESTAMP(3) NULL,
  `new_is_active` TINYINT(1) NULL,
  `created_at`    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_bot_time` (`user_bot_id`, `created_at`),
  KEY `idx_user_time` (`user_id`, `created_at`),
  CONSTRAINT `fk_bot_events_bot` FOREIGN KEY (`user_bot_id`) REFERENCES `user_bots` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE IF NOT EXISTS `user_bot_state` (
  `user_bot_id`      BIGINT UNSIGNED NOT NULL,
  `user_id`          BIGINT UNSIGNED NOT NULL,
  `can_start`        TINYINT(1) NOT NULL DEFAULT 1,
  `can_send`         TINYINT(1) NOT NULL DEFAULT 1,
  `last_error_group` ENUM('auth_key','session','phone','password','api','user_block','chat_access','content','media','rate_limit','other') NULL,
  `last_error_code`  VARCHAR(64) NULL,
  `last_error_at`    TIMESTAMP(3) NULL,
  `rate_limit_until` TIMESTAMP(3) NULL,
  `updated_at`       TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`user_bot_id`),
  KEY `idx_user` (`user_id`),
  KEY `idx_state_flags` (`can_start`, `can_send`),
  KEY `idx_rate` (`rate_limit_until`),
  CONSTRAINT `fk_bot_state_bot` FOREIGN KEY (`user_bot_id`) REFERENCES `user_bots` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
