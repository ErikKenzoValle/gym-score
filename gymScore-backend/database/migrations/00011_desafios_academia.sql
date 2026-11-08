-- +goose Up
-- Sprint 6: fluxo Academia -> Instrutor -> Atleta. Só a academia cria desafio e designa um instrutor;
-- atleta se inscreve pagando a entrada; o instrutor lê o QR do ingresso (presença) e lança o pódio;
-- a votação entre atletas deixa de existir. Tabelas de votos e check-ins são mantidas (histórico).
--
-- Escrita para funcionar num banco novo (00001-00010) e no RDS de testes (que já teve colunas/tabelas
-- criadas por AutoMigrate legado): todo ADD/DROP é condicional via procedures temporárias (removidas
-- no fim) e o resto (UPDATE/MODIFY) é idempotente. Ver `database/README.md`.

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00011_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00011_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- ── desafios: dono (academia), instrutor designado, data do evento, categoria, tamanho do pódio ──
-- id_academia/id_instrutor/data_evento são NULL só para desafios legados (criados antes do Sprint 6);
-- a API exige os três na criação.
CALL `_goose_00011_add`('desafios', 'coluna', 'id_academia', "COLUMN `id_academia` BIGINT UNSIGNED NULL COMMENT 'Academia dona do desafio (NULL só em legados)' AFTER `id_criador`");
CALL `_goose_00011_add`('desafios', 'coluna', 'id_instrutor', "COLUMN `id_instrutor` BIGINT UNSIGNED NULL COMMENT 'Instrutor designado que monitora o desafio (NULL só em legados)' AFTER `id_academia`");
CALL `_goose_00011_add`('desafios', 'coluna', 'data_evento', "COLUMN `data_evento` DATETIME(3) NULL COMMENT 'Data/hora em que o desafio acontece presencialmente' AFTER `local`");
CALL `_goose_00011_add`('desafios', 'coluna', 'categoria', "COLUMN `categoria` VARCHAR(30) NOT NULL DEFAULT 'geral' COMMENT 'Categoria do desafio (chips do feed)' AFTER `descricao`");
CALL `_goose_00011_add`('desafios', 'coluna', 'posicoes_podio', "COLUMN `posicoes_podio` TINYINT NOT NULL DEFAULT 3 COMMENT 'Quantidade de posições premiadas (1 a 5)' AFTER `vagas`");

-- Backfill dos legados: academia dona = academia do criador; data_evento = data_encerramento previsto.
UPDATE `desafios` d JOIN `academias` a ON a.`id_usuario` = d.`id_criador`
   SET d.`id_academia` = a.`id` WHERE d.`id_academia` IS NULL;
UPDATE `desafios` SET `data_evento` = `data_encerramento` WHERE `data_evento` IS NULL AND `data_encerramento` IS NOT NULL;
UPDATE `desafios` SET `categoria` = 'geral' WHERE `categoria` IS NULL OR `categoria` = '';
UPDATE `desafios` SET `posicoes_podio` = LEAST(GREATEST(COALESCE(`posicoes_podio`, 3), 1), 5);
UPDATE `desafios` SET `posicoes_podio` = GREATEST(LEAST(`posicoes_podio`, `vagas`), 1) WHERE `vagas` >= 1;

ALTER TABLE `desafios`
  MODIFY COLUMN `categoria` VARCHAR(30) NOT NULL DEFAULT 'geral' COMMENT 'Categoria do desafio (chips do feed)',
  MODIFY COLUMN `posicoes_podio` TINYINT NOT NULL DEFAULT 3 COMMENT 'Quantidade de posições premiadas (1 a 5)';

