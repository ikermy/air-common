-- Phase 6.4: per-service database users and least-privilege grants.
--
-- Replace __PASSWORD__ with a value from your secret store before running.
-- Do not commit real passwords. Apply once per environment:
--   mariadb < phase6_privileges.sql
--
-- Split the SQL if your environment cannot run CREATE USER from the app host.

-- ── Reference tables (read-only for every service) ──────────────────────────
-- model_providers, gpt_models, realtime_models, voice_models, currency,
-- languages, user_roles, chat_type

-- ── Users ───────────────────────────────────────────────────────────────────
CREATE USER IF NOT EXISTS 'air_migrator'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_orchestrator'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_whatsbot'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_tgbot'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_tguserbot'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_avito'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_widget'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_operator'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_payment'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_phantom'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_lead_hunter'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_analytics'@'%' IDENTIFIED BY '__PASSWORD__';
CREATE USER IF NOT EXISTS 'air_backup'@'%' IDENTIFIED BY '__PASSWORD__';

-- Enforce TLS per user (requires the server to have TLS configured).
ALTER USER 'air_migrator'@'%' REQUIRE SSL;
ALTER USER 'air_orchestrator'@'%' REQUIRE SSL;
ALTER USER 'air_whatsbot'@'%' REQUIRE SSL;
ALTER USER 'air_tgbot'@'%' REQUIRE SSL;
ALTER USER 'air_tguserbot'@'%' REQUIRE SSL;
ALTER USER 'air_avito'@'%' REQUIRE SSL;
ALTER USER 'air_widget'@'%' REQUIRE SSL;
ALTER USER 'air_operator'@'%' REQUIRE SSL;
ALTER USER 'air_payment'@'%' REQUIRE SSL;
ALTER USER 'air_phantom'@'%' REQUIRE SSL;
ALTER USER 'air_lead_hunter'@'%' REQUIRE SSL;
ALTER USER 'air_analytics'@'%' REQUIRE SSL;
ALTER USER 'air_backup'@'%' REQUIRE SSL;

-- ── Migrator: DDL only from CI/CD ───────────────────────────────────────────
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, INDEX, REFERENCES
  ON `air`.* TO 'air_migrator'@'%';

-- ── Orchestrator: user-scoped CRUD + reference reads ────────────────────────
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.users TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_auth TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_models TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_gpt TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_api_keys TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_channels TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_channel_configs TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_directory TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_storage_config TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_storage_quota TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bots TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_events TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_state TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialogs TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_messages TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_contexts TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.responders TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.notifications TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.subscriptions TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.billing TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.crypto_payments TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.crm_configs TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.crm_mappings TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.crm_oauth_states TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.google_oauth_tokens TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.operators TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.app_config TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.service_proxy TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.vector_embeddings TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.schema_migrations TO 'air_orchestrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.storage_migrations TO 'air_orchestrator'@'%';

-- ── Bots: user-scoped CRUD on their own rows + shared dialog/bot tables ────
-- The WhatsApp/Telegram/Avito/Widget services need dialogs, responders,
-- user_bots and user_channels; grants are intentionally identical for now and
-- should be tightened per service once row-level access is enforced in code.
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialogs TO 'air_whatsbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_messages TO 'air_whatsbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_contexts TO 'air_whatsbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.responders TO 'air_whatsbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bots TO 'air_whatsbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_events TO 'air_whatsbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_state TO 'air_whatsbot'@'%';
GRANT SELECT ON `air`.user_channels TO 'air_whatsbot'@'%';
GRANT SELECT ON `air`.user_channel_configs TO 'air_whatsbot'@'%';
GRANT SELECT ON `air`.users TO 'air_whatsbot'@'%';

GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialogs TO 'air_tgbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_messages TO 'air_tgbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_contexts TO 'air_tgbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.responders TO 'air_tgbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bots TO 'air_tgbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_events TO 'air_tgbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_state TO 'air_tgbot'@'%';
GRANT SELECT ON `air`.user_channels TO 'air_tgbot'@'%';
GRANT SELECT ON `air`.user_channel_configs TO 'air_tgbot'@'%';
GRANT SELECT ON `air`.users TO 'air_tgbot'@'%';

GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialogs TO 'air_tguserbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_messages TO 'air_tguserbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_contexts TO 'air_tguserbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.responders TO 'air_tguserbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bots TO 'air_tguserbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_events TO 'air_tguserbot'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_state TO 'air_tguserbot'@'%';
GRANT SELECT ON `air`.user_channels TO 'air_tguserbot'@'%';
GRANT SELECT ON `air`.user_channel_configs TO 'air_tguserbot'@'%';
GRANT SELECT ON `air`.users TO 'air_tguserbot'@'%';

GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialogs TO 'air_avito'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_messages TO 'air_avito'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_contexts TO 'air_avito'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.responders TO 'air_avito'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bots TO 'air_avito'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_events TO 'air_avito'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_state TO 'air_avito'@'%';
GRANT SELECT ON `air`.user_channels TO 'air_avito'@'%';
GRANT SELECT ON `air`.user_channel_configs TO 'air_avito'@'%';
GRANT SELECT ON `air`.users TO 'air_avito'@'%';

-- ── Read-only services ──────────────────────────────────────────────────────
GRANT SELECT ON `air`.users TO 'air_widget'@'%';
GRANT SELECT ON `air`.responders TO 'air_widget'@'%';
GRANT SELECT ON `air`.user_channels TO 'air_widget'@'%';
GRANT SELECT ON `air`.user_channel_configs TO 'air_widget'@'%';
GRANT SELECT ON `air`.dialogs TO 'air_widget'@'%';
GRANT SELECT ON `air`.dialog_messages TO 'air_widget'@'%';
GRANT SELECT ON `air`.user_bots TO 'air_widget'@'%';

GRANT SELECT ON `air`.users TO 'air_operator'@'%';
GRANT SELECT ON `air`.dialogs TO 'air_operator'@'%';
GRANT SELECT ON `air`.dialog_messages TO 'air_operator'@'%';
GRANT SELECT ON `air`.dialog_contexts TO 'air_operator'@'%';
GRANT SELECT ON `air`.responders TO 'air_operator'@'%';
GRANT SELECT ON `air`.operators TO 'air_operator'@'%';

GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.billing TO 'air_payment'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.crypto_payments TO 'air_payment'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.subscriptions TO 'air_payment'@'%';
GRANT SELECT ON `air`.users TO 'air_payment'@'%';

GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_channels TO 'air_phantom'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_channel_configs TO 'air_phantom'@'%';
GRANT SELECT ON `air`.users TO 'air_phantom'@'%';

-- Lead-hunter owns its contact/dialog staging tables.
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.service TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.service_contacts TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.service_contact_availability TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.service_dialogs TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.service_gpt TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialogs TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.dialog_messages TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.responders TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bots TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_events TO 'air_lead_hunter'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON `air`.user_bot_state TO 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.users TO 'air_lead_hunter'@'%';

-- ── Analytics / backup ──────────────────────────────────────────────────────
GRANT SELECT ON `air`.* TO 'air_analytics'@'%';
GRANT SELECT, RELOAD, LOCK TABLES, REPLICATION CLIENT, PROCESS, SHOW VIEW
  ON *.* TO 'air_backup'@'%';

-- ── Reference reads shared by all services ──────────────────────────────────
GRANT SELECT ON `air`.model_providers TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.gpt_models TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.realtime_models TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.voice_models TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.currency TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.languages TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.user_roles TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';
GRANT SELECT ON `air`.chat_type TO 'air_orchestrator'@'%', 'air_whatsbot'@'%', 'air_tgbot'@'%', 'air_tguserbot'@'%', 'air_avito'@'%', 'air_widget'@'%', 'air_operator'@'%', 'air_payment'@'%', 'air_phantom'@'%', 'air_lead_hunter'@'%';

FLUSH PRIVILEGES;

-- ── Server-side TLS (my.cnf, requires restart) ──────────────────────────────
-- [mysqld]
-- require_secure_transport = ON
-- tls_version = TLSv1.2,TLSv1.3
-- ssl_ca = /etc/mysql/certs/ca.pem
-- ssl_cert = /etc/mysql/certs/server-cert.pem
-- ssl_key = /etc/mysql/certs/server-key.pem
