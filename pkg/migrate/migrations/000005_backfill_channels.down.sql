-- Remove only deterministic legacy backfill rows, not Snowflake-created rows.
DELETE ucc FROM `user_channel_configs` ucc
JOIN `user_channels` uc ON uc.id = ucc.channel_id
WHERE uc.id < 1000000000000
  AND uc.channel_type IN ('tgbot','widget','tguserbot','whatsbot','insta','avito');

DELETE FROM `user_channels`
WHERE id < 1000000000000
  AND channel_type IN ('tgbot','widget','tguserbot','whatsbot','insta','avito');
