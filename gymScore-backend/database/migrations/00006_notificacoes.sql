-- +goose Up
CREATE TABLE `notificacoes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'Identificador da notificação',
  `id_usuario` bigint unsigned NOT NULL COMMENT 'Destinatário',
  `tipo` varchar(40) NOT NULL COMMENT 'Tipo do evento (ex.: amizade_solicitada, desafio_convite)',
  `titulo` varchar(150) NOT NULL COMMENT 'Título curto',
  `mensagem` varchar(500) NOT NULL COMMENT 'Texto da notificação',
  `link` varchar(255) NULL COMMENT 'Rota do frontend para abrir ao clicar',
  `lida_em` datetime(3) NULL COMMENT 'NULL = não lida',
  `criado_em` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Data/hora de criação',
  PRIMARY KEY (`id`),
  KEY `idx_notificacoes_usuario_lida` (`id_usuario`,`lida_em`,`criado_em`),
  CONSTRAINT `fk_notificacoes_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Notificações in-app';

-- +goose Down
DROP TABLE IF EXISTS `notificacoes`;
