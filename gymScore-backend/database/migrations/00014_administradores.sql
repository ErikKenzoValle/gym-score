-- +goose Up
-- Painel administrativo: administradores ficam em tabela própria (NÃO são um perfil de `usuarios`),
-- com autenticação separada (segredo, claim aud e cookie próprios). Ver PLANO.md, seção Arquitetura.
-- O 1º superadmin é criado pela CLI (`admin criar-superadmin`); os demais, pela tela, por um superadmin.
--
-- Idempotente: CREATE TABLE IF NOT EXISTS (padrão da 00012 para tabelas novas).

-- Limpeza do RDS de testes: a antiga 00014 (hoje 00015_moderacao_aprovacao) falhou no meio lá e deixou
-- a procedure temporária `_goose_00014_add` para trás (ver analise/moderacao-schema-delta.md, grupo B).
-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00014_add`;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS `administradores` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `nome` VARCHAR(100) NOT NULL COMMENT 'Nome de exibição do administrador',
  `email` VARCHAR(150) NOT NULL COMMENT 'Login do administrador (único)',
  `senha_hash` VARCHAR(255) NOT NULL COMMENT 'Hash bcrypt da senha; nunca a senha em claro',
  `papel` ENUM('superadmin','admin') NOT NULL DEFAULT 'admin' COMMENT 'superadmin: dinheiro (reembolso/marcar pago) e gestão de admins; admin: demais ações',
  `ativo` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '0 = desativado (não autentica; tokens recusados)',
  `token_versao` INT NOT NULL DEFAULT 0 COMMENT 'Claim tv do JWT admin; incrementar revoga todos os tokens emitidos',
  `ultimo_login_em` DATETIME(3) NULL COMMENT 'Último login bem-sucedido',
  `criado_por` BIGINT UNSIGNED NULL COMMENT 'Superadmin que criou este admin; NULL = criado pela CLI',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando a conta foi criada',
  `atualizado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'Última alteração',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_administradores_email` (`email`),
  KEY `idx_administradores_papel_ativo` (`papel`, `ativo`),
  KEY `fk_administradores_criado_por` (`criado_por`),
  CONSTRAINT `fk_administradores_criado_por` FOREIGN KEY (`criado_por`) REFERENCES `administradores` (`id`),
  CONSTRAINT `chk_administradores_token_versao` CHECK (`token_versao` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Contas do painel administrativo (separadas de usuarios)';

-- Trilha de auditoria append-only: a aplicação só faz INSERT e SELECT (o AuditoriaRepository não
-- expõe update/delete) e grava a linha NA MESMA TRANSAÇÃO da ação auditada. Em produção, recomenda-se
-- também revogar UPDATE/DELETE desta tabela do usuário da aplicação. Trigger de bloqueio não foi usado
-- de propósito: CREATE TRIGGER no RDS com binlog exige SUPER ou log_bin_trust_function_creators=1.
CREATE TABLE IF NOT EXISTS `admin_auditoria` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_admin` BIGINT UNSIGNED NOT NULL COMMENT 'Administrador que executou a ação',
  `acao` VARCHAR(60) NOT NULL COMMENT 'Identificador da ação (ex.: saque.reembolsar, usuario.bloquear)',
  `alvo_tipo` VARCHAR(30) NOT NULL COMMENT 'Tipo do alvo (transacao, usuario, desafio, academia, instrutor, administrador)',
  `alvo_id` BIGINT UNSIGNED NULL COMMENT 'ID do alvo (NULL quando a ação não tem alvo único)',
  `motivo` VARCHAR(500) NOT NULL COMMENT 'Justificativa obrigatória informada pelo admin',
  `detalhes` JSON NULL COMMENT 'Dados extras (antes/depois, resposta do Asaas, etc.)',
  `ip` VARCHAR(45) NOT NULL DEFAULT '' COMMENT 'IP de origem (IPv4 ou IPv6)',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando a ação foi executada',
  PRIMARY KEY (`id`),
  KEY `idx_auditoria_alvo` (`alvo_tipo`, `alvo_id`),
  KEY `idx_auditoria_admin_data` (`id_admin`, `criado_em`),
  KEY `idx_auditoria_acao_data` (`acao`, `criado_em`),
  KEY `idx_auditoria_data` (`criado_em`),
  CONSTRAINT `fk_auditoria_admin` FOREIGN KEY (`id_admin`) REFERENCES `administradores` (`id`),
  CONSTRAINT `chk_auditoria_motivo` CHECK (CHAR_LENGTH(TRIM(`motivo`)) > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Auditoria (append-only) de toda ação de escrita do painel admin';

-- +goose Down
-- Remove o painel admin. As FKs de academias/instrutores.aprovado_por (00015) já foram removidas pelo
-- Down da 00015, que roda antes deste.
DROP TABLE IF EXISTS `admin_auditoria`;
DROP TABLE IF EXISTS `administradores`;
