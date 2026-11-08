-- +goose Up
-- Integra a moderação da Sprint 7 (00013, reconstruída do RDS) às decisões da rodada 3:
--  (a) Atores admin em `administradores` (não em `usuarios`). Regra adotada por coluna:
--      * Coluna em que SÓ admin age → a FK troca de alvo (mesma coluna): `punicoes.revogada_por`
--        (fk_punicoes_revogada_por_admin) e `recursos.julgado_por` (fk_recursos_julgado_por_admin).
--      * Coluna em que admin OU conta da academia age → coluna irmã `*_admin` (FK administradores) +
--        CHECK de exclusividade: `punicoes.aplicada_por` vira NULL e ganha `aplicada_por_admin`
--        (exatamente um preenchido; escopo global ⇒ admin), `denuncias.id_responsavel` (academia) ganha
--        `id_responsavel_admin` (no máximo um preenchido), `moderacao_eventos.id_ator` (usuário/academia)
--        ganha `id_admin` (ator_papel = admin ⇔ id_admin; sistema ⇒ nenhum ator; usuario/academia ⇒ id_ator).
--      * `usuarios.perfil` perde o valor 'admin' (nunca usado; admins só em `administradores`).
--  (b) Bloqueio unificado: o bloqueio do painel vira punição (banimento). `usuarios.bloqueado*` (da 00015)
--      sai; contas com bloqueado = 1 viram punição `banimento` + `banido_em`/`motivo_bloqueio` antes do DROP.
--  (c) Q6: `plataforma_receitas` aceita `estorno_taxa` (valor NEGATIVO, sempre com id_desafio) para
--      devolver a taxa já liberada quando a anulação devolve as inscrições. A soma da tabela continua sendo a
--      receita líquida da plataforma (a conferência do painel fecha). UNIQUE (id_desafio, origem) mantido:
--      no máximo uma taxa e um estorno por desafio.
--  (d) Índices dos jobs: pagamentos_pendentes(status, liberar_em) e denuncias(status, jurisdicao,
--      prazo_academia_ate) já existem na 00013 (idx_pendentes_job, idx_denuncias_escalonamento); aqui entram
--      punicoes(tipo, revogada_em, fim) e usuarios(suspenso_ate) para o job de fim de suspensão.
--
-- A migration NÃO reinterpreta dados ambíguos: se as tabelas da moderação já tiverem atores admin gravados
-- como `usuarios` (código da Sprint 7), ela aborta com SIGNAL (no RDS de testes estão vazias).
-- Mesmo padrão da 00011/00012: procedures temporárias, tudo condicional por nome (reaplicável).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00019_verificar`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00019_verificar`()
BEGIN
  IF EXISTS (SELECT 1 FROM `usuarios` WHERE `perfil` = 'admin') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00019: existem usuarios com perfil admin - migrar para administradores antes';
  END IF;
  -- Atores admin gravados como usuarios (só se a coluna nova ainda não existe = primeira execução).
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE()
                  AND table_name = 'punicoes' AND column_name = 'aplicada_por_admin') THEN
    IF EXISTS (SELECT 1 FROM `punicoes` WHERE `revogada_por` IS NOT NULL OR `escopo` = 'global')
       OR EXISTS (SELECT 1 FROM `recursos` WHERE `julgado_por` IS NOT NULL)
       OR EXISTS (SELECT 1 FROM `denuncias` WHERE `id_responsavel` IS NOT NULL AND `jurisdicao` = 'admin')
       OR EXISTS (SELECT 1 FROM `moderacao_eventos` WHERE `ator_papel` = 'admin' AND `id_ator` IS NOT NULL) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00019: moderacao tem acoes de admin gravadas com id de usuarios - converter manualmente';
    END IF;
  END IF;
  IF EXISTS (SELECT 1 FROM `moderacao_eventos` WHERE `ator_papel` IN ('usuario','academia') AND `id_ator` IS NULL)
     OR EXISTS (SELECT 1 FROM `moderacao_eventos` WHERE `ator_papel` = 'sistema' AND `id_ator` IS NOT NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00019: moderacao_eventos com ator incoerente com ator_papel';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE()
              AND table_name = 'usuarios' AND column_name = 'bloqueado') THEN
    -- SQL dinâmico: a coluna pode não existir numa reexecução.
    SET @q = 'SELECT COUNT(*) INTO @bloqueados FROM `usuarios` WHERE `bloqueado` = 1';
    PREPARE stmt FROM @q; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    IF @bloqueados > 0 AND NOT EXISTS (SELECT 1 FROM `administradores`) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00019: usuarios bloqueados sem nenhum administrador para registrar a punicao';
    END IF;
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00019_verificar`();
DROP PROCEDURE `_goose_00019_verificar`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00019_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00019_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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
DROP PROCEDURE IF EXISTS `_goose_00019_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Remove coluna/índice/FK/CHECK somente se existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00019_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
BEGIN
  DECLARE v_existe INT DEFAULT 0;
  IF p_tipo = 'coluna' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.columns
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND column_name = p_nome;
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP COLUMN `', p_nome, '`');
  ELSEIF p_tipo = 'indice' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.statistics
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND index_name = p_nome;
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP INDEX `', p_nome, '`');
  ELSEIF p_tipo = 'fk' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.table_constraints
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome AND constraint_type = 'FOREIGN KEY';
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP FOREIGN KEY `', p_nome, '`');
  ELSE
    SELECT COUNT(*) INTO v_existe FROM information_schema.table_constraints
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome AND constraint_type = 'CHECK';
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP CHECK `', p_nome, '`');
  END IF;
  IF v_existe > 0 THEN
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

