-- +goose Up
-- Perfis RBAC (atleta/academia/instrutor), pódio e avaliações de desafio, trazidos da branch de
-- reestruturação. Este RDS de dev já teve essas colunas/tabelas criadas por um `AutoMigrate` legado
-- do GORM (modelos Academia/Instrutor/PodioDesafio/AvaliacaoDesafio) ANTES de migrations SQL existirem
-- neste projeto — mesma causa do incidente de `transacoes.tipo` na 00003. Por isso todo `ADD COLUMN` e
-- `CREATE TABLE` abaixo é escrito para funcionar tanto num banco novo (só com 00001-00009 aplicadas)
-- quanto neste RDS (onde já existem). Ver `database/README.md`.

-- Nulidade de cpf/data_nascimento/genero: contas de perfil "academia" (pessoa jurídica) não coletam
-- esses dados. `MODIFY COLUMN` para o mesmo estado não falha, então é seguro repetir em qualquer banco.
ALTER TABLE `usuarios`
  MODIFY COLUMN `cpf` VARCHAR(14) NULL COMMENT 'Nulo para contas de perfil "academia" (pessoa jurídica)',
  MODIFY COLUMN `data_nascimento` DATE NULL COMMENT 'Nulo para contas de perfil "academia"',
  MODIFY COLUMN `genero` ENUM('M','F','O') NULL COMMENT 'Nulo para contas de perfil "academia"';

-- Procedures temporárias: o MySQL não tem `ADD COLUMN IF NOT EXISTS` / `ADD INDEX IF NOT EXISTS`
-- (diferente do MariaDB), então a checagem de existência é feita via information_schema + SQL dinâmico.
-- São removidas no fim deste Up.
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00010_add_coluna`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00010_add_coluna`(IN p_tabela VARCHAR(64), IN p_coluna VARCHAR(64), IN p_definicao TEXT)
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = p_tabela AND column_name = p_coluna
  ) THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` ADD COLUMN ', p_definicao);
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00010_add_indice`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00010_add_indice`(IN p_tabela VARCHAR(64), IN p_indice VARCHAR(64), IN p_definicao TEXT)
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = p_tabela AND index_name = p_indice
  ) THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` ADD ', p_definicao);
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

-- `usuarios`: perfil + gamificação (username/elo/pontos/ultima_vez)
CALL `_goose_00010_add_coluna`('usuarios', 'perfil', "`perfil` ENUM('atleta','academia','instrutor') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta' AFTER `atualizado_em`");
CALL `_goose_00010_add_coluna`('usuarios', 'username', "`username` VARCHAR(50) NULL COMMENT 'Apelido público único, opcional' AFTER `perfil`");
CALL `_goose_00010_add_coluna`('usuarios', 'elo', "`elo` VARCHAR(20) NOT NULL DEFAULT 'Iniciante' COMMENT 'Faixa de gamificação' AFTER `username`");
CALL `_goose_00010_add_coluna`('usuarios', 'pontos', "`pontos` INT NOT NULL DEFAULT 0 COMMENT 'Pontuação de gamificação' AFTER `elo`");
CALL `_goose_00010_add_coluna`('usuarios', 'ultima_vez', "`ultima_vez` DATETIME(3) NULL COMMENT 'Último acesso registrado pelo app' AFTER `pontos`");
CALL `_goose_00010_add_indice`('usuarios', 'idx_usuarios_username', "UNIQUE KEY `idx_usuarios_username` (`username`)");

-- Normaliza nulidade/tipo/default mesmo quando a coluna já existia (legado AutoMigrate deixava
-- `perfil`/`elo`/`pontos` nullable, sem a garantia de NOT NULL que o schema atual exige).
UPDATE `usuarios` SET `perfil` = 'atleta' WHERE `perfil` IS NULL;
UPDATE `usuarios` SET `elo` = 'Iniciante' WHERE `elo` IS NULL;
UPDATE `usuarios` SET `pontos` = 0 WHERE `pontos` IS NULL;
ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia','instrutor') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta',
  MODIFY COLUMN `elo` VARCHAR(20) NOT NULL DEFAULT 'Iniciante' COMMENT 'Faixa de gamificação',
  MODIFY COLUMN `pontos` INT NOT NULL DEFAULT 0 COMMENT 'Pontuação de gamificação';

-- `desafio_participantes.presente`: check-in de presença feito pelo instrutor/academia (distinto do
-- `desafio_checkins` da 00005, que é o check-in com foto/geolocalização feito pelo próprio atleta).
CALL `_goose_00010_add_coluna`('desafio_participantes', 'presente', "`presente` TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'Compareceu presencialmente (marcado pelo instrutor/academia)' AFTER `criado_em`");
UPDATE `desafio_participantes` SET `presente` = 0 WHERE `presente` IS NULL;
ALTER TABLE `desafio_participantes`
  MODIFY COLUMN `presente` TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'Compareceu presencialmente (marcado pelo instrutor/academia)';

DROP PROCEDURE `_goose_00010_add_coluna`;
DROP PROCEDURE `_goose_00010_add_indice`;

