-- +goose Up
-- Integridade do ledger (`transacoes`) e índices do painel admin/jobs:
--  * D6: no máximo UM crédito de encerramento por (desafio, usuário, tipo) para `premio_desafio`,
--    `pagamento_instrutor` e `receita_academia`, via coluna gerada VIRTUAL + UNIQUE (NULL nos demais
--    tipos, e o MySQL aceita NULLs repetidos num UNIQUE). Coluna VIRTUAL (não STORED) de propósito: o
--    MySQL proíbe ON DELETE SET NULL numa FK cuja coluna é base de coluna gerada STORED, e
--    fk_transacoes_desafio (00003) é SET NULL.
--    NÃO há UNIQUE para `entrada_desafio` (entrar -> sair -> entrar é legítimo) nem para
--    `reserva_desafio`/`estorno_reserva` (repetem a cada edição da reserva).
--  * D6: CHECK de sinal de `valor_centavos` coerente com o tipo (os 13 tipos; `ajuste` livre). Os tipos da
--    moderação (00013) são débitos: `estorno_anulacao` (−, devolve o crédito do pódio anulado após a
--    liberação) e `abatimento_debito` (−, desconto de usuario_debitos num crédito do devedor).
--  * D7: índices (tipo, status, created_at) e (status, updated_at).
--  * D12 (N5): `id_transacao_origem` (FK para a própria tabela) liga a `tarifa_saque` ao saque, com
--    backfill pela `descricao` ("Tarifa do saque #<id>"). UNIQUE (id_transacao_origem, tipo): no máximo
--    uma tarifa por saque.
--  * Marca de revisão manual de saque (`em_revisao`/`revisao_motivo`), usada por B1/P6: é uma MARCA,
--    não um status — o saque continua `pending` (saldo já debitado), então a invariante
--    saldo == SUM(ledger efetivo) não muda.
--
-- A migration NÃO apaga nem corrige dinheiro: se houver linhas que violem as regras, ela aborta com
-- SIGNAL (a aplicação não sobe) e uma mensagem curta. Consultas para localizar as linhas (corrigir
-- manualmente, com estorno pelo ledger, e subir de novo):
--   SELECT id_desafio, id_usuario, tipo, COUNT(*) FROM transacoes
--    WHERE tipo IN ('premio_desafio','pagamento_instrutor','receita_academia') AND id_desafio IS NOT NULL
--    GROUP BY 1, 2, 3 HAVING COUNT(*) > 1;
--   SELECT * FROM transacoes WHERE NOT (
--       (tipo IN ('deposito','estorno_desafio','premio_desafio','estorno_reserva','pagamento_instrutor','receita_academia') AND valor_centavos > 0)
--    OR (tipo IN ('saque','entrada_desafio','reserva_desafio','tarifa_saque','estorno_anulacao','abatimento_debito') AND valor_centavos < 0)
--    OR tipo = 'ajuste');
--   -- tarifas duplicadas para o mesmo saque (o backfill abaixo ligaria as duas ao saque e o UNIQUE
--   -- uq_transacoes_origem_tipo falharia com 1062): estornar a tarifa extra e mudar a descrição dela
--   SELECT descricao, id_usuario, COUNT(*), GROUP_CONCAT(id) FROM transacoes
--    WHERE tipo = 'tarifa_saque' AND descricao REGEXP '^Tarifa do saque #[0-9]+$'
--    GROUP BY 1, 2 HAVING COUNT(*) > 1;
--
-- Mesmo padrão da 00011/00012: ADD condicional via procedure temporária (reaplicável).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00016_verificar`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00016_verificar`()
BEGIN
  IF EXISTS (SELECT 1 FROM `transacoes`
              WHERE `tipo` IN ('premio_desafio','pagamento_instrutor','receita_academia') AND `id_desafio` IS NOT NULL
              GROUP BY `id_desafio`, `id_usuario`, `tipo` HAVING COUNT(*) > 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00016: transacoes tem credito de encerramento duplicado por (desafio, usuario, tipo) - ver consultas no topo da migration';
  END IF;
  IF EXISTS (SELECT 1 FROM `transacoes` WHERE NOT (
               (`tipo` IN ('deposito','estorno_desafio','premio_desafio','estorno_reserva','pagamento_instrutor','receita_academia') AND `valor_centavos` > 0)
               OR (`tipo` IN ('saque','entrada_desafio','reserva_desafio','tarifa_saque','estorno_anulacao','abatimento_debito') AND `valor_centavos` < 0)
               OR `tipo` = 'ajuste')) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00016: transacoes tem valor_centavos com sinal incoerente com o tipo - ver consultas no topo da migration';
  END IF;
  -- Mesmo critério do backfill de id_transacao_origem (descrição exata + saque do mesmo usuário):
  -- duas tarifas que seriam ligadas ao mesmo saque violariam uq_transacoes_origem_tipo (1062).
  IF EXISTS (SELECT 1 FROM `transacoes` t
               JOIN `transacoes` s
                 ON s.`id` = CAST(SUBSTRING(t.`descricao`, CHAR_LENGTH('Tarifa do saque #') + 1) AS UNSIGNED)
                AND s.`tipo` = 'saque' AND s.`id_usuario` = t.`id_usuario`
              WHERE t.`tipo` = 'tarifa_saque' AND t.`descricao` REGEXP '^Tarifa do saque #[0-9]+$'
              GROUP BY s.`id` HAVING COUNT(*) > 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00016: transacoes tem tarifa_saque duplicada para o mesmo saque (descricao repetida) - ver consultas no topo da migration';
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00016_verificar`();
DROP PROCEDURE `_goose_00016_verificar`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00016_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00016_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
BEGIN
  DECLARE v_existe INT DEFAULT 0;
  IF p_tipo = 'coluna' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.columns
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND column_name = p_nome;
  ELSEIF p_tipo = 'indice' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.statistics
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND index_name = p_nome;
  ELSE
    SELECT COUNT(*) INTO v_existe FROM information_schema.table_constraints
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome;
  END IF;
  IF v_existe = 0 THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` ADD ', p_definicao);
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

-- ── D6: unicidade dos créditos de encerramento ──
CALL `_goose_00016_add`('transacoes', 'coluna', 'id_desafio_credito_unico', "COLUMN `id_desafio_credito_unico` BIGINT UNSIGNED GENERATED ALWAYS AS (IF(`tipo` IN ('premio_desafio','pagamento_instrutor','receita_academia'), `id_desafio`, NULL)) VIRTUAL COMMENT 'id_desafio só nos créditos de encerramento (premio/pagamento_instrutor/receita_academia); NULL nos demais. Base do UNIQUE uq_transacoes_credito_desafio'");
CALL `_goose_00016_add`('transacoes', 'indice', 'uq_transacoes_credito_desafio', "UNIQUE KEY `uq_transacoes_credito_desafio` (`id_desafio_credito_unico`, `id_usuario`, `tipo`)");

-- ── D6: sinal coerente com o tipo ──
CALL `_goose_00016_add`('transacoes', 'check', 'chk_transacoes_sinal', "CONSTRAINT `chk_transacoes_sinal` CHECK ((`tipo` IN ('deposito','estorno_desafio','premio_desafio','estorno_reserva','pagamento_instrutor','receita_academia') AND `valor_centavos` > 0) OR (`tipo` IN ('saque','entrada_desafio','reserva_desafio','tarifa_saque','estorno_anulacao','abatimento_debito') AND `valor_centavos` < 0) OR `tipo` = 'ajuste')");

-- ── D7: índices do painel admin e dos jobs ──
-- Listagem admin por tipo/status/período e varredura de saques pendentes (tipo='saque' AND status='pending' ORDER BY created_at).
CALL `_goose_00016_add`('transacoes', 'indice', 'idx_transacoes_tipo_status_criado', "KEY `idx_transacoes_tipo_status_criado` (`tipo`, `status`, `created_at`)");
-- Jobs de reconciliação que procuram linhas paradas num status (status = ? AND updated_at < ?).
CALL `_goose_00016_add`('transacoes', 'indice', 'idx_transacoes_status_atualizado', "KEY `idx_transacoes_status_atualizado` (`status`, `updated_at`)");

-- ── D12 (N5): tarifa ligada ao saque por coluna ──
CALL `_goose_00016_add`('transacoes', 'coluna', 'id_transacao_origem', "COLUMN `id_transacao_origem` BIGINT UNSIGNED NULL COMMENT 'Linha do ledger que originou esta (ex.: tarifa_saque -> saque)' AFTER `id_desafio`");
-- Backfill pela descrição gravada por CarteiraRepository.IniciarSaqueComTarifa ("Tarifa do saque #<id>"),
-- exigindo que o saque exista e seja do mesmo usuário. Reaplicável (só linhas ainda sem origem).
UPDATE `transacoes` t
  JOIN `transacoes` s
    ON s.`id` = CAST(SUBSTRING(t.`descricao`, CHAR_LENGTH('Tarifa do saque #') + 1) AS UNSIGNED)
   AND s.`tipo` = 'saque' AND s.`id_usuario` = t.`id_usuario`
   SET t.`id_transacao_origem` = s.`id`
 WHERE t.`tipo` = 'tarifa_saque' AND t.`id_transacao_origem` IS NULL
   AND t.`descricao` REGEXP '^Tarifa do saque #[0-9]+$';
CALL `_goose_00016_add`('transacoes', 'indice', 'uq_transacoes_origem_tipo', "UNIQUE KEY `uq_transacoes_origem_tipo` (`id_transacao_origem`, `tipo`)");
CALL `_goose_00016_add`('transacoes', 'fk', 'fk_transacoes_origem', "CONSTRAINT `fk_transacoes_origem` FOREIGN KEY (`id_transacao_origem`) REFERENCES `transacoes` (`id`)");

-- ── Revisão manual de saque (B1/P6) ──
CALL `_goose_00016_add`('transacoes', 'coluna', 'em_revisao', "COLUMN `em_revisao` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = saque aguardando revisão manual no painel admin (continua pending)'");
CALL `_goose_00016_add`('transacoes', 'coluna', 'revisao_motivo', "COLUMN `revisao_motivo` VARCHAR(255) NULL COMMENT 'Por que o saque foi para revisão (preenchido pelo sistema)' AFTER `em_revisao`");
CALL `_goose_00016_add`('transacoes', 'indice', 'idx_transacoes_revisao', "KEY `idx_transacoes_revisao` (`em_revisao`, `tipo`, `status`)");

DROP PROCEDURE `_goose_00016_add`;

-- +goose Down
-- Remove as restrições, índices e colunas. Perde-se a ligação tarifa -> saque (a descrição continua
-- com "Tarifa do saque #<id>") e a marca de revisão. Nenhum dado financeiro é alterado.
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00016_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00016_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
BEGIN
  DECLARE v_existe INT DEFAULT 0;
  IF p_tipo = 'coluna' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.columns
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND column_name = p_nome;
  ELSEIF p_tipo = 'indice' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.statistics
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND index_name = p_nome;
  ELSE
    SELECT COUNT(*) INTO v_existe FROM information_schema.table_constraints
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome;
  END IF;
  IF v_existe > 0 THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` ', CASE p_tipo
      WHEN 'coluna' THEN CONCAT('DROP COLUMN `', p_nome, '`')
      WHEN 'indice' THEN CONCAT('DROP INDEX `', p_nome, '`')
      WHEN 'fk' THEN CONCAT('DROP FOREIGN KEY `', p_nome, '`')
      ELSE CONCAT('DROP CHECK `', p_nome, '`') END);
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

CALL `_goose_00016_drop`('transacoes', 'indice', 'idx_transacoes_revisao');
CALL `_goose_00016_drop`('transacoes', 'coluna', 'revisao_motivo');
CALL `_goose_00016_drop`('transacoes', 'coluna', 'em_revisao');

CALL `_goose_00016_drop`('transacoes', 'fk', 'fk_transacoes_origem');
CALL `_goose_00016_drop`('transacoes', 'indice', 'uq_transacoes_origem_tipo');
CALL `_goose_00016_drop`('transacoes', 'coluna', 'id_transacao_origem');

CALL `_goose_00016_drop`('transacoes', 'indice', 'idx_transacoes_status_atualizado');
CALL `_goose_00016_drop`('transacoes', 'indice', 'idx_transacoes_tipo_status_criado');

CALL `_goose_00016_drop`('transacoes', 'check', 'chk_transacoes_sinal');
CALL `_goose_00016_drop`('transacoes', 'indice', 'uq_transacoes_credito_desafio');
CALL `_goose_00016_drop`('transacoes', 'coluna', 'id_desafio_credito_unico');

DROP PROCEDURE `_goose_00016_drop`;