CALL `_goose_00011_add`('desafios', 'check', 'chk_desafios_posicoes_podio', "CONSTRAINT `chk_desafios_posicoes_podio` CHECK (`posicoes_podio` BETWEEN 1 AND 5)");
-- Entrada de 0 = desafio gratuito. (Teto de R$ 10.000 fica na API: legados podem ter valor maior.)
CALL `_goose_00011_add`('desafios', 'check', 'chk_desafios_entrada_nao_negativa', "CONSTRAINT `chk_desafios_entrada_nao_negativa` CHECK (`valor_entrada_centavos` >= 0)");
ALTER TABLE `desafios`
  MODIFY COLUMN `valor_entrada_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Entrada por participante, em centavos (0 = gratuito)';

CALL `_goose_00011_add`('desafios', 'indice', 'idx_desafios_feed', "KEY `idx_desafios_feed` (`status`, `data_evento`)");
CALL `_goose_00011_add`('desafios', 'indice', 'idx_desafios_instrutor_status', "KEY `idx_desafios_instrutor_status` (`id_instrutor`, `status`)");
CALL `_goose_00011_add`('desafios', 'indice', 'idx_desafios_academia_status', "KEY `idx_desafios_academia_status` (`id_academia`, `status`)");
CALL `_goose_00011_add`('desafios', 'fk', 'fk_desafios_academia', "CONSTRAINT `fk_desafios_academia` FOREIGN KEY (`id_academia`) REFERENCES `academias` (`id`)");
CALL `_goose_00011_add`('desafios', 'fk', 'fk_desafios_instrutor', "CONSTRAINT `fk_desafios_instrutor` FOREIGN KEY (`id_instrutor`) REFERENCES `instrutores` (`id`)");

-- ── Desafios legados abertos criados por quem NÃO é academia: cancelar + estornar pelo ledger ──
-- Tudo numa transação dentro de uma procedure (MySQL não tem DDL transacional, mas aqui só há DML,
-- então estorno + saldo + cancelamento são atômicos). O valor estornado de cada (usuário, desafio) é
-- calculado a partir do próprio ledger (-SUM de entrada_desafio + estorno_desafio efetivos), não do
-- valor_entrada_centavos: quem já saiu do desafio (estorno prévio) dá 0 e não é estornado duas vezes,
-- e rodar de novo não duplica nada. Preserva a invariante saldo_centavos == SUM(ledger efetivo).
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00011_estornar_legados`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00011_estornar_legados`()
BEGIN
  DECLARE EXIT HANDLER FOR SQLEXCEPTION
  BEGIN
    ROLLBACK;
    RESIGNAL;
  END;

  START TRANSACTION;

  DROP TEMPORARY TABLE IF EXISTS `_goose_00011_tmp_desafios`;
  CREATE TEMPORARY TABLE `_goose_00011_tmp_desafios` (`id` BIGINT UNSIGNED NOT NULL PRIMARY KEY, `titulo` VARCHAR(200) NOT NULL);
  INSERT INTO `_goose_00011_tmp_desafios` (`id`, `titulo`)
  SELECT d.`id`, d.`titulo`
    FROM `desafios` d JOIN `usuarios` u ON u.`id` = d.`id_criador`
   WHERE d.`status` IN ('aberto', 'em_andamento', 'em_votacao') AND u.`perfil` <> 'academia';

  DROP TEMPORARY TABLE IF EXISTS `_goose_00011_tmp_estornos`;
  CREATE TEMPORARY TABLE `_goose_00011_tmp_estornos` (
    `id_usuario` BIGINT UNSIGNED NOT NULL, `id_desafio` BIGINT UNSIGNED NOT NULL, `valor` BIGINT NOT NULL,
    PRIMARY KEY (`id_usuario`, `id_desafio`)
  );
  INSERT INTO `_goose_00011_tmp_estornos` (`id_usuario`, `id_desafio`, `valor`)
  SELECT t.`id_usuario`, t.`id_desafio`, -SUM(t.`valor_centavos`)
    FROM `transacoes` t JOIN `_goose_00011_tmp_desafios` x ON x.`id` = t.`id_desafio`
   WHERE t.`tipo` IN ('entrada_desafio', 'estorno_desafio') AND t.`status` IN ('received', 'concluido')
   GROUP BY t.`id_usuario`, t.`id_desafio`
  HAVING -SUM(t.`valor_centavos`) > 0;

  INSERT INTO `transacoes` (`id_usuario`, `tipo`, `valor_centavos`, `id_desafio`, `asaas_payment_id`, `status`, `descricao`, `created_at`, `updated_at`)
  SELECT e.`id_usuario`, 'estorno_desafio', e.`valor`, e.`id_desafio`, NULL, 'concluido',
         CONCAT('Estorno: desafio #', e.`id_desafio`, ' cancelado (migração Sprint 6)'), NOW(3), NOW(3)
    FROM `_goose_00011_tmp_estornos` e;

  UPDATE `usuarios` u
    JOIN (SELECT `id_usuario`, SUM(`valor`) AS total FROM `_goose_00011_tmp_estornos` GROUP BY `id_usuario`) s ON s.`id_usuario` = u.`id`
     SET u.`saldo_centavos` = u.`saldo_centavos` + s.total;

  INSERT INTO `notificacoes` (`id_usuario`, `tipo`, `titulo`, `mensagem`, `link`, `criado_em`)
  SELECT e.`id_usuario`, 'desafio_cancelado', 'Desafio cancelado',
         CONCAT('O desafio "', x.`titulo`, '" foi cancelado e sua entrada foi reembolsada.'),
         CONCAT('/desafios?id=', e.`id_desafio`), NOW(3)
    FROM `_goose_00011_tmp_estornos` e JOIN `_goose_00011_tmp_desafios` x ON x.`id` = e.`id_desafio`;

  UPDATE `desafios` d JOIN `_goose_00011_tmp_desafios` x ON x.`id` = d.`id` SET d.`status` = 'cancelado';

  COMMIT;
  DROP TEMPORARY TABLE `_goose_00011_tmp_estornos`;
  DROP TEMPORARY TABLE `_goose_00011_tmp_desafios`;
END;
-- +goose StatementEnd
CALL `_goose_00011_estornar_legados`();
DROP PROCEDURE `_goose_00011_estornar_legados`;

-- ── Status: votação removida (em_votacao -> em_andamento) ──
UPDATE `desafios` SET `status` = 'em_andamento' WHERE `status` = 'em_votacao';
ALTER TABLE `desafios`
  MODIFY COLUMN `status` ENUM('aberto','em_andamento','encerrado','cancelado') NOT NULL DEFAULT 'aberto' COMMENT 'Ciclo de vida do desafio';

-- ── desafio_participantes: ingresso (QR) e momento da presença ──
-- codigo_ingresso: 32 hex = 128 bits aleatórios (RANDOM_BYTES = CSPRNG do OpenSSL), UNIQUE. Linhas
-- existentes são preenchidas aqui; linhas novas ganham o DEFAULT (expressão), mas a API deve gerar o
-- código (crypto/rand) e enviá-lo explicitamente.
CALL `_goose_00011_add`('desafio_participantes', 'coluna', 'codigo_ingresso', "COLUMN `codigo_ingresso` CHAR(32) NULL COMMENT 'Token aleatório (128 bits, hex) lido no QR do ingresso' AFTER `id_usuario`");
CALL `_goose_00011_add`('desafio_participantes', 'coluna', 'presente_em', "COLUMN `presente_em` DATETIME(3) NULL COMMENT 'Quando a presença foi confirmada pelo instrutor/academia (NULL = ausente)' AFTER `presente`");
UPDATE `desafio_participantes` SET `codigo_ingresso` = LOWER(HEX(RANDOM_BYTES(16))) WHERE `codigo_ingresso` IS NULL;
UPDATE `desafio_participantes` SET `presente_em` = NOW(3) WHERE `presente` = 1 AND `presente_em` IS NULL;
ALTER TABLE `desafio_participantes`
  MODIFY COLUMN `codigo_ingresso` CHAR(32) NOT NULL DEFAULT (LOWER(HEX(RANDOM_BYTES(16)))) COMMENT 'Token aleatório (128 bits, hex) lido no QR do ingresso';
CALL `_goose_00011_add`('desafio_participantes', 'indice', 'uq_participante_codigo_ingresso', "UNIQUE KEY `uq_participante_codigo_ingresso` (`codigo_ingresso`)");

-- ── podio_desafios: 1 a 5 posições, prêmio em centavos, participante sem repetição ──
CALL `_goose_00011_add`('podio_desafios', 'coluna', 'premio_centavos', "COLUMN `premio_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Prêmio creditado a esta posição, em centavos' AFTER `marca`");
UPDATE `podio_desafios` p JOIN `desafio_participantes` dp ON dp.`id_desafio` = p.`id_desafio` AND dp.`id_usuario` = p.`id_participante`
   SET p.`premio_centavos` = dp.`premio_centavos` WHERE p.`premio_centavos` = 0 AND dp.`premio_centavos` > 0;
