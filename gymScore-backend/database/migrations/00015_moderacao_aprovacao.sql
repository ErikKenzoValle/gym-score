-- +goose Up
-- Moderação pelo painel admin: bloqueio de usuários e aprovação de academias/instrutores.
--  * usuarios.bloqueado: login e tokens recusados enquanto = 1 (bloquear também incrementa token_versao,
--    na aplicação). O histórico completo (quem, quando, por quê) fica em admin_auditoria.
--  * academias.status / instrutores.status: cadastros novos nascem 'pendente' e só operam depois de
--    'aprovada'. Cadastros que já existem viram 'aprovada' (o DEFAULT do ADD COLUMN preenche as linhas
--    atuais; em seguida o DEFAULT passa a 'pendente'). Vale também para instrutor autônomo.
--  * aprovado_por/aprovado_em registram quem decidiu e quando (aprovação OU rejeição).
--
-- Mesmo padrão da 00011/00012: ADD condicional via procedure temporária; MODIFY idempotente.
-- Idempotente sobre o RDS de testes, onde esta migration (então numerada 00014) rodou pela metade sem
-- registro: `usuarios.bloqueado*` e `academias.status/motivo_rejeicao/aprovado_*` + índices já existem
-- (sem a FK), `instrutores` intacta. Tudo aqui é condicional por nome; o índice `fk_academias_aprovado_por`
-- já existente recebe a FK. `usuarios.bloqueado*` é removido depois pela 00019 (bloqueio unificado nas
-- punições da moderação).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00015_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00015_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- ── usuarios: bloqueio ──
CALL `_goose_00015_add`('usuarios', 'coluna', 'bloqueado', "COLUMN `bloqueado` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = conta bloqueada por um administrador (login e tokens recusados)'");
CALL `_goose_00015_add`('usuarios', 'coluna', 'bloqueado_motivo', "COLUMN `bloqueado_motivo` VARCHAR(500) NULL COMMENT 'Motivo do bloqueio vigente (histórico em admin_auditoria)' AFTER `bloqueado`");
CALL `_goose_00015_add`('usuarios', 'coluna', 'bloqueado_em', "COLUMN `bloqueado_em` DATETIME(3) NULL COMMENT 'Quando o bloqueio vigente foi aplicado' AFTER `bloqueado_motivo`");

-- ── academias: aprovação ──
CALL `_goose_00015_add`('academias', 'coluna', 'status', "COLUMN `status` ENUM('pendente','aprovada','rejeitada') NOT NULL DEFAULT 'aprovada' COMMENT 'Situação do cadastro no painel admin'");
CALL `_goose_00015_add`('academias', 'coluna', 'motivo_rejeicao', "COLUMN `motivo_rejeicao` VARCHAR(500) NULL COMMENT 'Preenchido quando status = rejeitada' AFTER `status`");
CALL `_goose_00015_add`('academias', 'coluna', 'aprovado_por', "COLUMN `aprovado_por` BIGINT UNSIGNED NULL COMMENT 'Administrador que aprovou/rejeitou' AFTER `motivo_rejeicao`");
CALL `_goose_00015_add`('academias', 'coluna', 'aprovado_em', "COLUMN `aprovado_em` DATETIME(3) NULL COMMENT 'Quando o cadastro foi aprovado/rejeitado' AFTER `aprovado_por`");
ALTER TABLE `academias`
  MODIFY COLUMN `status` ENUM('pendente','aprovada','rejeitada') NOT NULL DEFAULT 'pendente' COMMENT 'Situação do cadastro no painel admin';
CALL `_goose_00015_add`('academias', 'indice', 'idx_academias_status', "KEY `idx_academias_status` (`status`, `criado_em`)");
CALL `_goose_00015_add`('academias', 'indice', 'fk_academias_aprovado_por', "KEY `fk_academias_aprovado_por` (`aprovado_por`)");
CALL `_goose_00015_add`('academias', 'fk', 'fk_academias_aprovado_por', "CONSTRAINT `fk_academias_aprovado_por` FOREIGN KEY (`aprovado_por`) REFERENCES `administradores` (`id`)");

