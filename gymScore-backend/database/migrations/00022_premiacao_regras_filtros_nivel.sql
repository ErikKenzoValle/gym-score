-- +goose Up
-- Rodada 6 (D6-1) — premiação configurável, resumo/regras, filtros de nível e idade, Nível GymScore.
--  (U1) `desafios.posicoes_podio` passa de 1..5 para 1..100 (e `podio_desafios.posicao` idem). Nova tabela
--       `desafio_premio_posicoes` (id_desafio, posicao, pct_bp): % de cada posição em pontos-base (10000 = 100%,
--       permite 33,33%). Regras de soma = 10000, não crescente, cada > 0 e "uma linha por posição 1..posicoes_podio"
--       ficam na API (cruzam linhas). Backfill: TODO desafio existente recebe a tabela fixa atual
--       (services/pote.go percentuaisPodio) do seu posicoes_podio (1..5).
--  (U2) `desafios.resumo` VARCHAR(280) e `desafios.regras` TEXT (até 10000 caracteres, CHECK). Obrigatórios
--       (20–280 / 50–10000) só em desafios NOVOS — regra da API. Backfill a partir de `descricao` (legado):
--       resumo = descricao (cortada em 280 com reticências) ou o título; regras = descricao (ou NULL).
--  (U4) `nivel_min`/`nivel_max` (1–10) e `idade_min`/`idade_max` (0–120), NULL = sem filtro, min <= max.
--  (U3) `usuarios.pontuacao_gymscore` (>= 0), `nivel_gymscore` (1–10) e `gymscore_calculado_em` (NULL = nunca
--       calculado). SEM backfill calculado: todos ficam 0 / nível 1 / NULL e o job de reconciliação do backend
--       calcula (as faixas de nível são do backend; ver relatório DEV_DB rodada 6 no PLANO.md).
-- Mesmo padrão da 00011–00021: procedures temporárias, tudo condicional por nome (reaplicável).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00022_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00022_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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
DROP PROCEDURE IF EXISTS `_goose_00022_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Remove CHECK somente se existir (troca dos CHECKs de posição 1..5 -> 1..100).
CREATE PROCEDURE `_goose_00022_drop`(IN p_tabela VARCHAR(64), IN p_nome VARCHAR(64))
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

-- ── U1: nº de premiados até 100 ──
CALL `_goose_00022_drop`('desafios', 'chk_desafios_posicoes_podio');
CALL `_goose_00022_add`('desafios', 'check', 'chk_desafios_posicoes_podio_100', "CONSTRAINT `chk_desafios_posicoes_podio_100` CHECK (`posicoes_podio` BETWEEN 1 AND 100)");
ALTER TABLE `desafios`
  MODIFY COLUMN `posicoes_podio` TINYINT NOT NULL DEFAULT 3 COMMENT 'Quantidade de posições premiadas (1 a 100); a divisão está em desafio_premio_posicoes';
CALL `_goose_00022_drop`('podio_desafios', 'chk_podio_posicao');
CALL `_goose_00022_add`('podio_desafios', 'check', 'chk_podio_posicao_100', "CONSTRAINT `chk_podio_posicao_100` CHECK (`posicao` BETWEEN 1 AND 100)");

-- ── U1: divisão do prêmio por posição ──
CREATE TABLE IF NOT EXISTS `desafio_premio_posicoes` (
  `id_desafio` BIGINT UNSIGNED NOT NULL COMMENT 'Desafio',
  `posicao` TINYINT UNSIGNED NOT NULL COMMENT 'Posição do pódio (1 = primeiro); uma linha para cada posição 1..desafios.posicoes_podio',
  `pct_bp` SMALLINT UNSIGNED NOT NULL COMMENT 'Fatia do prêmio total da posição em pontos-base (10000 = 100%); soma por desafio = 10000 e não crescente (API)',
  PRIMARY KEY (`id_desafio`, `posicao`),
  CONSTRAINT `fk_desafio_premio_posicoes_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `chk_desafio_premio_posicoes_posicao` CHECK (`posicao` BETWEEN 1 AND 100),
  CONSTRAINT `chk_desafio_premio_posicoes_pct` CHECK (`pct_bp` BETWEEN 1 AND 10000)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Divisão do prêmio por posição definida pela academia (rodada 6); legados = tabela fixa antiga';

