-- Phase 1.5: replace the invalid vector_embeddings -> user_models(ModelId)
-- FK (ModelId is not unique) with a reference to user_models.Id.

ALTER TABLE `vector_embeddings`
  ADD COLUMN `user_model_id` BIGINT UNSIGNED NULL AFTER `model_id`;

UPDATE `vector_embeddings` ve
JOIN `user_models` um
  ON um.`ModelId` = ve.`model_id`
 AND um.`UserId` = ve.`user_id`
SET ve.`user_model_id` = um.`Id`;

ALTER TABLE `vector_embeddings`
  ADD KEY `idx_user_user_model` (`user_id`, `user_model_id`),
  ADD CONSTRAINT `fk_embeddings_user_model`
    FOREIGN KEY (`user_model_id`) REFERENCES `user_models` (`Id`)
    ON DELETE CASCADE ON UPDATE RESTRICT;
