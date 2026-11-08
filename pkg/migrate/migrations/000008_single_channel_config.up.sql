-- Single payload column for user channel configs.
-- The value is either plaintext (user has no MasterKey) or a $mk$-encrypted blob.

ALTER TABLE `user_channel_configs`
  ADD COLUMN `config` MEDIUMTEXT NULL;

UPDATE `user_channel_configs`
SET `config` = COALESCE(`secret_ciphertext`, `config_json`);

ALTER TABLE `user_channel_configs`
  DROP COLUMN `config_json`,
  DROP COLUMN `secret_ciphertext`,
  DROP COLUMN `encrypted`,
  DROP COLUMN `key_version`;
