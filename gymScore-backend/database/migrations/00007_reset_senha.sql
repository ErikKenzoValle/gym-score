-- +goose Up
CREATE TABLE `password_reset_tokens` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'Identificador',
  `id_usuario` bigint unsigned NOT NULL COMMENT 'Usuário dono do token',
  `token_hash` char(64) NOT NULL COMMENT 'SHA-256 (hex) do token enviado por e-mail',
  `expira_em` datetime(3) NOT NULL COMMENT 'Expiração (30 min após criação)',
  `usado_em` datetime(3) NULL COMMENT 'NULL = ainda não utilizado (uso único)',
  `criado_em` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Data/hora de criação',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_reset_token_hash` (`token_hash`),
  KEY `idx_reset_usuario` (`id_usuario`),
  CONSTRAINT `fk_reset_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Tokens de recuperação de senha (apenas o hash é armazenado)';

-- +goose Down
DROP TABLE IF EXISTS `password_reset_tokens`;
