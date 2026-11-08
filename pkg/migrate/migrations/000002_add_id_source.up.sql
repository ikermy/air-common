-- Track whether an entity ID was created by the legacy AUTO_INCREMENT path or
-- by the application Snowflake generator. This is diagnostic/audit metadata.

ALTER TABLE `users`
  ADD COLUMN `id_source` ENUM('legacy','snowflake') NOT NULL DEFAULT 'legacy' AFTER `Id`;

ALTER TABLE `dialogs`
  ADD COLUMN `id_source` ENUM('legacy','snowflake') NOT NULL DEFAULT 'legacy' AFTER `Id`;

ALTER TABLE `responders`
  ADD COLUMN `id_source` ENUM('legacy','snowflake') NOT NULL DEFAULT 'legacy' AFTER `Id`;
