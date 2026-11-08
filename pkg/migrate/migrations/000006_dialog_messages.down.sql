DROP TABLE IF EXISTS `dialog_contexts`;
DROP TABLE IF EXISTS `dialog_messages`;

ALTER TABLE `dialogs`
  DROP COLUMN IF EXISTS `messages_migrated_at`,
  DROP COLUMN IF EXISTS `messages_migrated`;