-- ── instrutores: aprovação ──
CALL `_goose_00015_add`('instrutores', 'coluna', 'status', "COLUMN `status` ENUM('pendente','aprovada','rejeitada') NOT NULL DEFAULT 'aprovada' COMMENT 'Situação do cadastro no painel admin'");
CALL `_goose_00015_add`('instrutores', 'coluna', 'motivo_rejeicao', "COLUMN `motivo_rejeicao` VARCHAR(500) NULL COMMENT 'Preenchido quando status = rejeitada' AFTER `status`");
CALL `_goose_00015_add`('instrutores', 'coluna', 'aprovado_por', "COLUMN `aprovado_por` BIGINT UNSIGNED NULL COMMENT 'Administrador que aprovou/rejeitou' AFTER `motivo_rejeicao`");
CALL `_goose_00015_add`('instrutores', 'coluna', 'aprovado_em', "COLUMN `aprovado_em` DATETIME(3) NULL COMMENT 'Quando o cadastro foi aprovado/rejeitado' AFTER `aprovado_por`");
ALTER TABLE `instrutores`
  MODIFY COLUMN `status` ENUM('pendente','aprovada','rejeitada') NOT NULL DEFAULT 'pendente' COMMENT 'Situação do cadastro no painel admin';
CALL `_goose_00015_add`('instrutores', 'indice', 'idx_instrutores_status', "KEY `idx_instrutores_status` (`status`, `criado_em`)");
CALL `_goose_00015_add`('instrutores', 'indice', 'fk_instrutores_aprovado_por', "KEY `fk_instrutores_aprovado_por` (`aprovado_por`)");
CALL `_goose_00015_add`('instrutores', 'fk', 'fk_instrutores_aprovado_por', "CONSTRAINT `fk_instrutores_aprovado_por` FOREIGN KEY (`aprovado_por`) REFERENCES `administradores` (`id`)");

DROP PROCEDURE `_goose_00015_add`;

-- +goose Down
-- Reversão com perda de informação: bloqueios, status de aprovação e motivos são apagados (o histórico
-- continua em admin_auditoria até o Down da 00014). Todos os cadastros voltam a operar sem aprovação.
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00015_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00015_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

CALL `_goose_00015_drop`('instrutores', 'fk', 'fk_instrutores_aprovado_por');
CALL `_goose_00015_drop`('instrutores', 'indice', 'fk_instrutores_aprovado_por');
CALL `_goose_00015_drop`('instrutores', 'indice', 'idx_instrutores_status');
CALL `_goose_00015_drop`('instrutores', 'coluna', 'aprovado_em');
CALL `_goose_00015_drop`('instrutores', 'coluna', 'aprovado_por');
CALL `_goose_00015_drop`('instrutores', 'coluna', 'motivo_rejeicao');
CALL `_goose_00015_drop`('instrutores', 'coluna', 'status');

CALL `_goose_00015_drop`('academias', 'fk', 'fk_academias_aprovado_por');
CALL `_goose_00015_drop`('academias', 'indice', 'fk_academias_aprovado_por');
CALL `_goose_00015_drop`('academias', 'indice', 'idx_academias_status');
CALL `_goose_00015_drop`('academias', 'coluna', 'aprovado_em');
CALL `_goose_00015_drop`('academias', 'coluna', 'aprovado_por');
CALL `_goose_00015_drop`('academias', 'coluna', 'motivo_rejeicao');
CALL `_goose_00015_drop`('academias', 'coluna', 'status');

CALL `_goose_00015_drop`('usuarios', 'coluna', 'bloqueado_em');
CALL `_goose_00015_drop`('usuarios', 'coluna', 'bloqueado_motivo');
CALL `_goose_00015_drop`('usuarios', 'coluna', 'bloqueado');

DROP PROCEDURE `_goose_00015_drop`;
