-- +goose Up
-- Pendências do backend parte 1 (D14):
--  * Convite de vínculo academia -> instrutor (B2/N3): a academia não vincula mais direto; grava um
--    convite em instrutores.id_academia_convite/convite_em e o instrutor aceita (vira id_academia) ou
--    recusa (limpa o convite). Um convite pendente por instrutor (o mais recente da mesma academia
--    renova convite_em). Academia excluída apaga o convite (ON DELETE SET NULL).
--  * Quem e como marcou a presença (B11/N6): desafio_participantes.presenca_marcada_por (FK usuarios,
--    ON DELETE SET NULL) e presenca_modo ENUM('qr','manual'). Presenças antigas ficam NULL (desconhecido).
--
-- Mesmo padrão da 00011–00017: ADD condicional via procedure temporária (reaplicável).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00018_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00018_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- ── instrutores: convite de vínculo ──
CALL `_goose_00018_add`('instrutores', 'coluna', 'id_academia_convite', "COLUMN `id_academia_convite` BIGINT UNSIGNED NULL COMMENT 'Academia que convidou o instrutor (convite pendente; NULL = sem convite)' AFTER `id_academia`");
CALL `_goose_00018_add`('instrutores', 'coluna', 'convite_em', "COLUMN `convite_em` DATETIME(3) NULL COMMENT 'Quando o convite pendente foi feito' AFTER `id_academia_convite`");
CALL `_goose_00018_add`('instrutores', 'indice', 'idx_instrutores_convite', "KEY `idx_instrutores_convite` (`id_academia_convite`, `convite_em`)");
CALL `_goose_00018_add`('instrutores', 'fk', 'fk_instrutores_academia_convite', "CONSTRAINT `fk_instrutores_academia_convite` FOREIGN KEY (`id_academia_convite`) REFERENCES `academias` (`id`) ON DELETE SET NULL");

-- ── desafio_participantes: quem e como marcou a presença ──
CALL `_goose_00018_add`('desafio_participantes', 'coluna', 'presenca_marcada_por', "COLUMN `presenca_marcada_por` BIGINT UNSIGNED NULL COMMENT 'Usuário (staff) que confirmou a presença' AFTER `presente_em`");
CALL `_goose_00018_add`('desafio_participantes', 'coluna', 'presenca_modo', "COLUMN `presenca_modo` ENUM('qr','manual') NULL COMMENT 'Como a presença foi confirmada (leitura do QR ou lista)' AFTER `presenca_marcada_por`");
CALL `_goose_00018_add`('desafio_participantes', 'indice', 'fk_participantes_presenca_marcada_por', "KEY `fk_participantes_presenca_marcada_por` (`presenca_marcada_por`)");
CALL `_goose_00018_add`('desafio_participantes', 'fk', 'fk_participantes_presenca_marcada_por', "CONSTRAINT `fk_participantes_presenca_marcada_por` FOREIGN KEY (`presenca_marcada_por`) REFERENCES `usuarios` (`id`) ON DELETE SET NULL");

DROP PROCEDURE `_goose_00018_add`;

-- +goose Down
-- Reversão com perda de informação: convites pendentes e autoria/modo das presenças são apagados.
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00018_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00018_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

CALL `_goose_00018_drop`('desafio_participantes', 'fk', 'fk_participantes_presenca_marcada_por');
CALL `_goose_00018_drop`('desafio_participantes', 'indice', 'fk_participantes_presenca_marcada_por');
CALL `_goose_00018_drop`('desafio_participantes', 'coluna', 'presenca_modo');
CALL `_goose_00018_drop`('desafio_participantes', 'coluna', 'presenca_marcada_por');

CALL `_goose_00018_drop`('instrutores', 'fk', 'fk_instrutores_academia_convite');
CALL `_goose_00018_drop`('instrutores', 'indice', 'idx_instrutores_convite');
CALL `_goose_00018_drop`('instrutores', 'coluna', 'convite_em');
CALL `_goose_00018_drop`('instrutores', 'coluna', 'id_academia_convite');

DROP PROCEDURE `_goose_00018_drop`;
