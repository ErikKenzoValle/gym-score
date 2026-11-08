-- +goose Up
-- Sprint 6 (financeiro): premiação (prêmio fixo da academia + pote das inscrições), taxa da plataforma,
-- pagamento do instrutor (fixo/percentual) com "leilão" de vaga (candidaturas com lance até um teto) e
-- reserva (garantia) debitada da academia. Receita da plataforma fica em tabela própria (não é carteira).
--
-- Escrita para funcionar num banco novo (00001-00011) e no RDS: todo ADD/DROP é condicional via
-- procedures temporárias (removidas no fim), tabelas novas usam CREATE TABLE IF NOT EXISTS e o MODIFY do
-- ENUM do ledger é idempotente. Defaults mantêm o comportamento dos desafios existentes (pote = 100% das
-- inscrições, sem taxa, sem instrutor pago). `desafios.id_instrutor` já é NULL desde a 00011 (vaga aberta
-- não exige mudança de coluna/FK). Ver `database/README.md`.

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00012_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00012_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- ── desafios: configuração financeira (gravada na criação; mudanças no .env não afetam desafios já criados) ──
CALL `_goose_00012_add`('desafios', 'coluna', 'premio_fixo_centavos', "COLUMN `premio_fixo_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Prêmio fixo posto pela academia (centavos); reservado do saldo dela na criação' AFTER `posicoes_podio`");
CALL `_goose_00012_add`('desafios', 'coluna', 'dividir_pote', "COLUMN `dividir_pote` TINYINT(1) NOT NULL DEFAULT 1 COMMENT 'Se 1, parte das inscrições (percentual_pote) entra no prêmio dos atletas' AFTER `premio_fixo_centavos`");
CALL `_goose_00012_add`('desafios', 'coluna', 'percentual_pote', "COLUMN `percentual_pote` TINYINT UNSIGNED NOT NULL DEFAULT 100 COMMENT '% das inscrições (T) que vira pote dos atletas; só vale com dividir_pote = 1' AFTER `dividir_pote`");
CALL `_goose_00012_add`('desafios', 'coluna', 'taxa_plataforma_pct', "COLUMN `taxa_plataforma_pct` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '% das inscrições retido pela plataforma (snapshot do .env na criação); sai da parte da academia' AFTER `percentual_pote`");
CALL `_goose_00012_add`('desafios', 'coluna', 'instrutor_modo_pagamento', "COLUMN `instrutor_modo_pagamento` ENUM('fixo','percentual','fixo_percentual') NOT NULL DEFAULT 'fixo' COMMENT 'Como o instrutor é pago' AFTER `taxa_plataforma_pct`");
CALL `_goose_00012_add`('desafios', 'coluna', 'instrutor_fixo_centavos', "COLUMN `instrutor_fixo_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Parte fixa do instrutor (centavos); em vaga aberta passa a ser o lance escolhido' AFTER `instrutor_modo_pagamento`");
CALL `_goose_00012_add`('desafios', 'coluna', 'instrutor_pct', "COLUMN `instrutor_pct` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '% das inscrições (T) pago ao instrutor; sai da parte da academia' AFTER `instrutor_fixo_centavos`");
CALL `_goose_00012_add`('desafios', 'coluna', 'instrutor_selecao', "COLUMN `instrutor_selecao` ENUM('designado','vaga_aberta') NOT NULL DEFAULT 'designado' COMMENT 'designado = academia indica o instrutor; vaga_aberta = instrutores se candidatam (id_instrutor NULL até a escolha)' AFTER `instrutor_pct`");
CALL `_goose_00012_add`('desafios', 'coluna', 'instrutor_teto_centavos', "COLUMN `instrutor_teto_centavos` BIGINT NULL COMMENT 'Teto do lance na vaga aberta (centavos); obrigatório quando instrutor_selecao = vaga_aberta' AFTER `instrutor_selecao`");
CALL `_goose_00012_add`('desafios', 'coluna', 'reserva_academia_centavos', "COLUMN `reserva_academia_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Garantia hoje debitada da academia (prêmio fixo + instrutor fixo, ou teto na vaga aberta); devolvida no cancelamento' AFTER `instrutor_teto_centavos`");

CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_premio_fixo', "CONSTRAINT `chk_desafios_premio_fixo` CHECK (`premio_fixo_centavos` >= 0)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_percentual_pote', "CONSTRAINT `chk_desafios_percentual_pote` CHECK (`percentual_pote` BETWEEN 0 AND 100)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_taxa_pct', "CONSTRAINT `chk_desafios_taxa_pct` CHECK (`taxa_plataforma_pct` BETWEEN 0 AND 100)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_instrutor_pct', "CONSTRAINT `chk_desafios_instrutor_pct` CHECK (`instrutor_pct` BETWEEN 0 AND 100)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_soma_pct', "CONSTRAINT `chk_desafios_soma_pct` CHECK (`percentual_pote` + `taxa_plataforma_pct` + `instrutor_pct` <= 100)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_instrutor_fixo', "CONSTRAINT `chk_desafios_instrutor_fixo` CHECK (`instrutor_fixo_centavos` >= 0)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_instrutor_teto', "CONSTRAINT `chk_desafios_instrutor_teto` CHECK (`instrutor_teto_centavos` IS NULL OR `instrutor_teto_centavos` >= 0)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_vaga_aberta_teto', "CONSTRAINT `chk_desafios_vaga_aberta_teto` CHECK (`instrutor_selecao` <> 'vaga_aberta' OR `instrutor_teto_centavos` IS NOT NULL)");
CALL `_goose_00012_add`('desafios', 'check', 'chk_desafios_reserva', "CONSTRAINT `chk_desafios_reserva` CHECK (`reserva_academia_centavos` >= 0)");
-- Vagas abertas por data (aba "Vagas" da agenda do instrutor).
CALL `_goose_00012_add`('desafios', 'indice', 'idx_desafios_vagas', "KEY `idx_desafios_vagas` (`instrutor_selecao`, `status`, `data_evento`)");

