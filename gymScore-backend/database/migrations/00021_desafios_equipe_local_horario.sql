-- +goose Up
-- Rodada 5 (D5-1) — desafios: localização estruturada, horário de fim, equipe de instrutores (fim do leilão).
--  (T7) `desafios` ganha o endereço estruturado igual ao de `academias` (00020): `cep`, `logradouro`, `numero`,
--       `complemento`, `bairro`, `cidade`, `uf` + os mesmos CHECKs. Tudo NULL (desafios existentes ficam só
--       com o texto em `local`). `local` continua sendo o texto montado (compat) e passa a VARCHAR(500).
--  (T6) `data_fim_evento` (fim do evento, intervalo [data_evento, data_fim_evento)). Backfill: data_evento + 2h.
--       CHECK: fim > início e fim só existe com início. "Obrigatória em desafios novos" é regra da API
--       (o banco aceita NULL para os legados sem data_evento).
--  (T1/T2) `instrutores_vagas` (1..20) = tamanho da equipe; valores POR INSTRUTOR, logo o CHECK de soma passa a
--       `percentual_pote + taxa_plataforma_pct + instrutor_pct * instrutores_vagas <= 100`.
--       Nova tabela `desafio_instrutores` (equipe), no máx. 1 responsável por desafio (coluna gerada + UNIQUE).
--  Migração: todo `desafios.id_instrutor` vira membro responsável da equipe (origem `candidatura` quando o
--       desafio era `vaga_aberta`, senão `indicado`). `desafios.id_instrutor` FICA como espelho LEGADO do
--       responsável (o backend grava os dois na mesma transação; sem trigger — ver 00014 sobre CREATE TRIGGER
--       no RDS). (T8) Desafios `vaga_aberta` ainda abertos/em andamento sem instrutor: 1 vaga com fixo = teto
--       (a reserva atual já é prêmio fixo + teto: ledger intacto); modo `percentual` vira `fixo_percentual`
--       nesses (a apuração sempre pagou o fixo); candidaturas pendentes continuam pendentes (passam a
--       significar "aceito as condições"; o lance fica só como histórico).
--  (T4) `desafio_candidaturas_instrutor.lance_centavos` vira NULL (legado, não usado) e o status ganha `removida`
--       (membro aceito via candidatura e depois removido pela academia). `instrutor_selecao` e
--       `instrutor_teto_centavos` ficam LEGADOS (não usados pelo código novo; mantidos para o Down).
-- Mesmo padrão da 00011–00020: procedures temporárias, tudo condicional por nome (reaplicável).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00021_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00021_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00021_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Remove CHECK somente se existir (troca do CHECK de soma).
CREATE PROCEDURE `_goose_00021_drop`(IN p_tabela VARCHAR(64), IN p_nome VARCHAR(64))
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.table_constraints
              WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome) THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP CHECK `', p_nome, '`');
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

-- ── T7: desafios — endereço estruturado (mesmos tipos/CHECKs de academias, 00020) ──
ALTER TABLE `desafios`
  MODIFY COLUMN `local` VARCHAR(500) NULL COMMENT 'Endereço do evento em texto formatado (montado pelo backend a partir dos campos estruturados; legados: texto livre)';
