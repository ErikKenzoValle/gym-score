-- +goose Up
-- Modelo de desafio v2: cada participante paga a entrada; pote dividido na apuração.
-- Desafios antigos (sem reserva de saldo) não encerrados são marcados como cancelados, sem movimentar saldo.

ALTER TABLE `desafios`
  ADD COLUMN `valor_entrada_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Valor da entrada por participante, em centavos' AFTER `descricao`,
  ADD COLUMN `votacao_aberta_em` DATETIME(3) NULL COMMENT 'Quando a votação foi aberta' AFTER `data_encerramento`;

UPDATE `desafios` SET `valor_entrada_centavos` = GREATEST(CAST(ROUND(`valor` * 100) AS SIGNED), 0);

-- Participantes: prêmio e flag de vencedor (+ cascade ao apagar o desafio)
ALTER TABLE `desafio_participantes`
  ADD COLUMN `premio_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Prêmio recebido na apuração, em centavos' AFTER `id_usuario`,
  ADD COLUMN `vencedor` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = venceu (ou empatou na vitória) o desafio' AFTER `premio_centavos`;

ALTER TABLE `desafio_participantes`
  DROP FOREIGN KEY `fk_desafios_participantes`;
ALTER TABLE `desafio_participantes`
  ADD CONSTRAINT `fk_desafios_participantes` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE CASCADE;

-- Vencedores antigos viram participantes vencedores (o prêmio no modelo antigo era o valor do desafio)
INSERT IGNORE INTO `desafio_participantes` (`id_desafio`, `id_usuario`, `criado_em`)
SELECT d.`id`, d.`id_vencedor`, d.`atualizado_em` FROM `desafios` d WHERE d.`id_vencedor` IS NOT NULL;

UPDATE `desafio_participantes` p
JOIN `desafios` d ON d.`id` = p.`id_desafio` AND d.`id_vencedor` = p.`id_usuario`
SET p.`vencedor` = 1, p.`premio_centavos` = d.`valor_entrada_centavos`;

-- Status: amplia o enum, converte, e fixa o enum final
ALTER TABLE `desafios`
  MODIFY COLUMN `status` ENUM('pendente','aberto','em_andamento','em_votacao','encerrado','cancelado') NOT NULL DEFAULT 'aberto';

UPDATE `desafios` SET `status` = 'cancelado' WHERE `status` <> 'encerrado';

ALTER TABLE `desafios`
  MODIFY COLUMN `status` ENUM('aberto','em_andamento','em_votacao','encerrado','cancelado') NOT NULL DEFAULT 'aberto' COMMENT 'Ciclo de vida do desafio',
  MODIFY COLUMN `vagas` INT NOT NULL DEFAULT 2 COMMENT 'Número máximo de participantes',
  DROP FOREIGN KEY `fk_desafios_desafiado`,
  DROP FOREIGN KEY `fk_desafios_vencedor`;

ALTER TABLE `desafios`
  DROP COLUMN `valor`,
  DROP COLUMN `estado`,
  DROP COLUMN `id_desafiado`,
  DROP COLUMN `id_perdedor`,
  DROP COLUMN `id_vencedor`,
  ADD KEY `idx_desafios_status` (`status`);

-- +goose Down
-- Reversão com perda de informação: id_perdedor/id_desafiado não existem mais; estado é recalculado.
ALTER TABLE `desafios`
  DROP KEY `idx_desafios_status`,
  ADD COLUMN `valor` decimal(10,2) NOT NULL DEFAULT 0.00 AFTER `descricao`,
  ADD COLUMN `estado` tinyint DEFAULT '1' AFTER `status`,
  ADD COLUMN `id_desafiado` bigint unsigned DEFAULT NULL AFTER `id_criador`,
  ADD COLUMN `id_vencedor` bigint unsigned DEFAULT NULL AFTER `id_desafiado`,
  ADD COLUMN `id_perdedor` bigint unsigned DEFAULT NULL AFTER `id_vencedor`;

UPDATE `desafios` SET `valor` = `valor_entrada_centavos` / 100;
UPDATE `desafios` SET `estado` = CASE `status` WHEN 'cancelado' THEN 2 WHEN 'encerrado' THEN 0 ELSE 1 END;
UPDATE `desafios` d
JOIN (SELECT `id_desafio`, MIN(`id_usuario`) AS vid FROM `desafio_participantes` WHERE `vencedor` = 1 GROUP BY `id_desafio`) v ON v.`id_desafio` = d.`id`
SET d.`id_vencedor` = v.vid;

ALTER TABLE `desafios`
  MODIFY COLUMN `status` ENUM('pendente','aberto','em_andamento','em_votacao','encerrado','cancelado') NOT NULL DEFAULT 'aberto';
UPDATE `desafios` SET `status` = 'encerrado' WHERE `status` IN ('cancelado');
UPDATE `desafios` SET `status` = 'em_andamento' WHERE `status` = 'em_votacao';
ALTER TABLE `desafios`
  MODIFY COLUMN `status` ENUM('pendente','aberto','em_andamento','encerrado') DEFAULT 'aberto',
  MODIFY COLUMN `vagas` bigint DEFAULT '2',
  DROP COLUMN `valor_entrada_centavos`,
  DROP COLUMN `votacao_aberta_em`,
  ADD KEY `fk_desafios_desafiado` (`id_desafiado`),
  ADD KEY `fk_desafios_vencedor` (`id_vencedor`),
  ADD CONSTRAINT `fk_desafios_desafiado` FOREIGN KEY (`id_desafiado`) REFERENCES `usuarios` (`id`),
  ADD CONSTRAINT `fk_desafios_vencedor` FOREIGN KEY (`id_vencedor`) REFERENCES `usuarios` (`id`);

ALTER TABLE `desafio_participantes`
  DROP FOREIGN KEY `fk_desafios_participantes`;
ALTER TABLE `desafio_participantes`
  DROP COLUMN `premio_centavos`,
  DROP COLUMN `vencedor`,
  ADD CONSTRAINT `fk_desafios_participantes` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`);