-- O pódio só aceita quem é participante do desafio: garante a linha em desafio_participantes p/ FK composta.
INSERT IGNORE INTO `desafio_participantes` (`id_desafio`, `id_usuario`, `criado_em`)
SELECT DISTINCT p.`id_desafio`, p.`id_participante`, NOW(3) FROM `podio_desafios` p;
UPDATE `desafio_participantes` SET `codigo_ingresso` = LOWER(HEX(RANDOM_BYTES(16))) WHERE `codigo_ingresso` = '' OR `codigo_ingresso` IS NULL;
CALL `_goose_00011_add`('podio_desafios', 'check', 'chk_podio_posicao', "CONSTRAINT `chk_podio_posicao` CHECK (`posicao` BETWEEN 1 AND 5)");
CALL `_goose_00011_add`('podio_desafios', 'check', 'chk_podio_premio_nao_negativo', "CONSTRAINT `chk_podio_premio_nao_negativo` CHECK (`premio_centavos` >= 0)");
CALL `_goose_00011_add`('podio_desafios', 'indice', 'uq_podio_participante', "UNIQUE KEY `uq_podio_participante` (`id_desafio`, `id_participante`)");
CALL `_goose_00011_add`('podio_desafios', 'fk', 'fk_podio_desafios_inscricao', "CONSTRAINT `fk_podio_desafios_inscricao` FOREIGN KEY (`id_desafio`, `id_participante`) REFERENCES `desafio_participantes` (`id_desafio`, `id_usuario`)");