CALL `_goose_00021_add`('desafios', 'coluna', 'cep', "COLUMN `cep` CHAR(8) CHARACTER SET ascii COLLATE ascii_bin NULL COMMENT 'CEP do local do evento, só dígitos (8)' AFTER `local`");
CALL `_goose_00021_add`('desafios', 'coluna', 'logradouro', "COLUMN `logradouro` VARCHAR(150) NULL COMMENT 'Rua/avenida do evento' AFTER `cep`");
CALL `_goose_00021_add`('desafios', 'coluna', 'numero', "COLUMN `numero` VARCHAR(20) NULL COMMENT 'Número (texto: aceita S/N, 12A)' AFTER `logradouro`");
CALL `_goose_00021_add`('desafios', 'coluna', 'complemento', "COLUMN `complemento` VARCHAR(100) NULL COMMENT 'Complemento (opcional)' AFTER `numero`");
CALL `_goose_00021_add`('desafios', 'coluna', 'bairro', "COLUMN `bairro` VARCHAR(100) NULL COMMENT 'Bairro' AFTER `complemento`");
CALL `_goose_00021_add`('desafios', 'coluna', 'cidade', "COLUMN `cidade` VARCHAR(100) NULL COMMENT 'Município' AFTER `bairro`");
CALL `_goose_00021_add`('desafios', 'coluna', 'uf', "COLUMN `uf` CHAR(2) CHARACTER SET ascii COLLATE ascii_bin NULL COMMENT 'Sigla da UF (maiúsculas; collation binária: sp é recusado)' AFTER `cidade`");
CALL `_goose_00021_add`('desafios', 'check', 'chk_desafios_cep', "CONSTRAINT `chk_desafios_cep` CHECK (`cep` IS NULL OR REGEXP_LIKE(`cep`, '^[0-9]{8}$', 'c'))");
CALL `_goose_00021_add`('desafios', 'check', 'chk_desafios_uf', "CONSTRAINT `chk_desafios_uf` CHECK (`uf` IS NULL OR `uf` IN ('AC','AL','AP','AM','BA','CE','DF','ES','GO','MA','MT','MS','MG','PA','PB','PR','PE','PI','RJ','RN','RS','RO','RR','SC','SP','SE','TO'))");
CALL `_goose_00021_add`('desafios', 'indice', 'idx_desafios_uf_cidade', "KEY `idx_desafios_uf_cidade` (`uf`, `cidade`)");

-- ── T6: horário de fim ──
CALL `_goose_00021_add`('desafios', 'coluna', 'data_fim_evento', "COLUMN `data_fim_evento` DATETIME(3) NULL COMMENT 'Fim do evento (intervalo [data_evento, data_fim_evento)); obrigatório em desafios novos (API); legados: data_evento + 2h' AFTER `data_evento`");
UPDATE `desafios` SET `data_fim_evento` = `data_evento` + INTERVAL 2 HOUR
 WHERE `data_fim_evento` IS NULL AND `data_evento` IS NOT NULL;
CALL `_goose_00021_add`('desafios', 'check', 'chk_desafios_fim_evento', "CONSTRAINT `chk_desafios_fim_evento` CHECK (`data_fim_evento` IS NULL OR (`data_evento` IS NOT NULL AND `data_fim_evento` > `data_evento`))");
-- Choque de horário por academia: desafios ativos da academia que cruzam um intervalo.
CALL `_goose_00021_add`('desafios', 'indice', 'idx_desafios_academia_horario', "KEY `idx_desafios_academia_horario` (`id_academia`, `status`, `data_evento`, `data_fim_evento`)");

-- ── T1/T2: tamanho da equipe e CHECK de soma por instrutor ──
CALL `_goose_00021_add`('desafios', 'coluna', 'instrutores_vagas', "COLUMN `instrutores_vagas` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT 'Tamanho da equipe de instrutores (1 a 20); fixo e % são POR instrutor' AFTER `instrutor_pct`");
CALL `_goose_00021_add`('desafios', 'check', 'chk_desafios_instrutores_vagas', "CONSTRAINT `chk_desafios_instrutores_vagas` CHECK (`instrutores_vagas` BETWEEN 1 AND 20)");
CALL `_goose_00021_drop`('desafios', 'chk_desafios_soma_pct');
CALL `_goose_00021_add`('desafios', 'check', 'chk_desafios_soma_pct_equipe', "CONSTRAINT `chk_desafios_soma_pct_equipe` CHECK (`percentual_pote` + `taxa_plataforma_pct` + `instrutor_pct` * `instrutores_vagas` <= 100)");

