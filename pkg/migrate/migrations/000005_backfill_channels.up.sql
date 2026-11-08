-- Backfill legacy wide `channels` into the canonical v2 tables.
-- Deterministic legacy ids are user_id*10 + channel_code; Snowflake ids are
-- much larger, so there is no collision with application-generated rows.

-- tgbot = 1
INSERT IGNORE INTO `user_channels` (`id`, `user_id`, `channel_type`, `enabled`, `created_at`, `updated_at`)
SELECT `UserId` * 10 + 1, `UserId`, 'tgbot', `TgBot_enabled`, NOW(3), NOW(3)
FROM `channels`
WHERE `TgBot` IS NOT NULL OR `TgBot_enabled` = 1;

INSERT IGNORE INTO `user_channel_configs`
  (`channel_id`, `user_id`, `config_json`, `secret_ciphertext`, `encrypted`, `key_version`, `updated_at`)
SELECT `UserId` * 10 + 1, `UserId`,
  CASE WHEN NOT (`TgBot` LIKE '$mk$%' OR `TgBot` LIKE '$app$%') AND JSON_VALID(`TgBot`) THEN `TgBot` ELSE NULL END,
  CASE WHEN (`TgBot` LIKE '$mk$%' OR `TgBot` LIKE '$app$%') OR NOT JSON_VALID(`TgBot`) THEN `TgBot` ELSE NULL END,
  CASE WHEN (`TgBot` LIKE '$mk$%' OR `TgBot` LIKE '$app$%') THEN 1 ELSE 0 END,
  CASE WHEN (`TgBot` LIKE '$mk$%' OR `TgBot` LIKE '$app$%') THEN 1 ELSE NULL END,
  NOW(3)
FROM `channels`
WHERE `TgBot` IS NOT NULL;

-- widget = 2
INSERT IGNORE INTO `user_channels` (`id`, `user_id`, `channel_type`, `enabled`, `created_at`, `updated_at`)
SELECT `UserId` * 10 + 2, `UserId`, 'widget', `Widget_enabled`, NOW(3), NOW(3)
FROM `channels`
WHERE `Widget` IS NOT NULL OR `Widget_enabled` = 1;

INSERT IGNORE INTO `user_channel_configs`
  (`channel_id`, `user_id`, `config_json`, `secret_ciphertext`, `encrypted`, `key_version`, `updated_at`)
SELECT `UserId` * 10 + 2, `UserId`,
  CASE WHEN NOT (`Widget` LIKE '$mk$%' OR `Widget` LIKE '$app$%') AND JSON_VALID(`Widget`) THEN `Widget` ELSE NULL END,
  CASE WHEN (`Widget` LIKE '$mk$%' OR `Widget` LIKE '$app$%') OR NOT JSON_VALID(`Widget`) THEN `Widget` ELSE NULL END,
  CASE WHEN (`Widget` LIKE '$mk$%' OR `Widget` LIKE '$app$%') THEN 1 ELSE 0 END,
  CASE WHEN (`Widget` LIKE '$mk$%' OR `Widget` LIKE '$app$%') THEN 1 ELSE NULL END,
  NOW(3)
FROM `channels`
WHERE `Widget` IS NOT NULL;

-- tguserbot = 3 (legacy column TgUserBot)
INSERT IGNORE INTO `user_channels` (`id`, `user_id`, `channel_type`, `enabled`, `created_at`, `updated_at`)
SELECT `UserId` * 10 + 3, `UserId`, 'tguserbot', `TgUserBot_enabled`, NOW(3), NOW(3)
FROM `channels`
WHERE `TgUserBot` IS NOT NULL OR `TgUserBot_enabled` = 1;

INSERT IGNORE INTO `user_channel_configs`
  (`channel_id`, `user_id`, `config_json`, `secret_ciphertext`, `encrypted`, `key_version`, `updated_at`)
SELECT `UserId` * 10 + 3, `UserId`,
  CASE WHEN NOT (`TgUserBot` LIKE '$mk$%' OR `TgUserBot` LIKE '$app$%') AND JSON_VALID(`TgUserBot`) THEN `TgUserBot` ELSE NULL END,
  CASE WHEN (`TgUserBot` LIKE '$mk$%' OR `TgUserBot` LIKE '$app$%') OR NOT JSON_VALID(`TgUserBot`) THEN `TgUserBot` ELSE NULL END,
  CASE WHEN (`TgUserBot` LIKE '$mk$%' OR `TgUserBot` LIKE '$app$%') THEN 1 ELSE 0 END,
  CASE WHEN (`TgUserBot` LIKE '$mk$%' OR `TgUserBot` LIKE '$app$%') THEN 1 ELSE NULL END,
  NOW(3)
