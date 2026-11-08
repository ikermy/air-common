ALTER TABLE `user_channel_configs`
  ADD COLUMN `config_json` JSON NULL,
  ADD COLUMN `secret_ciphertext` MEDIUMTEXT NULL,
  ADD COLUMN `encrypted` TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN `key_version` SMALLINT(5) UNSIGNED NULL;

UPDATE `user_channel_configs`
SET `secret_ciphertext` = `config`,
    `encrypted` = 1
WHERE `config` LIKE '$mk$%';

UPDATE `user_channel_configs`
SET `config_json` = `config`
WHERE `config` NOT LIKE '$mk$%' AND JSON_VALID(`config`);

ALTER TABLE `user_channel_configs`
  DROP COLUMN `config`;
