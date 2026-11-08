-- Phase 1: widen entity and foreign-key columns to BIGINT UNSIGNED.
-- Existing numeric values fit without rewriting. AUTO_INCREMENT is kept as a
-- transition fallback; application-generated Snowflake IDs are used for new rows.

-- Drop valid FKs before widening parent/child columns (MariaDB refuses to modify
-- a column that participates in a FK even with foreign_key_checks=0).

ALTER TABLE `billing` DROP FOREIGN KEY `FK_billing_currency`;
ALTER TABLE `billing` DROP FOREIGN KEY `FK_billing_users`;
ALTER TABLE `channels` DROP FOREIGN KEY `FK_channels_users`;
ALTER TABLE `crm_configs` DROP FOREIGN KEY `FK_crm_configs_users`;
ALTER TABLE `crm_mappings` DROP FOREIGN KEY `crm_mappings_ibfk_1`;
ALTER TABLE `crm_mappings` DROP FOREIGN KEY `crm_mappings_ibfk_2`;
ALTER TABLE `crm_mappings` DROP FOREIGN KEY `fk_crm_mappings_application`;
ALTER TABLE `crm_mappings` DROP FOREIGN KEY `fk_crm_mappings_crm_config`;
ALTER TABLE `crm_mappings` DROP FOREIGN KEY `FK_crm_mappings_users`;
ALTER TABLE `crm_oauth_states` DROP FOREIGN KEY `FK_crm_oauth_states_users`;
ALTER TABLE `crypto_payments` DROP FOREIGN KEY `fk_crypto_payments_user`;
ALTER TABLE `dialogs` DROP FOREIGN KEY `fk_chat_type`;
ALTER TABLE `dialogs` DROP FOREIGN KEY `fk_responder_id`;
ALTER TABLE `dialogs` DROP FOREIGN KEY `fk_user_id`;
ALTER TABLE `gpt_models` DROP FOREIGN KEY `FK_provider`;
ALTER TABLE `notifications` DROP FOREIGN KEY `FK__users`;
ALTER TABLE `operators` DROP FOREIGN KEY `FK_userId`;
ALTER TABLE `realtime_models` DROP FOREIGN KEY `FK_provider`;
ALTER TABLE `responders` DROP FOREIGN KEY `fk_chat_type_for_responders`;
ALTER TABLE `service` DROP FOREIGN KEY `fk_service_user`;
ALTER TABLE `service_contacts` DROP FOREIGN KEY `FK1_uid`;
ALTER TABLE `service_contact_availability` DROP FOREIGN KEY `fk_availability_contact`;
ALTER TABLE `service_dialogs` DROP FOREIGN KEY `FK2_resp`;
ALTER TABLE `service_dialogs` DROP FOREIGN KEY `fk_service_dialogs_dialog`;
ALTER TABLE `service_gpt` DROP FOREIGN KEY `FK_service_gpt_model`;
ALTER TABLE `service_gpt` DROP FOREIGN KEY `FK_service_gpt_userid`;
ALTER TABLE `service_proxy` DROP FOREIGN KEY `fk_service_proxy_user`;
ALTER TABLE `service_tgbots` DROP FOREIGN KEY `fk_service_tgbots_user`;
ALTER TABLE `service_wabots` DROP FOREIGN KEY `fk_service_wabots_user`;
ALTER TABLE `storage_migrations` DROP FOREIGN KEY `fk_storage_migration_user`;
ALTER TABLE `subscriptions` DROP FOREIGN KEY `subscriptions_ibfk_1`;
ALTER TABLE `users` DROP FOREIGN KEY `fk_currency_name`;
ALTER TABLE `users` DROP FOREIGN KEY `fk_lang`;
ALTER TABLE `users` DROP FOREIGN KEY `fk_role`;
ALTER TABLE `user_api_keys` DROP FOREIGN KEY `fk_uak_user`;
ALTER TABLE `user_auth` DROP FOREIGN KEY `user_auth_ibfk_1`;
ALTER TABLE `user_gpt` DROP FOREIGN KEY `FK_user_gpt_gpt_models`;
ALTER TABLE `user_gpt` DROP FOREIGN KEY `fk_user_gpt_model_v2`;
ALTER TABLE `user_gpt` DROP FOREIGN KEY `fk_user_gpt_provider`;
ALTER TABLE `user_gpt` DROP FOREIGN KEY `fk_user_gpt_provider_v2`;
ALTER TABLE `user_gpt` DROP FOREIGN KEY `fk_user_gpt_realtime_v2`;
ALTER TABLE `user_models` DROP FOREIGN KEY `fk_user_models_model`;
ALTER TABLE `user_models` DROP FOREIGN KEY `fk_user_models_provider`;
ALTER TABLE `user_models` DROP FOREIGN KEY `fk_user_models_user`;
ALTER TABLE `user_storage_config` DROP FOREIGN KEY `fk_storage_config_user`;
ALTER TABLE `user_storage_quota` DROP FOREIGN KEY `fk_storage_quota_user`;
ALTER TABLE `vector_embeddings` DROP FOREIGN KEY `fk_embeddings_model`;
ALTER TABLE `voice_models` DROP FOREIGN KEY `FK_voice_provider`;

