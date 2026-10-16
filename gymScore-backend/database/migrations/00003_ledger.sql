-- +goose Up
-- transacoes vira o ledger de toda movimentação de saldo.
-- valor_centavos é assinado: positivo = crédito, negativo = débito.
ALTER TABLE `transacoes`
  ADD COLUMN `tipo` ENUM('deposito','saque','entrada_desafio','estorno_desafio','premio_desafio','ajuste') NOT NULL DEFAULT 'deposito' COMMENT 'Tipo da movimentação' AFTER `id_usuario`,
  ADD COLUMN `valor_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Valor assinado em centavos (+crédito / -débito)' AFTER `tipo`,
  ADD COLUMN `id_desafio` BIGINT UNSIGNED NULL COMMENT 'Desafio relacionado (entrada/estorno/prêmio)' AFTER `valor_centavos`,
  ADD COLUMN `asaas_transfer_id` VARCHAR(100) NULL COMMENT 'ID da transferência PIX no Asaas (saque)' AFTER `asaas_payment_id`,
  ADD COLUMN `descricao` VARCHAR(255) NULL COMMENT 'Descrição legível da movimentação';

UPDATE `transacoes` SET `tipo` = 'deposito', `valor_centavos` = CAST(ROUND(`valor` * 100) AS SIGNED);

ALTER TABLE `transacoes`
  MODIFY COLUMN `asaas_payment_id` VARCHAR(100) NULL COMMENT 'ID da cobrança no Asaas (depósito); NULL nas demais';

-- Linha de abertura (tipo 'ajuste') para dados legados, garantindo a invariante
-- saldo_centavos == SUM(valor_centavos das linhas received/concluido + saques pending).
-- valor do ajuste = saldo migrado - soma dos depósitos legados já 'received' (pode ser negativo se o saldo já foi gasto).
-- Saldo legado negativo foi zerado na 00002: gera ajuste (valor conforme a regra acima) com o valor original na descrição.
INSERT INTO `transacoes` (`id_usuario`, `tipo`, `valor`, `valor_centavos`, `asaas_payment_id`, `status`, `descricao`, `created_at`, `updated_at`)
SELECT x.`id`, 'ajuste', 0, x.`saldo_centavos` - x.`ledger`, NULL, 'received',
       IF(x.`saldo_legado` < 0,
          CONCAT('Saldo migrado (saldo legado negativo de ', FORMAT(x.`saldo_legado`, 2), ' zerado)'),
          'Saldo migrado'),
       NOW(3), NOW(3)
FROM (
  SELECT u.`id`, u.`saldo_centavos`, COALESCE(u.`saldo`, 0) AS saldo_legado,
         COALESCE((SELECT SUM(t.`valor_centavos`) FROM `transacoes` t
                    WHERE t.`id_usuario` = u.`id` AND t.`tipo` = 'deposito' AND t.`status` = 'received'), 0) AS ledger
  FROM `usuarios` u
) x
WHERE x.`saldo_centavos` > 0 OR x.`saldo_legado` < 0 OR x.`saldo_centavos` <> x.`ledger`;

ALTER TABLE `transacoes`
  MODIFY COLUMN `status` ENUM('pending','received','refunded','failed','concluido') NOT NULL DEFAULT 'pending' COMMENT 'Status da movimentação',
  DROP COLUMN `valor`,
  ADD UNIQUE KEY `uq_transacoes_asaas_transfer_id` (`asaas_transfer_id`),
  ADD KEY `idx_transacoes_usuario_data` (`id_usuario`, `created_at`),
  ADD KEY `idx_transacoes_desafio` (`id_desafio`),
  ADD CONSTRAINT `fk_transacoes_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`),
  ADD CONSTRAINT `fk_transacoes_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE SET NULL;

ALTER TABLE `usuarios`
  DROP COLUMN `saldo`;

-- +goose Down
ALTER TABLE `usuarios`
  ADD COLUMN `saldo` decimal(10,2) DEFAULT '0.00' AFTER `genero`;
UPDATE `usuarios` SET `saldo` = `saldo_centavos` / 100;

ALTER TABLE `transacoes`
  DROP FOREIGN KEY `fk_transacoes_desafio`,
  DROP FOREIGN KEY `fk_transacoes_usuario`;

ALTER TABLE `transacoes`
  ADD COLUMN `valor` decimal(10,2) NOT NULL DEFAULT 0.00 AFTER `asaas_payment_id`;

-- Mantém apenas depósitos (únicos que existiam no modelo antigo)
DELETE FROM `transacoes` WHERE `tipo` <> 'deposito' OR `asaas_payment_id` IS NULL;
UPDATE `transacoes` SET `valor` = `valor_centavos` / 100, `status` = IF(`status` IN ('failed','concluido'), 'pending', `status`);

ALTER TABLE `transacoes`
  DROP INDEX `uq_transacoes_asaas_transfer_id`,
  DROP INDEX `idx_transacoes_usuario_data`,
  DROP INDEX `idx_transacoes_desafio`,
  DROP COLUMN `tipo`,
  DROP COLUMN `valor_centavos`,
  DROP COLUMN `id_desafio`,
  DROP COLUMN `asaas_transfer_id`,
  DROP COLUMN `descricao`,
  MODIFY COLUMN `asaas_payment_id` VARCHAR(100) NOT NULL,
  MODIFY COLUMN `status` ENUM('pending','received','refunded') DEFAULT 'pending';