-- Tabelas novas: `CREATE TABLE IF NOT EXISTS` já é idempotente (mesmo padrão da 00001_baseline) —
-- neste RDS elas já existem (AutoMigrate legado) e não são alteradas.
CREATE TABLE IF NOT EXISTS `academias` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_usuario` BIGINT UNSIGNED NOT NULL COMMENT 'Usuário de acesso (email/senha) desta conta de academia',
  `razao_social` VARCHAR(150) NOT NULL COMMENT 'Denominação legal perante a Receita Federal',
  `nome_fantasia` VARCHAR(150) NOT NULL COMMENT 'Nome comercial exibido no app',
  `cnpj` VARCHAR(18) NOT NULL COMMENT 'Formatado: 00.000.000/0000-00',
  `endereco` VARCHAR(255) DEFAULT NULL,
  `telefone` VARCHAR(20) DEFAULT NULL,
  `criado_em` DATETIME(3) DEFAULT NULL,
  `atualizado_em` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_academias_id_usuario` (`id_usuario`),
  UNIQUE KEY `idx_academias_cnpj` (`cnpj`),
  CONSTRAINT `fk_academias_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Dados de pessoa jurídica das contas de perfil "academia"';

CREATE TABLE IF NOT EXISTS `instrutores` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_usuario` BIGINT UNSIGNED NOT NULL COMMENT 'Usuário de acesso (email/senha) deste instrutor',
  `cref` VARCHAR(20) NOT NULL COMMENT 'Registro profissional (ex: 001234-G/SP)',
  `especialidade` VARCHAR(100) DEFAULT NULL,
  `id_academia` BIGINT UNSIGNED DEFAULT NULL COMMENT 'Vínculo opcional com academia; NULL = autônomo',
  `criado_em` DATETIME(3) DEFAULT NULL,
  `atualizado_em` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_instrutores_id_usuario` (`id_usuario`),
  KEY `fk_instrutores_academia` (`id_academia`),
  CONSTRAINT `fk_instrutores_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_instrutores_academia` FOREIGN KEY (`id_academia`) REFERENCES `academias` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Profissionais de educação física, autônomos ou vinculados a uma academia';

CREATE TABLE IF NOT EXISTS `podio_desafios` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_desafio` BIGINT UNSIGNED NOT NULL,
  `posicao` INT NOT NULL COMMENT '1 = primeiro lugar',
  `id_participante` BIGINT UNSIGNED NOT NULL,
  `marca` VARCHAR(50) DEFAULT NULL COMMENT 'Registro da performance do participante (tempo, peso, repetições, etc.)',
  `criado_em` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_podio_posicao` (`id_desafio`, `posicao`),
  KEY `fk_podio_desafios_participante` (`id_participante`),
  CONSTRAINT `fk_podio_desafios_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_podio_desafios_participante` FOREIGN KEY (`id_participante`) REFERENCES `usuarios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Pódio (top 5) de um desafio encerrado, lançado pela academia';

CREATE TABLE IF NOT EXISTS `avaliacoes_desafios` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_desafio` BIGINT UNSIGNED NOT NULL,
  `id_usuario` BIGINT UNSIGNED NOT NULL,
  `nota` INT NOT NULL COMMENT '1 a 5',
  `criado_em` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_avaliacao` (`id_desafio`, `id_usuario`),
  KEY `fk_avaliacoes_desafios_usuario` (`id_usuario`),
  CONSTRAINT `chk_avaliacoes_desafios_nota` CHECK (`nota` BETWEEN 1 AND 5),
  CONSTRAINT `fk_avaliacoes_desafios_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_avaliacoes_desafios_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Avaliação (1 a 5) de um usuário sobre um desafio em que participou';

-- +goose Down
-- Reversão com perda de informação se já existir conta de perfil "academia" (sem cpf/data_nascimento/
-- genero): o MODIFY final para NOT NULL falha nesse caso — é esperado, mesmo padrão documentado para
-- os Downs da 00002/00003/00004 no database/README.md.
DROP TABLE IF EXISTS `avaliacoes_desafios`;
DROP TABLE IF EXISTS `podio_desafios`;
DROP TABLE IF EXISTS `instrutores`;
DROP TABLE IF EXISTS `academias`;

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00010_drop_coluna`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00010_drop_coluna`(IN p_tabela VARCHAR(64), IN p_coluna VARCHAR(64))
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = p_tabela AND column_name = p_coluna
  ) THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP COLUMN `', p_coluna, '`');
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00010_drop_indice`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00010_drop_indice`(IN p_tabela VARCHAR(64), IN p_indice VARCHAR(64))
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = p_tabela AND index_name = p_indice
  ) THEN
    SET @ddl = CONCAT('ALTER TABLE `', p_tabela, '` DROP INDEX `', p_indice, '`');
    PREPARE stmt FROM @ddl;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END;
-- +goose StatementEnd

CALL `_goose_00010_drop_coluna`('desafio_participantes', 'presente');
CALL `_goose_00010_drop_indice`('usuarios', 'idx_usuarios_username');
CALL `_goose_00010_drop_coluna`('usuarios', 'ultima_vez');
CALL `_goose_00010_drop_coluna`('usuarios', 'pontos');
CALL `_goose_00010_drop_coluna`('usuarios', 'elo');
CALL `_goose_00010_drop_coluna`('usuarios', 'username');
CALL `_goose_00010_drop_coluna`('usuarios', 'perfil');

DROP PROCEDURE `_goose_00010_drop_coluna`;
DROP PROCEDURE `_goose_00010_drop_indice`;

ALTER TABLE `usuarios`
  MODIFY COLUMN `cpf` VARCHAR(14) NOT NULL,
  MODIFY COLUMN `data_nascimento` DATE NOT NULL,
  MODIFY COLUMN `genero` ENUM('M','F','O') NOT NULL;
