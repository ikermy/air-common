ALTER TABLE `vector_embeddings` DROP FOREIGN KEY `fk_embeddings_user_model`;
ALTER TABLE `vector_embeddings` DROP KEY `idx_user_user_model`;
ALTER TABLE `vector_embeddings` DROP COLUMN `user_model_id`;