-- Backfill: tabela fixa antiga (60/30/10 etc.) para todo desafio ainda sem divisão (reaplicável).
INSERT INTO `desafio_premio_posicoes` (`id_desafio`, `posicao`, `pct_bp`)
SELECT d.`id`, t.`posicao`, t.`pct_bp`
  FROM `desafios` d
  JOIN (          SELECT 1 AS n, 1 AS posicao, 10000 AS pct_bp
        UNION ALL SELECT 2, 1, 7000 UNION ALL SELECT 2, 2, 3000
        UNION ALL SELECT 3, 1, 6000 UNION ALL SELECT 3, 2, 3000 UNION ALL SELECT 3, 3, 1000
        UNION ALL SELECT 4, 1, 5000 UNION ALL SELECT 4, 2, 2500 UNION ALL SELECT 4, 3, 1500 UNION ALL SELECT 4, 4, 1000
        UNION ALL SELECT 5, 1, 4000 UNION ALL SELECT 5, 2, 2500 UNION ALL SELECT 5, 3, 1500 UNION ALL SELECT 5, 4, 1200 UNION ALL SELECT 5, 5, 800
       ) t ON t.`n` = d.`posicoes_podio`
 WHERE NOT EXISTS (SELECT 1 FROM `desafio_premio_posicoes` p WHERE p.`id_desafio` = d.`id`);

-- ── U2: resumo e regras ──
CALL `_goose_00022_add`('desafios', 'coluna', 'resumo', "COLUMN `resumo` VARCHAR(280) NULL COMMENT 'Breve descrição (texto puro); obrigatória em desafios novos, 20 a 280 caracteres (API); legados: derivada de descricao' AFTER `descricao`");
CALL `_goose_00022_add`('desafios', 'coluna', 'regras', "COLUMN `regras` TEXT NULL COMMENT 'Regras completas (texto puro); obrigatórias em desafios novos, 50 a 10000 caracteres (API); legados: = descricao' AFTER `resumo`");
UPDATE `desafios`
   SET `resumo` = CASE
         WHEN NULLIF(TRIM(`descricao`), '') IS NULL THEN LEFT(`titulo`, 280)
         WHEN CHAR_LENGTH(TRIM(`descricao`)) > 280 THEN CONCAT(LEFT(TRIM(`descricao`), 279), '…')
         ELSE TRIM(`descricao`) END
 WHERE `resumo` IS NULL;
UPDATE `desafios` SET `regras` = LEFT(TRIM(`descricao`), 10000)
 WHERE `regras` IS NULL AND NULLIF(TRIM(`descricao`), '') IS NOT NULL;
CALL `_goose_00022_add`('desafios', 'check', 'chk_desafios_regras_tamanho', "CONSTRAINT `chk_desafios_regras_tamanho` CHECK (`regras` IS NULL OR CHAR_LENGTH(`regras`) <= 10000)");
ALTER TABLE `desafios`
  MODIFY COLUMN `descricao` TEXT NULL COMMENT 'LEGADO (rodada 6): substituída por resumo + regras; mantida para compatibilidade';