ALTER TABLE `desafios`
  MODIFY COLUMN `id_instrutor` BIGINT UNSIGNED NULL COMMENT 'LEGADO (rodada 5): espelho do responsável da equipe (desafio_instrutores.responsavel); o backend grava junto, o código novo lê a equipe',
  MODIFY COLUMN `instrutor_selecao` ENUM('designado','vaga_aberta') NOT NULL DEFAULT 'designado' COMMENT 'LEGADO (rodada 5): substituído por instrutores_vagas + desafio_instrutores; não usado pelo código novo',
  MODIFY COLUMN `instrutor_teto_centavos` BIGINT NULL COMMENT 'LEGADO (rodada 5): teto do antigo leilão; não usado pelo código novo',
  MODIFY COLUMN `instrutor_fixo_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Parte fixa paga a CADA instrutor da equipe (centavos)',
  MODIFY COLUMN `instrutor_pct` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '% das inscrições (T) pago a CADA instrutor da equipe; sai da parte da academia',
  MODIFY COLUMN `reserva_academia_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Garantia hoje debitada da academia (prêmio fixo + instrutores_vagas * instrutor_fixo); vagas vazias devolvidas no fechamento';

-- ── T2: equipe de instrutores ──
CREATE TABLE IF NOT EXISTS `desafio_instrutores` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_desafio` BIGINT UNSIGNED NOT NULL COMMENT 'Desafio',
  `id_instrutor` BIGINT UNSIGNED NOT NULL COMMENT 'Instrutor membro da equipe',
  `origem` ENUM('indicado','candidatura') NOT NULL COMMENT 'indicado = posto pela academia (vinculado aprovado); candidatura = candidatura aceita',
  `responsavel` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = responsável pelo pódio (único que lança/relança); no máx. 1 por desafio',
  `responsavel_desafio` BIGINT UNSIGNED AS (IF(`responsavel` = 1, `id_desafio`, NULL)) STORED COMMENT 'Gerada: id_desafio só no responsável (UNIQUE garante 1 por desafio)',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando entrou na equipe',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_desafio_instrutores` (`id_desafio`, `id_instrutor`),
  UNIQUE KEY `uq_desafio_instrutores_responsavel` (`responsavel_desafio`),
  KEY `idx_desafio_instrutores_instrutor` (`id_instrutor`, `id_desafio`),
  CONSTRAINT `fk_desafio_instrutores_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_desafio_instrutores_instrutor` FOREIGN KEY (`id_instrutor`) REFERENCES `instrutores` (`id`),
  CONSTRAINT `chk_desafio_instrutores_responsavel` CHECK (`responsavel` IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Equipe de instrutores do desafio (rodada 5); tamanho máximo = desafios.instrutores_vagas (validado na API)';

-- Migração do instrutor único para a equipe (responsável). Reaplicável: só desafios ainda sem equipe.
INSERT INTO `desafio_instrutores` (`id_desafio`, `id_instrutor`, `origem`, `responsavel`, `criado_em`)
SELECT d.`id`, d.`id_instrutor`,
       IF(d.`instrutor_selecao` = 'vaga_aberta', 'candidatura', 'indicado'), 1,
       COALESCE((SELECT MAX(c.`atualizado_em`) FROM `desafio_candidaturas_instrutor` c
                  WHERE c.`id_desafio` = d.`id` AND c.`id_instrutor` = d.`id_instrutor` AND c.`status` = 'escolhida'),
                d.`criado_em`)
  FROM `desafios` d
 WHERE d.`id_instrutor` IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM `desafio_instrutores` e WHERE e.`id_desafio` = d.`id`);