FROM `channels`
WHERE `TgUserBot` IS NOT NULL;

-- whatsbot = 4 (legacy column Whats)
INSERT IGNORE INTO `user_channels` (`id`, `user_id`, `channel_type`, `enabled`, `created_at`, `updated_at`)
SELECT `UserId` * 10 + 4, `UserId`, 'whatsbot', `Whats_enabled`, NOW(3), NOW(3)
FROM `channels`
WHERE `Whats` IS NOT NULL OR `Whats_enabled` = 1;

INSERT IGNORE INTO `user_channel_configs`
  (`channel_id`, `user_id`, `config_json`, `secret_ciphertext`, `encrypted`, `key_version`, `updated_at`)
SELECT `UserId` * 10 + 4, `UserId`,
  CASE WHEN NOT (`Whats` LIKE '$mk$%' OR `Whats` LIKE '$app$%') AND JSON_VALID(`Whats`) THEN `Whats` ELSE NULL END,
  CASE WHEN (`Whats` LIKE '$mk$%' OR `Whats` LIKE '$app$%') OR NOT JSON_VALID(`Whats`) THEN `Whats` ELSE NULL END,
  CASE WHEN (`Whats` LIKE '$mk$%' OR `Whats` LIKE '$app$%') THEN 1 ELSE 0 END,
  CASE WHEN (`Whats` LIKE '$mk$%' OR `Whats` LIKE '$app$%') THEN 1 ELSE NULL END,
  NOW(3)
FROM `channels`
WHERE `Whats` IS NOT NULL;

-- insta = 5
INSERT IGNORE INTO `user_channels` (`id`, `user_id`, `channel_type`, `enabled`, `created_at`, `updated_at`)
SELECT `UserId` * 10 + 5, `UserId`, 'insta', `Insta_enabled`, NOW(3), NOW(3)
FROM `channels`
WHERE `Insta` IS NOT NULL OR `Insta_enabled` = 1;

INSERT IGNORE INTO `user_channel_configs`
  (`channel_id`, `user_id`, `config_json`, `secret_ciphertext`, `encrypted`, `key_version`, `updated_at`)
SELECT `UserId` * 10 + 5, `UserId`,
  CASE WHEN NOT (`Insta` LIKE '$mk$%' OR `Insta` LIKE '$app$%') AND JSON_VALID(`Insta`) THEN `Insta` ELSE NULL END,
  CASE WHEN (`Insta` LIKE '$mk$%' OR `Insta` LIKE '$app$%') OR NOT JSON_VALID(`Insta`) THEN `Insta` ELSE NULL END,
  CASE WHEN (`Insta` LIKE '$mk$%' OR `Insta` LIKE '$app$%') THEN 1 ELSE 0 END,
  CASE WHEN (`Insta` LIKE '$mk$%' OR `Insta` LIKE '$app$%') THEN 1 ELSE NULL END,
  NOW(3)
FROM `channels`
WHERE `Insta` IS NOT NULL;

-- avito = 6
INSERT IGNORE INTO `user_channels` (`id`, `user_id`, `channel_type`, `enabled`, `created_at`, `updated_at`)
SELECT `UserId` * 10 + 6, `UserId`, 'avito', `Avito_enabled`, NOW(3), NOW(3)
FROM `channels`
WHERE `Avito` IS NOT NULL OR `Avito_enabled` = 1;

INSERT IGNORE INTO `user_channel_configs`
  (`channel_id`, `user_id`, `config_json`, `secret_ciphertext`, `encrypted`, `key_version`, `updated_at`)
SELECT `UserId` * 10 + 6, `UserId`,
  CASE WHEN NOT (`Avito` LIKE '$mk$%' OR `Avito` LIKE '$app$%') AND JSON_VALID(`Avito`) THEN `Avito` ELSE NULL END,
  CASE WHEN (`Avito` LIKE '$mk$%' OR `Avito` LIKE '$app$%') OR NOT JSON_VALID(`Avito`) THEN `Avito` ELSE NULL END,
  CASE WHEN (`Avito` LIKE '$mk$%' OR `Avito` LIKE '$app$%') THEN 1 ELSE 0 END,
  CASE WHEN (`Avito` LIKE '$mk$%' OR `Avito` LIKE '$app$%') THEN 1 ELSE NULL END,
  NOW(3)
FROM `channels`
WHERE `Avito` IS NOT NULL;
