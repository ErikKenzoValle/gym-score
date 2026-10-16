-- +goose Up
CREATE TABLE `desafio_checkins` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'Identificador do check-in',
  `id_desafio` bigint unsigned NOT NULL COMMENT 'Desafio',
  `id_usuario` bigint unsigned NOT NULL COMMENT 'Participante que fez o check-in',
  `texto` varchar(500) NULL COMMENT 'Descrição opcional do treino',
  `foto_path` varchar(255) NULL COMMENT 'Caminho relativo da foto em uploads/checkins/',
  `latitude` decimal(10,8) NULL COMMENT 'Latitude opcional',
  `longitude` decimal(11,8) NULL COMMENT 'Longitude opcional',
  `data` date NOT NULL COMMENT 'Dia do check-in (1 por participante por dia)',
  `criado_em` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Data/hora de criação',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_checkin_desafio_usuario_data` (`id_desafio`,`id_usuario`,`data`),
  KEY `idx_checkins_usuario_data` (`id_usuario`,`data`),
  CONSTRAINT `fk_checkins_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_checkins_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Check-ins (prova de treino) de participantes de desafios';

CREATE TABLE `desafio_votos` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'Identificador do voto',
  `id_desafio` bigint unsigned NOT NULL COMMENT 'Desafio',
  `id_eleitor` bigint unsigned NOT NULL COMMENT 'Participante que votou',
  `id_votado` bigint unsigned NOT NULL COMMENT 'Participante votado',
  `criado_em` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Data/hora do voto',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_voto_desafio_eleitor` (`id_desafio`,`id_eleitor`),
  KEY `idx_votos_votado` (`id_desafio`,`id_votado`),
  KEY `fk_votos_eleitor` (`id_eleitor`),
  KEY `fk_votos_votado` (`id_votado`),
  CONSTRAINT `fk_votos_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_votos_eleitor` FOREIGN KEY (`id_eleitor`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_votos_votado` FOREIGN KEY (`id_votado`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `chk_voto_nao_em_si` CHECK (`id_eleitor` <> `id_votado`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Votos da apuração de desafios (1 por participante)';

-- +goose Down
DROP TABLE IF EXISTS `desafio_votos`;
DROP TABLE IF EXISTS `desafio_checkins`;