-- T8: vaga aberta ainda sem instrutor em desafio ativo → 1 vaga com fixo = teto (reserva já = prêmio + teto).
UPDATE `desafios`
   SET `instrutor_fixo_centavos` = `instrutor_teto_centavos`,
       `instrutor_modo_pagamento` = IF(`instrutor_modo_pagamento` = 'percentual' AND `instrutor_teto_centavos` > 0, 'fixo_percentual', `instrutor_modo_pagamento`),
       `instrutores_vagas` = 1
 WHERE `instrutor_selecao` = 'vaga_aberta' AND `id_instrutor` IS NULL
   AND `status` IN ('aberto','em_andamento') AND `instrutor_teto_centavos` IS NOT NULL;

-- ── T4: candidaturas sem lance ──
ALTER TABLE `desafio_candidaturas_instrutor`
  MODIFY COLUMN `lance_centavos` BIGINT NULL COMMENT 'LEGADO (rodada 5): lance do antigo leilão; candidaturas novas = NULL (aceitam as condições do desafio)',
  MODIFY COLUMN `status` ENUM('pendente','escolhida','recusada','retirada','removida') NOT NULL DEFAULT 'pendente' COMMENT 'pendente | escolhida (= aceita, entrou na equipe) | recusada | retirada (pelo instrutor) | removida (da equipe pela academia)',
  COMMENT = 'Candidaturas de instrutores às vagas da equipe do desafio (aceitam as condições; sem lance desde a rodada 5)';

DROP PROCEDURE `_goose_00021_drop`;
DROP PROCEDURE `_goose_00021_add`;

-- +goose Down
-- Volta ao instrutor único + leilão, com perda de informação:
--  * equipe: só o responsável sobrevive (já espelhado em desafios.id_instrutor); demais membros são perdidos.
--    Desafio sem responsável espelhado recebe o primeiro membro (por id) como id_instrutor.
--  * desafio aberto sem instrutor volta a `vaga_aberta` com teto = fixo (CHECK chk_desafios_vaga_aberta_teto).
--  * reserva_academia_centavos e ledger NÃO são mexidos (a apuração antiga usa a reserva gravada; o excedente
--    de vagas > 1 volta à academia como sobra no encerramento/cancelamento).
--  * fixo = teto e modo fixo_percentual da conversão T8 não são desfeitos (inofensivos no modelo antigo).
--  * lance NULL → 0; candidatura `removida` → `recusada`; endereço estruturado e data_fim_evento são perdidos.
--  * `local` só volta a VARCHAR(200) se nenhum valor passar disso.

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00021_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00021_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00021_reverter`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00021_reverter`()
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'desafio_instrutores') THEN
    -- id_instrutor = responsável; sem responsável, o primeiro membro.
    UPDATE `desafios` d
       SET d.`id_instrutor` = COALESCE(
             (SELECT e.`id_instrutor` FROM `desafio_instrutores` e WHERE e.`id_desafio` = d.`id` AND e.`responsavel` = 1),
             (SELECT e.`id_instrutor` FROM `desafio_instrutores` e WHERE e.`id_desafio` = d.`id` ORDER BY e.`id` LIMIT 1),
             d.`id_instrutor`)
     WHERE EXISTS (SELECT 1 FROM `desafio_instrutores` e WHERE e.`id_desafio` = d.`id`);
    DROP TABLE `desafio_instrutores`;
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00021_reverter`();
DROP PROCEDURE `_goose_00021_reverter`;

UPDATE `desafios`
   SET `instrutor_selecao` = 'vaga_aberta', `instrutor_teto_centavos` = COALESCE(`instrutor_teto_centavos`, `instrutor_fixo_centavos`)
 WHERE `id_instrutor` IS NULL AND `id_academia` IS NOT NULL AND `status` = 'aberto';

UPDATE `desafio_candidaturas_instrutor` SET `lance_centavos` = 0 WHERE `lance_centavos` IS NULL;
UPDATE `desafio_candidaturas_instrutor` SET `status` = 'recusada' WHERE `status` = 'removida';
ALTER TABLE `desafio_candidaturas_instrutor`
  MODIFY COLUMN `lance_centavos` BIGINT NOT NULL COMMENT 'Valor fixo que o instrutor cobra; deve ser <= desafios.instrutor_teto_centavos (validado na API)',
  MODIFY COLUMN `status` ENUM('pendente','escolhida','recusada','retirada') NOT NULL DEFAULT 'pendente' COMMENT 'Situação da candidatura',
  COMMENT = 'Candidaturas de instrutores a vagas abertas de desafio (leilão com teto)';

ALTER TABLE `desafios`
  MODIFY COLUMN `id_instrutor` BIGINT UNSIGNED NULL COMMENT 'Instrutor designado que monitora o desafio (NULL só em legados)',
  MODIFY COLUMN `instrutor_selecao` ENUM('designado','vaga_aberta') NOT NULL DEFAULT 'designado' COMMENT 'designado = academia indica o instrutor; vaga_aberta = instrutores se candidatam (id_instrutor NULL até a escolha)',
  MODIFY COLUMN `instrutor_teto_centavos` BIGINT NULL COMMENT 'Teto do lance na vaga aberta (centavos); obrigatório quando instrutor_selecao = vaga_aberta',
  MODIFY COLUMN `instrutor_fixo_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Parte fixa do instrutor (centavos); em vaga aberta passa a ser o lance escolhido',
  MODIFY COLUMN `instrutor_pct` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '% das inscrições (T) pago ao instrutor; sai da parte da academia',
  MODIFY COLUMN `reserva_academia_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Garantia hoje debitada da academia (prêmio fixo + instrutor fixo, ou teto na vaga aberta); devolvida no cancelamento';

-- CHECK de soma antigo (instrutor único). Com instrutores_vagas >= 1 o novo é mais restritivo, então os dados passam.
CALL `_goose_00021_drop`('desafios', 'check', 'chk_desafios_soma_pct_equipe');
CALL `_goose_00021_drop`('desafios', 'check', 'chk_desafios_instrutores_vagas');
CALL `_goose_00021_drop`('desafios', 'coluna', 'instrutores_vagas');
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00021_soma`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00021_soma`()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
                  WHERE table_schema = DATABASE() AND table_name = 'desafios' AND constraint_name = 'chk_desafios_soma_pct') THEN
    ALTER TABLE `desafios` ADD CONSTRAINT `chk_desafios_soma_pct` CHECK (`percentual_pote` + `taxa_plataforma_pct` + `instrutor_pct` <= 100);
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00021_soma`();
DROP PROCEDURE `_goose_00021_soma`;

