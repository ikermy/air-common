ALTER TABLE `responders`
  DROP FOREIGN KEY `fk_responders_user`;

ALTER TABLE `responders`
  DROP INDEX IF EXISTS `idx_responders_user`,
  DROP COLUMN IF EXISTS `is_orphan`,
  DROP COLUMN IF EXISTS `user_id`;