DROP PROCEDURE `_goose_00011_add`;

-- +goose Down
-- Reversão com perda de informação (mesmo padrão dos Downs de 2/3/4/10):
--  * desafios legados cancelados + estornos da migração NÃO são desfeitos (o dinheiro já voltou aos
--    usuários e reverter quebraria a invariante saldo == ledger);
--  * perde-se dono/instrutor/data/categoria/pódio configurado, ingressos e prêmios do pódio.
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00011_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00011_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

CALL `_goose_00011_drop`('podio_desafios', 'fk', 'fk_podio_desafios_inscricao');
CALL `_goose_00011_drop`('podio_desafios', 'indice', 'uq_podio_participante');
CALL `_goose_00011_drop`('podio_desafios', 'check', 'chk_podio_premio_nao_negativo');
CALL `_goose_00011_drop`('podio_desafios', 'check', 'chk_podio_posicao');
CALL `_goose_00011_drop`('podio_desafios', 'coluna', 'premio_centavos');

CALL `_goose_00011_drop`('desafio_participantes', 'indice', 'uq_participante_codigo_ingresso');
CALL `_goose_00011_drop`('desafio_participantes', 'coluna', 'codigo_ingresso');
CALL `_goose_00011_drop`('desafio_participantes', 'coluna', 'presente_em');

ALTER TABLE `desafios`
  MODIFY COLUMN `status` ENUM('aberto','em_andamento','em_votacao','encerrado','cancelado') NOT NULL DEFAULT 'aberto' COMMENT 'Ciclo de vida do desafio';

CALL `_goose_00011_drop`('desafios', 'fk', 'fk_desafios_instrutor');
CALL `_goose_00011_drop`('desafios', 'fk', 'fk_desafios_academia');
CALL `_goose_00011_drop`('desafios', 'indice', 'idx_desafios_academia_status');
CALL `_goose_00011_drop`('desafios', 'indice', 'idx_desafios_instrutor_status');
CALL `_goose_00011_drop`('desafios', 'indice', 'idx_desafios_feed');
CALL `_goose_00011_drop`('desafios', 'check', 'chk_desafios_entrada_nao_negativa');
CALL `_goose_00011_drop`('desafios', 'check', 'chk_desafios_posicoes_podio');
CALL `_goose_00011_drop`('desafios', 'coluna', 'posicoes_podio');
CALL `_goose_00011_drop`('desafios', 'coluna', 'categoria');
CALL `_goose_00011_drop`('desafios', 'coluna', 'data_evento');
CALL `_goose_00011_drop`('desafios', 'coluna', 'id_instrutor');
CALL `_goose_00011_drop`('desafios', 'coluna', 'id_academia');

DROP PROCEDURE `_goose_00011_drop`;