-- ── desafio_candidaturas_instrutor: candidaturas (lance) à vaga de instrutor ──
CREATE TABLE IF NOT EXISTS `desafio_candidaturas_instrutor` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_desafio` BIGINT UNSIGNED NOT NULL COMMENT 'Desafio com vaga de instrutor aberta',
  `id_instrutor` BIGINT UNSIGNED NOT NULL COMMENT 'Instrutor candidato (vinculado ou autônomo)',
  `lance_centavos` BIGINT NOT NULL COMMENT 'Valor fixo que o instrutor cobra; deve ser <= desafios.instrutor_teto_centavos (validado na API)',
  `mensagem` VARCHAR(500) NULL COMMENT 'Mensagem do instrutor para a academia',
  `status` ENUM('pendente','escolhida','recusada','retirada') NOT NULL DEFAULT 'pendente' COMMENT 'Situação da candidatura',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando se candidatou',
  `atualizado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'Última alteração (lance, retirada, escolha)',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_candidatura_desafio_instrutor` (`id_desafio`, `id_instrutor`),
  KEY `idx_candidatura_instrutor_status` (`id_instrutor`, `status`),
  CONSTRAINT `fk_candidatura_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_candidatura_instrutor` FOREIGN KEY (`id_instrutor`) REFERENCES `instrutores` (`id`),
  CONSTRAINT `chk_candidatura_lance` CHECK (`lance_centavos` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Candidaturas de instrutores a vagas abertas de desafio (leilão com teto)';

-- ── plataforma_receitas: receita da plataforma (não é carteira de usuário) ──
-- id_desafio+origem únicos evitam lançar a taxa de inscrição duas vezes no mesmo desafio; id_transacao
-- único evita contar a mesma tarifa de saque duas vezes (NULLs repetidos são permitidos pelo MySQL).
CREATE TABLE IF NOT EXISTS `plataforma_receitas` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `origem` ENUM('taxa_inscricao','tarifa_saque') NOT NULL COMMENT 'De onde veio a receita',
  `valor_centavos` BIGINT NOT NULL COMMENT 'Valor recebido pela plataforma, em centavos (sempre > 0)',
  `id_desafio` BIGINT UNSIGNED NULL COMMENT 'Desafio que gerou a taxa de inscrição (NULL em tarifa de saque)',
  `id_transacao` BIGINT UNSIGNED NULL COMMENT 'Linha do ledger que gerou a receita (tarifa_saque)',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando a receita foi reconhecida',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_receita_desafio_origem` (`id_desafio`, `origem`),
  UNIQUE KEY `uq_receita_transacao` (`id_transacao`),
  KEY `idx_receita_data` (`criado_em`),
  KEY `idx_receita_origem_data` (`origem`, `criado_em`),
  CONSTRAINT `fk_receita_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_receita_transacao` FOREIGN KEY (`id_transacao`) REFERENCES `transacoes` (`id`),
  CONSTRAINT `chk_receita_valor` CHECK (`valor_centavos` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Receita da plataforma (taxa sobre inscrições e tarifa de saque)';

-- ── Ledger: novos tipos (valores existentes mantidos, na mesma ordem) ──
-- reserva_desafio (-) / estorno_reserva (+): garantia da academia; pagamento_instrutor (+);
-- receita_academia (+): parte líquida da academia no encerramento; tarifa_saque (-): debitada junto do saque.
ALTER TABLE `transacoes`
  MODIFY COLUMN `tipo` ENUM('deposito','saque','entrada_desafio','estorno_desafio','premio_desafio','ajuste','reserva_desafio','estorno_reserva','pagamento_instrutor','receita_academia','tarifa_saque') NOT NULL DEFAULT 'deposito' COMMENT 'Tipo da movimentação';

DROP PROCEDURE `_goose_00012_add`;

-- +goose Down
-- Reversão com perda de informação:
--  * configuração financeira dos desafios, candidaturas e receitas da plataforma são apagadas;
--  * linhas do ledger com os tipos novos viram `ajuste` (mesmo valor/sinal), preservando a invariante
--    saldo == soma do ledger. O dinheiro já movimentado (reservas, pagamentos, tarifas) NÃO é desfeito.
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00012_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00012_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

DROP TABLE IF EXISTS `plataforma_receitas`;
DROP TABLE IF EXISTS `desafio_candidaturas_instrutor`;

UPDATE `transacoes` SET `tipo` = 'ajuste'
 WHERE `tipo` IN ('reserva_desafio','estorno_reserva','pagamento_instrutor','receita_academia','tarifa_saque');
ALTER TABLE `transacoes`
  MODIFY COLUMN `tipo` ENUM('deposito','saque','entrada_desafio','estorno_desafio','premio_desafio','ajuste') NOT NULL DEFAULT 'deposito' COMMENT 'Tipo da movimentação';

CALL `_goose_00012_drop`('desafios', 'indice', 'idx_desafios_vagas');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_reserva');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_vaga_aberta_teto');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_instrutor_teto');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_instrutor_fixo');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_soma_pct');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_instrutor_pct');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_taxa_pct');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_percentual_pote');
CALL `_goose_00012_drop`('desafios', 'check', 'chk_desafios_premio_fixo');
CALL `_goose_00012_drop`('desafios', 'coluna', 'reserva_academia_centavos');
CALL `_goose_00012_drop`('desafios', 'coluna', 'instrutor_teto_centavos');
CALL `_goose_00012_drop`('desafios', 'coluna', 'instrutor_selecao');
CALL `_goose_00012_drop`('desafios', 'coluna', 'instrutor_pct');
CALL `_goose_00012_drop`('desafios', 'coluna', 'instrutor_fixo_centavos');
CALL `_goose_00012_drop`('desafios', 'coluna', 'instrutor_modo_pagamento');
CALL `_goose_00012_drop`('desafios', 'coluna', 'taxa_plataforma_pct');
CALL `_goose_00012_drop`('desafios', 'coluna', 'percentual_pote');
CALL `_goose_00012_drop`('desafios', 'coluna', 'dividir_pote');
CALL `_goose_00012_drop`('desafios', 'coluna', 'premio_fixo_centavos');

DROP PROCEDURE `_goose_00012_drop`;