-- ── (a) usuarios.perfil sem 'admin' ──
ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia','instrutor') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta (admins ficam em administradores)';

-- ── (a) punicoes: aplicador usuário (academia) × admin; revogação só por admin ──
ALTER TABLE `punicoes`
  MODIFY COLUMN `aplicada_por` BIGINT UNSIGNED NULL COMMENT 'Conta da academia que aplicou (usuarios.id); NULL quando aplicada por admin';
CALL `_goose_00019_add`('punicoes', 'coluna', 'aplicada_por_admin', "COLUMN `aplicada_por_admin` BIGINT UNSIGNED NULL COMMENT 'Administrador que aplicou (administradores.id); NULL quando aplicada pela academia' AFTER `aplicada_por`");
CALL `_goose_00019_add`('punicoes', 'indice', 'idx_punicoes_aplicada_por_admin', "KEY `idx_punicoes_aplicada_por_admin` (`aplicada_por_admin`)");
CALL `_goose_00019_add`('punicoes', 'fk', 'fk_punicoes_aplicada_por_admin', "CONSTRAINT `fk_punicoes_aplicada_por_admin` FOREIGN KEY (`aplicada_por_admin`) REFERENCES `administradores` (`id`)");
CALL `_goose_00019_add`('punicoes', 'check', 'chk_punicoes_aplicador', "CONSTRAINT `chk_punicoes_aplicador` CHECK ((`aplicada_por` IS NULL) <> (`aplicada_por_admin` IS NULL) AND (`escopo` = 'academia' OR `aplicada_por_admin` IS NOT NULL))");
CALL `_goose_00019_drop`('punicoes', 'fk', 'fk_punicoes_revogada_por');
ALTER TABLE `punicoes`
  MODIFY COLUMN `revogada_por` BIGINT UNSIGNED NULL COMMENT 'Administrador que revogou (administradores.id)';
CALL `_goose_00019_add`('punicoes', 'fk', 'fk_punicoes_revogada_por_admin', "CONSTRAINT `fk_punicoes_revogada_por_admin` FOREIGN KEY (`revogada_por`) REFERENCES `administradores` (`id`)");

-- ── (a) recursos: julgados só por admin ──
CALL `_goose_00019_drop`('recursos', 'fk', 'fk_recursos_julgado_por');
ALTER TABLE `recursos`
  MODIFY COLUMN `julgado_por` BIGINT UNSIGNED NULL COMMENT 'Administrador que julgou (administradores.id)';
CALL `_goose_00019_add`('recursos', 'fk', 'fk_recursos_julgado_por_admin', "CONSTRAINT `fk_recursos_julgado_por_admin` FOREIGN KEY (`julgado_por`) REFERENCES `administradores` (`id`)");

-- ── (a) denuncias: responsável academia × admin ──
ALTER TABLE `denuncias`
  MODIFY COLUMN `id_responsavel` BIGINT UNSIGNED NULL COMMENT 'Conta da academia (usuarios.id) que assumiu a análise; NULL se ninguém ou se foi um admin';
CALL `_goose_00019_add`('denuncias', 'coluna', 'id_responsavel_admin', "COLUMN `id_responsavel_admin` BIGINT UNSIGNED NULL COMMENT 'Administrador que assumiu a análise (administradores.id)' AFTER `id_responsavel`");
CALL `_goose_00019_add`('denuncias', 'indice', 'idx_denuncias_responsavel_admin', "KEY `idx_denuncias_responsavel_admin` (`id_responsavel_admin`)");
CALL `_goose_00019_add`('denuncias', 'fk', 'fk_denuncias_responsavel_admin', "CONSTRAINT `fk_denuncias_responsavel_admin` FOREIGN KEY (`id_responsavel_admin`) REFERENCES `administradores` (`id`)");
CALL `_goose_00019_add`('denuncias', 'check', 'chk_denuncias_responsavel', "CONSTRAINT `chk_denuncias_responsavel` CHECK (`id_responsavel` IS NULL OR `id_responsavel_admin` IS NULL)");

