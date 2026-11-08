DROP TRIGGER IF EXISTS `trg_tgbot_delete_cascade_events`;
DROP TRIGGER IF EXISTS `trg_tgbot_delete_cascade_state`;

-- Restore the original (legacy) triggers.
CREATE TRIGGER `tg_delete_tgbot_events` AFTER DELETE ON `service_tgbots`
FOR EACH ROW
DELETE FROM `service_user_bot_events` WHERE `BotType` = 'telegram' AND `BotId` = OLD.`BotId`;

CREATE TRIGGER `tg_delete_tgbot_state` AFTER DELETE ON `service_tgbots`
FOR EACH ROW
DELETE FROM `service_user_bot_state` WHERE `BotType` = 'telegram' AND `BotId` = OLD.`BotId`;

CREATE TRIGGER `tg_delete_wabot_events` AFTER DELETE ON `service_wabots`
FOR EACH ROW
DELETE FROM `service_user_bot_events` WHERE `BotType` = 'whatsapp' AND `BotId` = OLD.`BotId`;

CREATE TRIGGER `tg_delete_wabot_state` AFTER DELETE ON `service_wabots`
FOR EACH ROW
DELETE FROM `service_user_bot_state` WHERE `BotType` = 'whatsapp' AND `BotId` = OLD.`BotId`;
