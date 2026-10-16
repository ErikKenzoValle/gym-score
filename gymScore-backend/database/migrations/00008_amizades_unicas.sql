-- +goose Up
-- Remove autoamizades e duplicatas (inclusive em direções opostas), mantendo a "melhor" linha do par:
-- aceita > pendente > recusada; em empate, a de menor id.
DELETE FROM `amizades` WHERE `id_usuario` = `id_amigo`;

DELETE a FROM `amizades` a
JOIN `amizades` b
  ON LEAST(a.`id_usuario`, a.`id_amigo`) = LEAST(b.`id_usuario`, b.`id_amigo`)
 AND GREATEST(a.`id_usuario`, a.`id_amigo`) = GREATEST(b.`id_usuario`, b.`id_amigo`)
 AND a.`id` <> b.`id`
WHERE (CASE b.`status` WHEN 'aceita' THEN 3 WHEN 'pendente' THEN 2 WHEN 'recusada' THEN 1 ELSE 0 END)
    > (CASE a.`status` WHEN 'aceita' THEN 3 WHEN 'pendente' THEN 2 WHEN 'recusada' THEN 1 ELSE 0 END)
   OR ((CASE b.`status` WHEN 'aceita' THEN 3 WHEN 'pendente' THEN 2 WHEN 'recusada' THEN 1 ELSE 0 END)
     = (CASE a.`status` WHEN 'aceita' THEN 3 WHEN 'pendente' THEN 2 WHEN 'recusada' THEN 1 ELSE 0 END)
       AND b.`id` < a.`id`);

UPDATE `amizades` SET `status` = 'pendente' WHERE `status` IS NULL;

ALTER TABLE `amizades`
  MODIFY COLUMN `status` ENUM('pendente','aceita','recusada') NOT NULL DEFAULT 'pendente',
  ADD COLUMN `par` VARCHAR(41) GENERATED ALWAYS AS (CONCAT(LEAST(`id_usuario`,`id_amigo`),'-',GREATEST(`id_usuario`,`id_amigo`))) STORED COMMENT 'Par não-direcional (menor-maior) para unicidade',
  ADD UNIQUE KEY `uq_amizades_par` (`par`),
  ADD CONSTRAINT `chk_amizades_nao_si` CHECK (`id_usuario` <> `id_amigo`);

-- +goose Down
ALTER TABLE `amizades`
  DROP CONSTRAINT `chk_amizades_nao_si`,
  DROP INDEX `uq_amizades_par`,
  DROP COLUMN `par`,
  MODIFY COLUMN `status` ENUM('pendente','aceita','recusada') DEFAULT 'pendente';