CALL `_goose_00021_drop`('desafios', 'indice', 'idx_desafios_academia_horario');
CALL `_goose_00021_drop`('desafios', 'check', 'chk_desafios_fim_evento');
CALL `_goose_00021_drop`('desafios', 'coluna', 'data_fim_evento');

CALL `_goose_00021_drop`('desafios', 'indice', 'idx_desafios_uf_cidade');
CALL `_goose_00021_drop`('desafios', 'check', 'chk_desafios_uf');
CALL `_goose_00021_drop`('desafios', 'check', 'chk_desafios_cep');
CALL `_goose_00021_drop`('desafios', 'coluna', 'uf');
CALL `_goose_00021_drop`('desafios', 'coluna', 'cidade');
CALL `_goose_00021_drop`('desafios', 'coluna', 'bairro');
CALL `_goose_00021_drop`('desafios', 'coluna', 'complemento');
CALL `_goose_00021_drop`('desafios', 'coluna', 'numero');
CALL `_goose_00021_drop`('desafios', 'coluna', 'logradouro');
CALL `_goose_00021_drop`('desafios', 'coluna', 'cep');

DROP PROCEDURE `_goose_00021_drop`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00021_local`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00021_local`()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM `desafios` WHERE CHAR_LENGTH(`local`) > 200) THEN
    ALTER TABLE `desafios` MODIFY COLUMN `local` VARCHAR(200) NULL;
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00021_local`();
DROP PROCEDURE `_goose_00021_local`;
