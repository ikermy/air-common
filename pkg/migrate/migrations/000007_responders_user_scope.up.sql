-- Phase 4 (expand): responders become user-scoped.
-- user_id stays nullable during expand: unused responders are marked is_orphan.
-- A unique key on (user_id, Type, RealId) is added after the Go backfill.

ALTER TABLE `responders`
  ADD COLUMN `user_id` BIGINT UNSIGNED NULL,
  ADD COLUMN `is_orphan` TINYINT(1) NOT NULL DEFAULT 0,
  ADD KEY `idx_responders_user` (`user_id`, `Type`, `RealId`),
  ADD CONSTRAINT `fk_responders_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE;
