-- +goose Up
-- Rodada 4 (modo atleta/instrutor + endereço estruturado):
--  (D4-1) `academias` ganha o endereço estruturado: `cep` (8 dígitos, sem máscara), `logradouro`, `numero`,
--         `complemento`, `bairro`, `cidade`, `uf` (CHAR(2) ascii_bin, CHECK das 27 UFs em maiúsculas). Tudo NULL: academias existentes
--         mantêm só o texto antigo em `endereco` até editarem (sem backfill). `endereco` continua sendo o
--         texto formatado (compatibilidade) e passa a VARCHAR(500) para caber a montagem a partir dos campos.
--  (D4-2) `usuarios.perfil` passa a ser o TIPO DE CONTA: `atleta` | `academia`. O papel de instrutor é um
--         segundo modo da mesma conta de atleta e vem da linha em `instrutores` (status pendente/aprovada/
--         rejeitada). Contas com perfil = 'instrutor' voltam a `atleta` (a linha em `instrutores` fica).
--
-- Aborta (SIGNAL) se houver conta de academia com registro de instrutor (dado incoerente com o modelo novo:
-- academia não tem modo instrutor) — não reinterpreta dado ambíguo.
-- Mesmo padrão da 00011–00019: procedures temporárias, tudo condicional por nome (reaplicável).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00020_verificar`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00020_verificar`()
BEGIN
  IF EXISTS (SELECT 1 FROM `instrutores` i JOIN `usuarios` u ON u.`id` = i.`id_usuario` WHERE u.`perfil` = 'academia') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00020: conta de academia com registro em instrutores - corrigir manualmente';
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00020_verificar`();
DROP PROCEDURE `_goose_00020_verificar`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00020_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00020_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- ── D4-1: academias — endereço estruturado ──
ALTER TABLE `academias`
  MODIFY COLUMN `endereco` VARCHAR(500) NULL COMMENT 'Endereço em texto formatado (montado pelo backend a partir dos campos estruturados; academias antigas: texto livre)';
CALL `_goose_00020_add`('academias', 'coluna', 'cep', "COLUMN `cep` CHAR(8) CHARACTER SET ascii COLLATE ascii_bin NULL COMMENT 'CEP só com dígitos (8), sem máscara' AFTER `endereco`");
CALL `_goose_00020_add`('academias', 'coluna', 'logradouro', "COLUMN `logradouro` VARCHAR(150) NULL COMMENT 'Rua/avenida' AFTER `cep`");
CALL `_goose_00020_add`('academias', 'coluna', 'numero', "COLUMN `numero` VARCHAR(20) NULL COMMENT 'Número (texto: aceita S/N, 12A)' AFTER `logradouro`");
CALL `_goose_00020_add`('academias', 'coluna', 'complemento', "COLUMN `complemento` VARCHAR(100) NULL COMMENT 'Complemento (opcional)' AFTER `numero`");
CALL `_goose_00020_add`('academias', 'coluna', 'bairro', "COLUMN `bairro` VARCHAR(100) NULL COMMENT 'Bairro' AFTER `complemento`");
CALL `_goose_00020_add`('academias', 'coluna', 'cidade', "COLUMN `cidade` VARCHAR(100) NULL COMMENT 'Município' AFTER `bairro`");
CALL `_goose_00020_add`('academias', 'coluna', 'uf', "COLUMN `uf` CHAR(2) CHARACTER SET ascii COLLATE ascii_bin NULL COMMENT 'Sigla da UF (maiúsculas; collation binária: sp é recusado)' AFTER `cidade`");
CALL `_goose_00020_add`('academias', 'check', 'chk_academias_cep', "CONSTRAINT `chk_academias_cep` CHECK (`cep` IS NULL OR REGEXP_LIKE(`cep`, '^[0-9]{8}$', 'c'))");
CALL `_goose_00020_add`('academias', 'check', 'chk_academias_uf', "CONSTRAINT `chk_academias_uf` CHECK (`uf` IS NULL OR `uf` IN ('AC','AL','AP','AM','BA','CE','DF','ES','GO','MA','MT','MS','MG','PA','PB','PR','PE','PI','RJ','RN','RS','RO','RR','SC','SP','SE','TO'))");
CALL `_goose_00020_add`('academias', 'indice', 'idx_academias_uf_cidade', "KEY `idx_academias_uf_cidade` (`uf`, `cidade`)");

DROP PROCEDURE `_goose_00020_add`;

-- ── D4-2: usuarios.perfil = tipo de conta ──
-- O modo instrutor vem de `instrutores` (o registro e o status do pedido não mudam).
UPDATE `usuarios` SET `perfil` = 'atleta' WHERE `perfil` = 'instrutor';
ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia') NOT NULL DEFAULT 'atleta' COMMENT 'Tipo de conta: atleta (pode ter também o modo instrutor, via instrutores) ou academia';

-- +goose Down
-- Volta ao modelo de perfil exclusivo: conta de atleta com registro em `instrutores` (qualquer status) volta a
-- perfil = 'instrutor' (era o invariante antigo: o pedido de instrutor mudava o perfil na mesma transação).
-- Os campos estruturados do endereço são perdidos (o texto em `endereco` fica). `endereco` só volta a
-- VARCHAR(255) se nenhum valor passar disso (senão fica VARCHAR(500), para não truncar).
ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia','instrutor') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta (admins ficam em administradores)';
UPDATE `usuarios` u JOIN `instrutores` i ON i.`id_usuario` = u.`id` SET u.`perfil` = 'instrutor' WHERE u.`perfil` = 'atleta';

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00020_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00020_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

CALL `_goose_00020_drop`('academias', 'indice', 'idx_academias_uf_cidade');
CALL `_goose_00020_drop`('academias', 'check', 'chk_academias_uf');
CALL `_goose_00020_drop`('academias', 'check', 'chk_academias_cep');
CALL `_goose_00020_drop`('academias', 'coluna', 'uf');
CALL `_goose_00020_drop`('academias', 'coluna', 'cidade');
CALL `_goose_00020_drop`('academias', 'coluna', 'bairro');
CALL `_goose_00020_drop`('academias', 'coluna', 'complemento');
CALL `_goose_00020_drop`('academias', 'coluna', 'numero');
CALL `_goose_00020_drop`('academias', 'coluna', 'logradouro');
CALL `_goose_00020_drop`('academias', 'coluna', 'cep');

DROP PROCEDURE `_goose_00020_drop`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00020_endereco`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00020_endereco`()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM `academias` WHERE CHAR_LENGTH(`endereco`) > 255) THEN
    ALTER TABLE `academias` MODIFY COLUMN `endereco` VARCHAR(255) NULL;
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00020_endereco`();
DROP PROCEDURE `_goose_00020_endereco`;