-- ── U4: filtros de nível e idade ──
CALL `_goose_00022_add`('desafios', 'coluna', 'nivel_min', "COLUMN `nivel_min` TINYINT UNSIGNED NULL COMMENT 'Nível GymScore mínimo para entrar (1 a 10; NULL = sem mínimo); conferido na inscrição' AFTER `posicoes_podio`");
CALL `_goose_00022_add`('desafios', 'coluna', 'nivel_max', "COLUMN `nivel_max` TINYINT UNSIGNED NULL COMMENT 'Nível GymScore máximo para entrar (1 a 10; NULL = sem máximo)' AFTER `nivel_min`");
CALL `_goose_00022_add`('desafios', 'coluna', 'idade_min', "COLUMN `idade_min` TINYINT UNSIGNED NULL COMMENT 'Idade mínima na data do evento (0 a 120; NULL = sem mínimo)' AFTER `nivel_max`");
CALL `_goose_00022_add`('desafios', 'coluna', 'idade_max', "COLUMN `idade_max` TINYINT UNSIGNED NULL COMMENT 'Idade máxima na data do evento (0 a 120; NULL = sem máximo)' AFTER `idade_min`");
CALL `_goose_00022_add`('desafios', 'check', 'chk_desafios_nivel_filtro', "CONSTRAINT `chk_desafios_nivel_filtro` CHECK ((`nivel_min` IS NULL OR `nivel_min` BETWEEN 1 AND 10) AND (`nivel_max` IS NULL OR `nivel_max` BETWEEN 1 AND 10) AND (`nivel_min` IS NULL OR `nivel_max` IS NULL OR `nivel_min` <= `nivel_max`))");
CALL `_goose_00022_add`('desafios', 'check', 'chk_desafios_idade_filtro', "CONSTRAINT `chk_desafios_idade_filtro` CHECK ((`idade_min` IS NULL OR `idade_min` <= 120) AND (`idade_max` IS NULL OR `idade_max` <= 120) AND (`idade_min` IS NULL OR `idade_max` IS NULL OR `idade_min` <= `idade_max`))");

-- ── U3: Nível GymScore (cache; calculado pelo backend) ──
CALL `_goose_00022_add`('usuarios', 'coluna', 'pontuacao_gymscore', "COLUMN `pontuacao_gymscore` INT NOT NULL DEFAULT 0 COMMENT 'Cache da pontuação GymScore (pódios + presenças + dias de treino); recalculada pelo backend' AFTER `pontos`");
CALL `_goose_00022_add`('usuarios', 'coluna', 'nivel_gymscore', "COLUMN `nivel_gymscore` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT 'Cache do Nível GymScore (1 a 10), derivado de pontuacao_gymscore pelas faixas do backend' AFTER `pontuacao_gymscore`");
CALL `_goose_00022_add`('usuarios', 'coluna', 'gymscore_calculado_em', "COLUMN `gymscore_calculado_em` DATETIME(3) NULL COMMENT 'Último cálculo da pontuação/nível (NULL = nunca calculado: o job de reconciliação calcula)' AFTER `nivel_gymscore`");
CALL `_goose_00022_add`('usuarios', 'check', 'chk_usuarios_pontuacao_gymscore', "CONSTRAINT `chk_usuarios_pontuacao_gymscore` CHECK (`pontuacao_gymscore` >= 0)");
CALL `_goose_00022_add`('usuarios', 'check', 'chk_usuarios_nivel_gymscore', "CONSTRAINT `chk_usuarios_nivel_gymscore` CHECK (`nivel_gymscore` BETWEEN 1 AND 10)");
-- Job de reconciliação: pega primeiro quem nunca foi calculado / está há mais tempo sem recalcular.
CALL `_goose_00022_add`('usuarios', 'indice', 'idx_usuarios_gymscore_calculado', "KEY `idx_usuarios_gymscore_calculado` (`gymscore_calculado_em`)");
-- Dias de treino por usuário numa janela (pontuação e estatísticas, janela de 180 dias).
CALL `_goose_00022_add`('treino_conclusoes', 'indice', 'idx_treino_conclusoes_usuario_data', "KEY `idx_treino_conclusoes_usuario_data` (`id_usuario`, `concluido_em`)");
-- Pódios do usuário (pontuação: posições em desafios encerrados).
CALL `_goose_00022_add`('podio_desafios', 'indice', 'idx_podio_desafios_participante_posicao', "KEY `idx_podio_desafios_participante_posicao` (`id_participante`, `posicao`, `id_desafio`)");

DROP PROCEDURE `_goose_00022_drop`;
DROP PROCEDURE `_goose_00022_add`;