-- Phase 1: widen entity and foreign-key columns to BIGINT UNSIGNED.
-- Existing numeric values fit without rewriting. AUTO_INCREMENT is kept as a
-- transition fallback; application-generated Snowflake IDs are used for new rows.

ALTER TABLE `users`             MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `billing`           MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `channels`          MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `crm_configs`       MODIFY `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `crm_mappings`      MODIFY `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `user_id` BIGINT UNSIGNED NOT NULL,
                                MODIFY `crm_config_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `crm_oauth_states`  MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `crypto_payments`   MODIFY `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `dialogs`           MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `User` BIGINT UNSIGNED NOT NULL,
                                MODIFY `Responder` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `google_oauth_tokens` MODIFY `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                  MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `notifications`     MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `operators`         MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `responders`        MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `service`           MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `service_contacts`  MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `service_contact_availability` MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                           MODIFY `ContactId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE service_dialogs   MODIFY Id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY DialogId BIGINT UNSIGNED NULL,
                                MODIFY `Responder` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `service_gpt`       MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL,
                                MODIFY `ModelId` BIGINT UNSIGNED NULL;
ALTER TABLE `service_proxy`     MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `service_tgbots`    MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `service_wabots`    MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `storage_migrations` MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `subscriptions`     MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `user_api_keys`     MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `user_auth`         MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `user_gpt`          MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `user_models`       MODIFY `Id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                MODIFY `UserId` BIGINT UNSIGNED NOT NULL,
                                MODIFY `ModelId` BIGINT UNSIGNED NULL;
ALTER TABLE `user_storage_config` MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `user_storage_quota`  MODIFY `user_id` BIGINT UNSIGNED NOT NULL;
ALTER TABLE `vector_embeddings`   MODIFY `user_id` BIGINT UNSIGNED NOT NULL,
                                  MODIFY `model_id` BIGINT UNSIGNED NOT NULL;

-- Recreate valid FKs. Broken crm_applications FKs and duplicate user_gpt/crm_mappings
-- FKs are intentionally not recreated and must be resolved separately (see plan G2).

-- Data repair: legacy user_gpt.Realtime contains values that no longer exist in
-- realtime_models (the original FK was created with foreign_key_checks=0).
UPDATE `user_gpt` g
LEFT JOIN `realtime_models` r ON g.`Realtime` = r.`Id`
SET g.`Realtime` = NULL
WHERE g.`Realtime` IS NOT NULL AND r.`Id` IS NULL;

ALTER TABLE `billing` ADD CONSTRAINT `FK_billing_currency` FOREIGN KEY (`Currency`) REFERENCES `currency` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `billing` ADD CONSTRAINT `FK_billing_users` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `channels` ADD CONSTRAINT `FK_channels_users` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `crm_configs` ADD CONSTRAINT `FK_crm_configs_users` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `crm_mappings` ADD CONSTRAINT `fk_crm_mappings_crm_config` FOREIGN KEY (`crm_config_id`) REFERENCES `crm_configs` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `crm_mappings` ADD CONSTRAINT `FK_crm_mappings_users` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `crm_oauth_states` ADD CONSTRAINT `FK_crm_oauth_states_users` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `crypto_payments` ADD CONSTRAINT `fk_crypto_payments_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `dialogs` ADD CONSTRAINT `fk_chat_type` FOREIGN KEY (`Type`) REFERENCES `chat_type` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `dialogs` ADD CONSTRAINT `fk_responder_id` FOREIGN KEY (`Responder`) REFERENCES `responders` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `dialogs` ADD CONSTRAINT `fk_user_id` FOREIGN KEY (`User`) REFERENCES `users` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `gpt_models` ADD CONSTRAINT `FK_provider` FOREIGN KEY (`Provider`) REFERENCES `model_providers` (`Id`) ON DELETE NO ACTION ON UPDATE NO ACTION;
ALTER TABLE `notifications` ADD CONSTRAINT `FK__users` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `operators` ADD CONSTRAINT `FK_userId` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `realtime_models` ADD CONSTRAINT `FK_provider` FOREIGN KEY (`Provider`) REFERENCES `model_providers` (`Id`) ON DELETE NO ACTION ON UPDATE NO ACTION;
ALTER TABLE `responders` ADD CONSTRAINT `fk_chat_type_for_responders` FOREIGN KEY (`Type`) REFERENCES `chat_type` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `service` ADD CONSTRAINT `fk_service_user` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_contacts` ADD CONSTRAINT `FK1_uid` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `service_contact_availability` ADD CONSTRAINT `fk_availability_contact` FOREIGN KEY (`ContactId`) REFERENCES `service_contacts` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_dialogs` ADD CONSTRAINT `FK2_resp` FOREIGN KEY (`Responder`) REFERENCES `service_contacts` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_dialogs` ADD CONSTRAINT `fk_service_dialogs_dialog` FOREIGN KEY (`DialogId`) REFERENCES `dialogs` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_gpt` ADD CONSTRAINT `FK_service_gpt_model` FOREIGN KEY (`ModelId`) REFERENCES `user_models` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `service_gpt` ADD CONSTRAINT `FK_service_gpt_userid` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_proxy` ADD CONSTRAINT `fk_service_proxy_user` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_tgbots` ADD CONSTRAINT `fk_service_tgbots_user` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `service_wabots` ADD CONSTRAINT `fk_service_wabots_user` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `storage_migrations` ADD CONSTRAINT `fk_storage_migration_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `subscriptions` ADD CONSTRAINT `subscriptions_ibfk_1` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `users` ADD CONSTRAINT `fk_currency_name` FOREIGN KEY (`currency`) REFERENCES `currency` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `users` ADD CONSTRAINT `fk_lang` FOREIGN KEY (`Lang`) REFERENCES `languages` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `users` ADD CONSTRAINT `fk_role` FOREIGN KEY (`RoleId`) REFERENCES `user_roles` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `user_api_keys` ADD CONSTRAINT `fk_uak_user` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `user_auth` ADD CONSTRAINT `user_auth_ibfk_1` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE `user_gpt` ADD CONSTRAINT `FK_user_gpt_gpt_models` FOREIGN KEY (`Model`) REFERENCES `gpt_models` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `user_gpt` ADD CONSTRAINT `fk_user_gpt_provider` FOREIGN KEY (`Provider`) REFERENCES `model_providers` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `user_gpt` ADD CONSTRAINT `fk_user_gpt_realtime_v2` FOREIGN KEY (`Realtime`) REFERENCES `realtime_models` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `user_models` ADD CONSTRAINT `fk_user_models_model` FOREIGN KEY (`ModelId`) REFERENCES `user_gpt` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `user_models` ADD CONSTRAINT `fk_user_models_provider` FOREIGN KEY (`Provider`) REFERENCES `model_providers` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE `user_models` ADD CONSTRAINT `fk_user_models_user` FOREIGN KEY (`UserId`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `user_storage_config` ADD CONSTRAINT `fk_storage_config_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `user_storage_quota` ADD CONSTRAINT `fk_storage_quota_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`Id`) ON DELETE CASCADE ON UPDATE RESTRICT;
ALTER TABLE `voice_models` ADD CONSTRAINT `FK_voice_provider` FOREIGN KEY (`Provider`) REFERENCES `model_providers` (`Id`) ON DELETE RESTRICT ON UPDATE RESTRICT;




