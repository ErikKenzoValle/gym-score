-- +goose Up
-- N1 (triagem pós-Sprint 6): no fluxo antigo quem criava o desafio entrava como participante pagando a
-- entrada. A 00011 só cancelou os legados ativos criados por quem NÃO é academia, então desafios legados
-- de academia continuam ativos com a própria conta `academia` inscrita — e ela, sendo staff, poderia
-- marcar a própria presença e se pôr no pódio. Aqui: em desafios ATIVOS (aberto/em_andamento), toda
-- inscrição de conta com perfil `academia` é estornada pelo ledger e removida.
--
-- Mesmo padrão da 00011 (`_goose_00011_estornar_legados`): procedure transacional, valor estornado
-- calculado pelo próprio ledger (-SUM de entrada_desafio + estorno_desafio efetivos por usuário/desafio),
-- então quem já foi estornado dá 0 e rodar de novo não duplica nada. Preserva a invariante
-- saldo_centavos == SUM(ledger efetivo). Inscrição que esteja no pódio (FK fk_podio_desafios_inscricao)
-- não é removida — num desafio ativo isso não deveria existir.
-- O desafio em si NÃO é cancelado: os atletas inscritos seguem normalmente.

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00017_remover_academias`;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE PROCEDURE `_goose_00017_remover_academias`()
BEGIN
  DECLARE EXIT HANDLER FOR SQLEXCEPTION
  BEGIN
    ROLLBACK;
    RESIGNAL;
  END;

  START TRANSACTION;

  -- Inscrições alvo, travadas (FOR UPDATE) durante a transação.
  DROP TEMPORARY TABLE IF EXISTS `_goose_00017_tmp_inscricoes`;
  CREATE TEMPORARY TABLE `_goose_00017_tmp_inscricoes` (
    `id_participante` BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    `id_desafio` BIGINT UNSIGNED NOT NULL, `id_usuario` BIGINT UNSIGNED NOT NULL, `titulo` VARCHAR(200) NOT NULL,
    UNIQUE KEY (`id_usuario`, `id_desafio`)
  );
  INSERT INTO `_goose_00017_tmp_inscricoes` (`id_participante`, `id_desafio`, `id_usuario`, `titulo`)
  SELECT dp.`id`, dp.`id_desafio`, dp.`id_usuario`, d.`titulo`
    FROM `desafio_participantes` dp
    JOIN `desafios` d ON d.`id` = dp.`id_desafio`
    JOIN `usuarios` u ON u.`id` = dp.`id_usuario`
   WHERE d.`status` IN ('aberto', 'em_andamento') AND u.`perfil` = 'academia'
     AND NOT EXISTS (SELECT 1 FROM `podio_desafios` p
                      WHERE p.`id_desafio` = dp.`id_desafio` AND p.`id_participante` = dp.`id_usuario`)
     FOR UPDATE;

  DROP TEMPORARY TABLE IF EXISTS `_goose_00017_tmp_estornos`;
  CREATE TEMPORARY TABLE `_goose_00017_tmp_estornos` (
    `id_usuario` BIGINT UNSIGNED NOT NULL, `id_desafio` BIGINT UNSIGNED NOT NULL, `valor` BIGINT NOT NULL,
    PRIMARY KEY (`id_usuario`, `id_desafio`)
  );
  INSERT INTO `_goose_00017_tmp_estornos` (`id_usuario`, `id_desafio`, `valor`)
  SELECT t.`id_usuario`, t.`id_desafio`, -SUM(t.`valor_centavos`)
    FROM `transacoes` t
    JOIN `_goose_00017_tmp_inscricoes` i ON i.`id_usuario` = t.`id_usuario` AND i.`id_desafio` = t.`id_desafio`
   WHERE t.`tipo` IN ('entrada_desafio', 'estorno_desafio') AND t.`status` IN ('received', 'concluido')
   GROUP BY t.`id_usuario`, t.`id_desafio`
  HAVING -SUM(t.`valor_centavos`) > 0;

  INSERT INTO `transacoes` (`id_usuario`, `tipo`, `valor_centavos`, `id_desafio`, `asaas_payment_id`, `status`, `descricao`, `created_at`, `updated_at`)
  SELECT e.`id_usuario`, 'estorno_desafio', e.`valor`, e.`id_desafio`, NULL, 'concluido',
         CONCAT('Estorno: academia removida dos participantes do desafio #', e.`id_desafio`, ' (migração)'), NOW(3), NOW(3)
    FROM `_goose_00017_tmp_estornos` e;

  UPDATE `usuarios` u
    JOIN (SELECT `id_usuario`, SUM(`valor`) AS total FROM `_goose_00017_tmp_estornos` GROUP BY `id_usuario`) s ON s.`id_usuario` = u.`id`
     SET u.`saldo_centavos` = u.`saldo_centavos` + s.total;

  INSERT INTO `notificacoes` (`id_usuario`, `tipo`, `titulo`, `mensagem`, `link`, `criado_em`)
  SELECT i.`id_usuario`, 'desafio_cancelado', 'Inscrição removida',
         CONCAT('Sua conta de academia foi removida dos participantes do desafio "', i.`titulo`, '"',
                IF(e.`valor` IS NULL, '.', ' e a entrada foi reembolsada.')),
         CONCAT('/desafios?id=', i.`id_desafio`), NOW(3)
    FROM `_goose_00017_tmp_inscricoes` i
    LEFT JOIN `_goose_00017_tmp_estornos` e ON e.`id_usuario` = i.`id_usuario` AND e.`id_desafio` = i.`id_desafio`;

  DELETE dp FROM `desafio_participantes` dp JOIN `_goose_00017_tmp_inscricoes` i ON i.`id_participante` = dp.`id`;

  COMMIT;
  DROP TEMPORARY TABLE `_goose_00017_tmp_estornos`;
  DROP TEMPORARY TABLE `_goose_00017_tmp_inscricoes`;
END;
-- +goose StatementEnd
CALL `_goose_00017_remover_academias`();
DROP PROCEDURE `_goose_00017_remover_academias`;

-- +goose Down
-- Sem reversão: o estorno já devolveu o dinheiro à academia e recriar a inscrição exigiria debitá-la
-- de novo (quebraria a invariante saldo == ledger). Down é no-op de propósito.
SELECT 1;