-- +goose Down
-- Volta à premiação fixa (1..5), sem resumo/regras, sem filtros e sem Nível GymScore, com perda de informação:
--  * a divisão por posição (desafio_premio_posicoes) é descartada: a apuração antiga usa a tabela fixa.
--  * os CHECKs antigos de posição (1..5) só voltam se nenhum dado passar disso (desafio com > 5 premiados
--    ou pódio com posição > 5 deixa a tabela sem esse CHECK; nada é cortado).
--  * resumo, regras, filtros e o cache do Nível GymScore são perdidos (`descricao` continua lá).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00022_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00022_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

CALL `_goose_00022_drop`('podio_desafios', 'indice', 'idx_podio_desafios_participante_posicao');
CALL `_goose_00022_drop`('treino_conclusoes', 'indice', 'idx_treino_conclusoes_usuario_data');
CALL `_goose_00022_drop`('usuarios', 'indice', 'idx_usuarios_gymscore_calculado');
CALL `_goose_00022_drop`('usuarios', 'check', 'chk_usuarios_nivel_gymscore');
CALL `_goose_00022_drop`('usuarios', 'check', 'chk_usuarios_pontuacao_gymscore');
CALL `_goose_00022_drop`('usuarios', 'coluna', 'gymscore_calculado_em');
CALL `_goose_00022_drop`('usuarios', 'coluna', 'nivel_gymscore');
CALL `_goose_00022_drop`('usuarios', 'coluna', 'pontuacao_gymscore');

CALL `_goose_00022_drop`('desafios', 'check', 'chk_desafios_idade_filtro');
CALL `_goose_00022_drop`('desafios', 'check', 'chk_desafios_nivel_filtro');
CALL `_goose_00022_drop`('desafios', 'coluna', 'idade_max');
CALL `_goose_00022_drop`('desafios', 'coluna', 'idade_min');
CALL `_goose_00022_drop`('desafios', 'coluna', 'nivel_max');
CALL `_goose_00022_drop`('desafios', 'coluna', 'nivel_min');

CALL `_goose_00022_drop`('desafios', 'check', 'chk_desafios_regras_tamanho');
CALL `_goose_00022_drop`('desafios', 'coluna', 'regras');
CALL `_goose_00022_drop`('desafios', 'coluna', 'resumo');
ALTER TABLE `desafios`
  MODIFY COLUMN `descricao` TEXT NULL;

DROP TABLE IF EXISTS `desafio_premio_posicoes`;

CALL `_goose_00022_drop`('podio_desafios', 'check', 'chk_podio_posicao_100');
CALL `_goose_00022_drop`('desafios', 'check', 'chk_desafios_posicoes_podio_100');
ALTER TABLE `desafios`
  MODIFY COLUMN `posicoes_podio` TINYINT NOT NULL DEFAULT 3 COMMENT 'Quantidade de posições premiadas (1 a 5)';

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00022_posicoes`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00022_posicoes`()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
                  WHERE table_schema = DATABASE() AND table_name = 'desafios' AND constraint_name = 'chk_desafios_posicoes_podio')
     AND NOT EXISTS (SELECT 1 FROM `desafios` WHERE `posicoes_podio` NOT BETWEEN 1 AND 5) THEN
    ALTER TABLE `desafios` ADD CONSTRAINT `chk_desafios_posicoes_podio` CHECK (`posicoes_podio` BETWEEN 1 AND 5);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
                  WHERE table_schema = DATABASE() AND table_name = 'podio_desafios' AND constraint_name = 'chk_podio_posicao')
     AND NOT EXISTS (SELECT 1 FROM `podio_desafios` WHERE `posicao` NOT BETWEEN 1 AND 5) THEN
    ALTER TABLE `podio_desafios` ADD CONSTRAINT `chk_podio_posicao` CHECK (`posicao` BETWEEN 1 AND 5);
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00022_posicoes`();
DROP PROCEDURE `_goose_00022_posicoes`;

DROP PROCEDURE `_goose_00022_drop`;