-- ── (a) moderacao_eventos: ator admin ──
ALTER TABLE `moderacao_eventos`
  MODIFY COLUMN `id_ator` BIGINT UNSIGNED NULL COMMENT 'Usuário que agiu (papel usuario/academia; usuarios.id). NULL para admin (ver id_admin) e sistema';
CALL `_goose_00019_add`('moderacao_eventos', 'coluna', 'id_admin', "COLUMN `id_admin` BIGINT UNSIGNED NULL COMMENT 'Administrador que agiu (papel admin; administradores.id)' AFTER `id_ator`");
CALL `_goose_00019_add`('moderacao_eventos', 'indice', 'idx_eventos_admin', "KEY `idx_eventos_admin` (`id_admin`, `criado_em`)");
CALL `_goose_00019_add`('moderacao_eventos', 'fk', 'fk_eventos_admin', "CONSTRAINT `fk_eventos_admin` FOREIGN KEY (`id_admin`) REFERENCES `administradores` (`id`)");
CALL `_goose_00019_add`('moderacao_eventos', 'check', 'chk_eventos_ator', "CONSTRAINT `chk_eventos_ator` CHECK ((`ator_papel` = 'admin') = (`id_admin` IS NOT NULL) AND (`ator_papel` IN ('usuario','academia')) = (`id_ator` IS NOT NULL))");

