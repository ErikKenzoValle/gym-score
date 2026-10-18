-- +goose Up
-- Baseline: reproduz exatamente o schema gerado pelo GORM AutoMigrate (models anteriores à Fase A).
-- Idempotente: em bancos que já possuem as tabelas (criadas pelo AutoMigrate) nada é alterado.

CREATE TABLE IF NOT EXISTS `usuarios` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `nome` varchar(100) NOT NULL,
  `sobrenome` varchar(100) NOT NULL,
  `cpf` varchar(14) NOT NULL,
  `email` varchar(150) NOT NULL,
  `senha` varchar(255) NOT NULL,
  `data_nascimento` date NOT NULL,
  `genero` enum('M','F','O') NOT NULL,
  `saldo` decimal(10,2) DEFAULT '0.00',
  `criado_em` datetime(3) DEFAULT NULL,
  `atualizado_em` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_usuarios_cpf` (`cpf`),
  UNIQUE KEY `idx_usuarios_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `desafios` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `titulo` varchar(200) NOT NULL,
  `descricao` text,
  `valor` decimal(10,2) NOT NULL,
  `local` varchar(200) DEFAULT NULL,
  `latitude` decimal(10,8) DEFAULT NULL,
  `longitude` decimal(11,8) DEFAULT NULL,
  `status` enum('pendente','aberto','em_andamento','encerrado') DEFAULT 'aberto',
  `estado` tinyint DEFAULT '1',
  `vagas` bigint DEFAULT '2',
  `data_encerramento` datetime(3) DEFAULT NULL,
  `id_criador` bigint unsigned NOT NULL,
  `id_desafiado` bigint unsigned DEFAULT NULL,
  `id_vencedor` bigint unsigned DEFAULT NULL,
  `id_perdedor` bigint unsigned DEFAULT NULL,
  `criado_em` datetime(3) DEFAULT NULL,
  `atualizado_em` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_desafios_criador` (`id_criador`),
  KEY `fk_desafios_desafiado` (`id_desafiado`),
  KEY `fk_desafios_vencedor` (`id_vencedor`),
  CONSTRAINT `fk_desafios_criador` FOREIGN KEY (`id_criador`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_desafios_desafiado` FOREIGN KEY (`id_desafiado`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_desafios_vencedor` FOREIGN KEY (`id_vencedor`) REFERENCES `usuarios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `desafio_participantes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_desafio` bigint unsigned NOT NULL,
  `id_usuario` bigint unsigned NOT NULL,
  `criado_em` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_desafio_usuario` (`id_desafio`,`id_usuario`),
  KEY `fk_desafio_participantes_usuario` (`id_usuario`),
  CONSTRAINT `fk_desafio_participantes_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_desafios_participantes` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `amizades` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_usuario` bigint unsigned NOT NULL,
  `id_amigo` bigint unsigned NOT NULL,
  `status` enum('pendente','aceita','recusada') DEFAULT 'pendente',
  `criado_em` datetime(3) DEFAULT NULL,
  `atualizado_em` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_amizades_id_usuario` (`id_usuario`),
  KEY `idx_amizades_id_amigo` (`id_amigo`),
  CONSTRAINT `fk_amizades_amigo` FOREIGN KEY (`id_amigo`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_amizades_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `transacoes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_usuario` bigint unsigned NOT NULL,
  `asaas_payment_id` varchar(100) NOT NULL,
  `valor` decimal(10,2) NOT NULL,
  `status` enum('pending','received','refunded') DEFAULT 'pending',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_transacoes_asaas_payment_id` (`asaas_payment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `treinos` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_usuario` bigint unsigned NOT NULL,
  `nome` varchar(120) NOT NULL,
  `descricao` varchar(300) DEFAULT NULL,
  `codigo` varchar(12) NOT NULL,
  `criado_em` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_treinos_codigo` (`codigo`),
  KEY `idx_treinos_id_usuario` (`id_usuario`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `treino_exercicios` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_treino` bigint unsigned NOT NULL,
  `nome` varchar(120) NOT NULL,
  `grupo_muscular` varchar(40) NOT NULL,
  `ordem` bigint NOT NULL DEFAULT '0',
  `series` bigint DEFAULT '0',
  `repeticoes` bigint DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_treino_exercicios_id_treino` (`id_treino`),
  CONSTRAINT `fk_treinos_exercicios` FOREIGN KEY (`id_treino`) REFERENCES `treinos` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `treino_conclusoes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_usuario` bigint unsigned NOT NULL,
  `id_treino` bigint unsigned DEFAULT NULL,
  `grupo_muscular` varchar(40) NOT NULL,
  `concluido_em` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_treino_conclusoes_id_usuario` (`id_usuario`),
  KEY `idx_treino_conclusoes_id_treino` (`id_treino`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
-- ATENÇÃO: remove TODAS as tabelas do sistema (destrutivo).
DROP TABLE IF EXISTS `treino_conclusoes`;
DROP TABLE IF EXISTS `treino_exercicios`;
DROP TABLE IF EXISTS `treinos`;
DROP TABLE IF EXISTS `transacoes`;
DROP TABLE IF EXISTS `amizades`;
DROP TABLE IF EXISTS `desafio_participantes`;
DROP TABLE IF EXISTS `desafios`;
DROP TABLE IF EXISTS `usuarios`;
