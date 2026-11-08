-- Legacy bot-delete triggers used OLD.BotId (external id), while
-- service_user_bot_events/state.BotId stores service_*bots.Id (internal id).
-- Replace them with correct OLD.Id triggers; the trg_wabot_* triggers already
-- use OLD.Id and are kept.
DROP TRIGGER IF EXISTS `tg_delete_tgbot_events`;
DROP TRIGGER IF EXISTS `tg_delete_tgbot_state`;
DROP TRIGGER IF EXISTS `tg_delete_wabot_events`;
DROP TRIGGER IF EXISTS `tg_delete_wabot_state`;

CREATE TRIGGER `trg_tgbot_delete_cascade_events` AFTER DELETE ON `service_tgbots`
FOR EACH ROW
DELETE FROM `service_user_bot_events` WHERE `BotType` = 'telegram' AND `BotId` = OLD.`Id`;

CREATE TRIGGER `trg_tgbot_delete_cascade_state` AFTER DELETE ON `service_tgbots`
FOR EACH ROW
DELETE FROM `service_user_bot_state` WHERE `BotType` = 'telegram' AND `BotId` = OLD.`Id`;