-- ── (b) bloqueio unificado: bloqueado = 1 vira banimento; depois as colunas saem ──
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00019_bloqueio`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00019_bloqueio`()
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE()
              AND table_name = 'usuarios' AND column_name = 'bloqueado') THEN
    -- Autor da punição: quem bloqueou (admin_auditoria) ou, sem registro, o superadmin mais antigo.
    SET @q = CONCAT(
      'INSERT INTO `punicoes` (`id_usuario`, `tipo`, `escopo`, `inicio`, `aplicada_por`, `aplicada_por_admin`, `justificativa`) ',
      'SELECT u.`id`, ''banimento'', ''global'', COALESCE(u.`bloqueado_em`, CURRENT_TIMESTAMP(3)), NULL, ',
      '  COALESCE((SELECT a.`id_admin` FROM `admin_auditoria` a WHERE a.`acao` = ''usuario_bloquear'' AND a.`alvo_id` = u.`id` ORDER BY a.`id` DESC LIMIT 1), ',
      '           (SELECT MIN(ad.`id`) FROM `administradores` ad WHERE ad.`papel` = ''superadmin''), ',
      '           (SELECT MIN(ad.`id`) FROM `administradores` ad)), ',
      '  COALESCE(NULLIF(TRIM(u.`bloqueado_motivo`), ''''), ''Bloqueio pelo painel admin (migrado na 00019)'') ',
      '  FROM `usuarios` u WHERE u.`bloqueado` = 1 AND u.`banido_em` IS NULL');
    PREPARE stmt FROM @q; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    SET @q = CONCAT(
      'UPDATE `usuarios` SET `banido_em` = COALESCE(`bloqueado_em`, CURRENT_TIMESTAMP(3)), ',
      '  `motivo_bloqueio` = LEFT(COALESCE(NULLIF(TRIM(`bloqueado_motivo`), ''''), ''Bloqueio pelo painel admin''), 255) ',
      ' WHERE `bloqueado` = 1 AND `banido_em` IS NULL');
    PREPARE stmt FROM @q; EXECUTE stmt; DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00019_bloqueio`();
DROP PROCEDURE `_goose_00019_bloqueio`;
CALL `_goose_00019_drop`('usuarios', 'coluna', 'bloqueado_em');
CALL `_goose_00019_drop`('usuarios', 'coluna', 'bloqueado_motivo');
CALL `_goose_00019_drop`('usuarios', 'coluna', 'bloqueado');

-- ── (c) Q6: estorno da taxa da plataforma ──
ALTER TABLE `plataforma_receitas`
  MODIFY COLUMN `origem` ENUM('taxa_inscricao','tarifa_saque','estorno_taxa') NOT NULL COMMENT 'De onde veio a receita; estorno_taxa = devolução da taxa de inscrição (anulação com devolução)',
  MODIFY COLUMN `valor_centavos` BIGINT NOT NULL COMMENT 'Valor em centavos: > 0 nas receitas, < 0 em estorno_taxa (a soma é a receita líquida)';
CALL `_goose_00019_drop`('plataforma_receitas', 'check', 'chk_receita_valor');
CALL `_goose_00019_add`('plataforma_receitas', 'check', 'chk_receita_sinal', "CONSTRAINT `chk_receita_sinal` CHECK ((`origem` = 'estorno_taxa' AND `valor_centavos` < 0) OR (`origem` <> 'estorno_taxa' AND `valor_centavos` > 0))");
CALL `_goose_00019_add`('plataforma_receitas', 'check', 'chk_receita_estorno_desafio', "CONSTRAINT `chk_receita_estorno_desafio` CHECK (`origem` <> 'estorno_taxa' OR (`id_desafio` IS NOT NULL AND `id_transacao` IS NULL))");

-- ── (d) índices dos jobs ──
-- fim-suspensao / expiração de punições: vigentes por tipo com fim vencido.
CALL `_goose_00019_add`('punicoes', 'indice', 'idx_punicoes_expiracao', "KEY `idx_punicoes_expiracao` (`tipo`, `revogada_em`, `fim`)");
-- fim-suspensao: contas com suspenso_ate vencido.
CALL `_goose_00019_add`('usuarios', 'indice', 'idx_usuarios_suspenso_ate', "KEY `idx_usuarios_suspenso_ate` (`suspenso_ate`)");

DROP PROCEDURE `_goose_00019_add`;
DROP PROCEDURE `_goose_00019_drop`;

-- +goose Down
-- Só para banco descartável. Recusa (SIGNAL) se houver dado que o schema antigo não representa: ações de
-- admin na moderação (ids de administradores não são usuarios) ou estorno de taxa. Banimentos ficam em
-- `punicoes`/`banido_em` (as colunas bloqueado* voltam zeradas).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00019_verificar_down`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00019_verificar_down`()
BEGIN
  IF EXISTS (SELECT 1 FROM `punicoes` WHERE `aplicada_por_admin` IS NOT NULL OR `revogada_por` IS NOT NULL)
     OR EXISTS (SELECT 1 FROM `recursos` WHERE `julgado_por` IS NOT NULL)
     OR EXISTS (SELECT 1 FROM `denuncias` WHERE `id_responsavel_admin` IS NOT NULL)
     OR EXISTS (SELECT 1 FROM `moderacao_eventos` WHERE `id_admin` IS NOT NULL)
     OR EXISTS (SELECT 1 FROM `plataforma_receitas` WHERE `origem` = 'estorno_taxa') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '00019 down: ha acoes de admin na moderacao ou estorno_taxa - nao reversivel';
  END IF;
END;
-- +goose StatementEnd
CALL `_goose_00019_verificar_down`();
DROP PROCEDURE `_goose_00019_verificar_down`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00019_add`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00019_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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
DROP PROCEDURE IF EXISTS `_goose_00019_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00019_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
BEGIN
  DECLARE v_existe INT DEFAULT 0;
  IF p_tipo = 'coluna' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.columns
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND column_name = p_nome;
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP COLUMN `', p_nome, '`');
  ELSEIF p_tipo = 'indice' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.statistics
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND index_name = p_nome;
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP INDEX `', p_nome, '`');
  ELSEIF p_tipo = 'fk' THEN
    SELECT COUNT(*) INTO v_existe FROM information_schema.table_constraints
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome AND constraint_type = 'FOREIGN KEY';
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP FOREIGN KEY `', p_nome, '`');
  ELSE
    SELECT COUNT(*) INTO v_existe FROM information_schema.table_constraints
     WHERE table_schema = DATABASE() AND table_name = p_tabela AND constraint_name = p_nome AND constraint_type = 'CHECK';
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP CHECK `', p_nome, '`');
  END IF;
  IF v_existe > 0 THEN
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

CALL `_goose_00019_drop`('usuarios', 'indice', 'idx_usuarios_suspenso_ate');
CALL `_goose_00019_drop`('punicoes', 'indice', 'idx_punicoes_expiracao');

CALL `_goose_00019_drop`('plataforma_receitas', 'check', 'chk_receita_estorno_desafio');
CALL `_goose_00019_drop`('plataforma_receitas', 'check', 'chk_receita_sinal');
CALL `_goose_00019_add`('plataforma_receitas', 'check', 'chk_receita_valor', "CONSTRAINT `chk_receita_valor` CHECK (`valor_centavos` > 0)");
ALTER TABLE `plataforma_receitas`
  MODIFY COLUMN `origem` ENUM('taxa_inscricao','tarifa_saque') NOT NULL COMMENT 'De onde veio a receita',
  MODIFY COLUMN `valor_centavos` BIGINT NOT NULL COMMENT 'Valor recebido pela plataforma, em centavos (sempre > 0)';

CALL `_goose_00019_add`('usuarios', 'coluna', 'bloqueado', "COLUMN `bloqueado` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = conta bloqueada por um administrador (login e tokens recusados)'");
CALL `_goose_00019_add`('usuarios', 'coluna', 'bloqueado_motivo', "COLUMN `bloqueado_motivo` VARCHAR(500) NULL COMMENT 'Motivo do bloqueio vigente (histórico em admin_auditoria)' AFTER `bloqueado`");
CALL `_goose_00019_add`('usuarios', 'coluna', 'bloqueado_em', "COLUMN `bloqueado_em` DATETIME(3) NULL COMMENT 'Quando o bloqueio vigente foi aplicado' AFTER `bloqueado_motivo`");

CALL `_goose_00019_drop`('moderacao_eventos', 'check', 'chk_eventos_ator');
CALL `_goose_00019_drop`('moderacao_eventos', 'fk', 'fk_eventos_admin');
CALL `_goose_00019_drop`('moderacao_eventos', 'indice', 'idx_eventos_admin');
CALL `_goose_00019_drop`('moderacao_eventos', 'coluna', 'id_admin');
ALTER TABLE `moderacao_eventos`
  MODIFY COLUMN `id_ator` BIGINT UNSIGNED NULL COMMENT 'Quem agiu (NULL = sistema/job)';

CALL `_goose_00019_drop`('denuncias', 'check', 'chk_denuncias_responsavel');
CALL `_goose_00019_drop`('denuncias', 'fk', 'fk_denuncias_responsavel_admin');
CALL `_goose_00019_drop`('denuncias', 'indice', 'idx_denuncias_responsavel_admin');
CALL `_goose_00019_drop`('denuncias', 'coluna', 'id_responsavel_admin');
ALTER TABLE `denuncias`
  MODIFY COLUMN `id_responsavel` BIGINT UNSIGNED NULL COMMENT 'Usuário (admin ou conta da academia) que assumiu a análise';

CALL `_goose_00019_drop`('recursos', 'fk', 'fk_recursos_julgado_por_admin');
ALTER TABLE `recursos`
  MODIFY COLUMN `julgado_por` BIGINT UNSIGNED NULL COMMENT 'Admin que julgou';
CALL `_goose_00019_add`('recursos', 'fk', 'fk_recursos_julgado_por', "CONSTRAINT `fk_recursos_julgado_por` FOREIGN KEY (`julgado_por`) REFERENCES `usuarios` (`id`)");

CALL `_goose_00019_drop`('punicoes', 'fk', 'fk_punicoes_revogada_por_admin');
ALTER TABLE `punicoes`
  MODIFY COLUMN `revogada_por` BIGINT UNSIGNED NULL COMMENT 'Admin que revogou';
CALL `_goose_00019_add`('punicoes', 'fk', 'fk_punicoes_revogada_por', "CONSTRAINT `fk_punicoes_revogada_por` FOREIGN KEY (`revogada_por`) REFERENCES `usuarios` (`id`)");
CALL `_goose_00019_drop`('punicoes', 'check', 'chk_punicoes_aplicador');
CALL `_goose_00019_drop`('punicoes', 'fk', 'fk_punicoes_aplicada_por_admin');
CALL `_goose_00019_drop`('punicoes', 'indice', 'idx_punicoes_aplicada_por_admin');
CALL `_goose_00019_drop`('punicoes', 'coluna', 'aplicada_por_admin');
-- NULL -> NOT NULL numa coluna com FK exige soltar a FK antes (erro 1832).
CALL `_goose_00019_drop`('punicoes', 'fk', 'fk_punicoes_aplicada_por');
ALTER TABLE `punicoes`
  MODIFY COLUMN `aplicada_por` BIGINT UNSIGNED NOT NULL COMMENT 'Admin ou conta da academia que aplicou';
CALL `_goose_00019_add`('punicoes', 'fk', 'fk_punicoes_aplicada_por', "CONSTRAINT `fk_punicoes_aplicada_por` FOREIGN KEY (`aplicada_por`) REFERENCES `usuarios` (`id`)");

ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia','instrutor','admin') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta (admin só por cmd/admin, nunca por cadastro)';

DROP PROCEDURE `_goose_00019_add`;
DROP PROCEDURE `_goose_00019_drop`;
