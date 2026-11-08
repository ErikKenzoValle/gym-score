-- +goose Up
-- Moderação (Sprint 7) — reconstrução fiel da migration 00013 aplicada no RDS de testes em 2026-10-20
-- (o código original se perdeu). Os COMMENTs abaixo são cópia literal dos do RDS e são a especificação.
-- Prova: 00001–00012 + este arquivo reproduz o schema do RDS (menos os parciais da antiga 00014 e o
-- legado AutoMigrate) — ver analise/moderacao-schema-delta.md.
--
-- Conteúdo: denúncias (com evidências em imagem e sigilo do denunciante), punições (advertência, suspensão,
-- banimento, bloqueio na academia, anulação de resultado) com recurso, janela de contestação do pódio
-- (pagamentos_pendentes), débitos de usuário após anulação, linha do tempo append-only (moderacao_eventos);
-- em tabelas existentes: suspensão/banimento em `usuarios`, lançamento/anulação do pódio em `desafios` e
-- dois tipos novos no ledger.
--
-- No RDS esta versão já está registrada (o goose pula). Num banco novo cria tudo. Mesmo padrão da
-- 00011/00012: ADD condicional via procedure temporária, CREATE TABLE IF NOT EXISTS, MODIFY idempotente.
-- Observação: os atores "admin" (punicoes.aplicada_por/revogada_por, recursos.julgado_por, denuncias.id_responsavel,
-- moderacao_eventos.id_ator) apontam para `usuarios`, como no RDS; a troca para `administradores` está na
-- 00019_moderacao_integracao (não aqui, para não divergir do RDS).

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00013_add`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Adiciona coluna/índice/FK/CHECK somente se ainda não existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00013_add`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64), IN p_definicao TEXT)
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

-- ── usuarios: perfil 'admin' e estado efetivo de suspensão/banimento ──
-- (No RDS o 'admin' de perfil existe; a decisão da rodada 3 é não usá-lo — admins ficam em `administradores`.)
ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia','instrutor','admin') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta (admin só por cmd/admin, nunca por cadastro)';
CALL `_goose_00013_add`('usuarios', 'coluna', 'suspenso_ate', "COLUMN `suspenso_ate` DATETIME NULL COMMENT 'Suspensão global vigente até este instante (NULL = não suspenso)' AFTER `perfil`");
CALL `_goose_00013_add`('usuarios', 'coluna', 'banido_em', "COLUMN `banido_em` DATETIME NULL COMMENT 'Quando foi banido (NULL = não banido); login recusado' AFTER `suspenso_ate`");
CALL `_goose_00013_add`('usuarios', 'coluna', 'motivo_bloqueio', "COLUMN `motivo_bloqueio` VARCHAR(255) NULL COMMENT 'Motivo exibido ao usuário suspenso/banido (cópia resumida da justificativa)' AFTER `banido_em`");

-- ── desafios: lançamento do pódio (início da janela de contestação) e anulação do resultado ──
CALL `_goose_00013_add`('desafios', 'coluna', 'podio_lancado_por', "COLUMN `podio_lancado_por` BIGINT UNSIGNED NULL COMMENT 'Usuário (instrutor designado ou academia dona) que lançou o pódio' AFTER `reserva_academia_centavos`");
CALL `_goose_00013_add`('desafios', 'coluna', 'podio_lancado_em', "COLUMN `podio_lancado_em` DATETIME(3) NULL COMMENT 'Quando o pódio foi lançado (base de liberar_em dos pagamentos pendentes)' AFTER `podio_lancado_por`");
CALL `_goose_00013_add`('desafios', 'coluna', 'resultado_anulado_em', "COLUMN `resultado_anulado_em` DATETIME(3) NULL COMMENT 'Quando o resultado foi anulado pelo admin (NULL = vigente)' AFTER `podio_lancado_em`");
CALL `_goose_00013_add`('desafios', 'coluna', 'resultado_anulacao_modo', "COLUMN `resultado_anulacao_modo` ENUM('relancar_podio','devolver_inscricoes') NULL COMMENT 'Opção da anulação: (a) relançar pódio ou (b) devolver inscrições' AFTER `resultado_anulado_em`");
CALL `_goose_00013_add`('desafios', 'indice', 'fk_desafios_podio_lancado_por', "KEY `fk_desafios_podio_lancado_por` (`podio_lancado_por`)");
CALL `_goose_00013_add`('desafios', 'fk', 'fk_desafios_podio_lancado_por', "CONSTRAINT `fk_desafios_podio_lancado_por` FOREIGN KEY (`podio_lancado_por`) REFERENCES `usuarios` (`id`)");
CALL `_goose_00013_add`('desafios', 'check', 'chk_desafios_anulacao', "CONSTRAINT `chk_desafios_anulacao` CHECK ((`resultado_anulado_em` IS NULL) = (`resultado_anulacao_modo` IS NULL))");

-- ── Ledger: novos tipos (valores existentes mantidos, na mesma ordem) ──
-- estorno_anulacao: estorno de crédito do pódio quando o resultado é anulado após a liberação;
-- abatimento_debito: desconto de usuario_debitos em créditos futuros do devedor.
ALTER TABLE `transacoes`
  MODIFY COLUMN `tipo` ENUM('deposito','saque','entrada_desafio','estorno_desafio','premio_desafio','ajuste','reserva_desafio','estorno_reserva','pagamento_instrutor','receita_academia','tarifa_saque','estorno_anulacao','abatimento_debito') NOT NULL DEFAULT 'deposito' COMMENT 'Tipo da movimentação';

-- ── denuncias ──
-- chave_aberta + UNIQUE: no máximo 1 denúncia aberta/em análise por (denunciante, denunciado, desafio).
CREATE TABLE IF NOT EXISTS `denuncias` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_denunciante` BIGINT UNSIGNED NOT NULL COMMENT 'Quem denunciou (sigiloso: só o admin enxerga)',
  `id_denunciado` BIGINT UNSIGNED NOT NULL COMMENT 'Usuário denunciado',
  `id_desafio` BIGINT UNSIGNED NULL COMMENT 'Desafio relacionado (obrigatório em suborno_instrutor, manipulacao_podio e fraude_presenca; validado na API)',
  `categoria` ENUM('suborno_instrutor','manipulacao_podio','fraude_presenca','conduta_abusiva','perfil_falso','outro') NOT NULL COMMENT 'Categoria da denúncia',
  `descricao` TEXT NOT NULL COMMENT 'Relato do denunciante (20 a 2000 caracteres, validado na API)',
  `status` ENUM('aberta','em_analise','procedente','improcedente','arquivada') NOT NULL DEFAULT 'aberta' COMMENT 'Ciclo: aberta -> em_analise -> procedente | improcedente; arquivada = duplicada/sem fundamento',
  `jurisdicao` ENUM('admin','academia') NOT NULL DEFAULT 'admin' COMMENT 'Quem julga agora: academia só em suborno_instrutor de desafio dela; vira admin por encaminhamento ou prazo',
  `id_academia_julgadora` BIGINT UNSIGNED NULL COMMENT 'Academia que julga (academias.id); preenchida mesmo após subir ao admin, para histórico',
  `id_responsavel` BIGINT UNSIGNED NULL COMMENT 'Usuário (admin ou conta da academia) que assumiu a análise',
  `prazo_academia_ate` DATETIME(3) NULL COMMENT 'Limite para a academia decidir; vencido, o job sobe a denúncia ao admin',
  `encaminhada_em` DATETIME(3) NULL COMMENT 'Quando subiu ao admin (encaminhamento da academia ou escalonamento por prazo)',
  `decidida_em` DATETIME(3) NULL COMMENT 'Quando recebeu decisão final (procedente/improcedente/arquivada)',
  `decisao_texto` TEXT NULL COMMENT 'Justificativa da decisão final',
  `chave_aberta` VARCHAR(70) GENERATED ALWAYS AS (IF(`status` IN ('aberta','em_analise'), CONCAT(`id_denunciante`, '-', `id_denunciado`, '-', COALESCE(`id_desafio`, 0)), NULL)) STORED COMMENT 'Coluna gerada (não gravar): chave de unicidade da denúncia aberta',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Abertura da denúncia',
  `atualizado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'Última alteração',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_denuncia_aberta` (`chave_aberta`),
  KEY `idx_denuncias_fila` (`status`, `jurisdicao`, `criado_em`),
  KEY `idx_denuncias_categoria_status` (`categoria`, `status`, `criado_em`),
  KEY `idx_denuncias_denunciado` (`id_denunciado`, `status`),
  KEY `idx_denuncias_denunciante_data` (`id_denunciante`, `criado_em`),
  KEY `idx_denuncias_desafio_retencao` (`id_desafio`, `status`, `categoria`),
  KEY `idx_denuncias_academia_fila` (`id_academia_julgadora`, `jurisdicao`, `status`, `criado_em`),
  KEY `idx_denuncias_escalonamento` (`status`, `jurisdicao`, `prazo_academia_ate`),
  KEY `idx_denuncias_responsavel` (`id_responsavel`),
  CONSTRAINT `fk_denuncias_academia` FOREIGN KEY (`id_academia_julgadora`) REFERENCES `academias` (`id`),
  CONSTRAINT `fk_denuncias_denunciado` FOREIGN KEY (`id_denunciado`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_denuncias_denunciante` FOREIGN KEY (`id_denunciante`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_denuncias_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_denuncias_responsavel` FOREIGN KEY (`id_responsavel`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `chk_denuncias_autodenuncia` CHECK (`id_denunciante` <> `id_denunciado`),
  CONSTRAINT `chk_denuncias_jurisdicao_academia` CHECK (`jurisdicao` <> 'academia' OR (`id_academia_julgadora` IS NOT NULL AND `categoria` = 'suborno_instrutor'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Denúncias entre usuários (sigilo do denunciante; nada é apagado)';

-- ── denuncia_evidencias: até 3 imagens por denúncia ──
CREATE TABLE IF NOT EXISTS `denuncia_evidencias` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_denuncia` BIGINT UNSIGNED NOT NULL COMMENT 'Denúncia à qual a imagem pertence',
  `ordem` TINYINT UNSIGNED NOT NULL COMMENT 'Posição da imagem (1 a 3)',
  `caminho` VARCHAR(255) NOT NULL COMMENT 'Caminho/nome aleatório do arquivo salvo (nunca o nome original)',
  `mime` VARCHAR(50) NOT NULL COMMENT 'Tipo validado por magic bytes (image/jpeg, image/png, image/webp)',
  `tamanho_bytes` BIGINT UNSIGNED NOT NULL COMMENT 'Tamanho do arquivo em bytes',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando foi enviada',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_evidencia_ordem` (`id_denuncia`, `ordem`),
  CONSTRAINT `fk_evidencia_denuncia` FOREIGN KEY (`id_denuncia`) REFERENCES `denuncias` (`id`),
  CONSTRAINT `chk_evidencia_ordem` CHECK (`ordem` BETWEEN 1 AND 3),
  CONSTRAINT `chk_evidencia_tamanho` CHECK (`tamanho_bytes` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Imagens de evidência das denúncias (máx. 3 por denúncia)';

-- ── punicoes ──
CREATE TABLE IF NOT EXISTS `punicoes` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_denuncia` BIGINT UNSIGNED NULL COMMENT 'Denúncia que originou a punição (NULL = aplicada direto pelo admin)',
  `id_usuario` BIGINT UNSIGNED NOT NULL COMMENT 'Usuário punido',
  `tipo` ENUM('advertencia','suspensao','banimento','bloqueio_academia','anulacao_resultado') NOT NULL COMMENT 'Tipo da punição',
  `escopo` ENUM('global','academia') NOT NULL DEFAULT 'global' COMMENT 'academia = efeito só na academia id_academia (advertência/bloqueio aplicados por academia)',
  `id_academia` BIGINT UNSIGNED NULL COMMENT 'Academia (academias.id) do escopo; obrigatório quando escopo = academia',
  `id_desafio` BIGINT UNSIGNED NULL COMMENT 'Desafio alvo (anulacao_resultado)',
  `inicio` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Início da vigência',
  `fim` DATETIME(3) NULL COMMENT 'Fim da vigência (NULL = permanente/sem prazo; suspensão sempre tem fim)',
  `aplicada_por` BIGINT UNSIGNED NOT NULL COMMENT 'Admin ou conta da academia que aplicou',
  `justificativa` TEXT NOT NULL COMMENT 'Justificativa obrigatória',
  `revogada_em` DATETIME(3) NULL COMMENT 'Quando foi revogada (NULL = não revogada)',
  `revogada_por` BIGINT UNSIGNED NULL COMMENT 'Admin que revogou',
  `revogada_motivo` TEXT NULL COMMENT 'Justificativa da revogação',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando foi registrada',
  PRIMARY KEY (`id`),
  KEY `idx_punicoes_usuario_vigencia` (`id_usuario`, `tipo`, `revogada_em`, `fim`),
  KEY `idx_punicoes_usuario_tipo_inicio` (`id_usuario`, `tipo`, `inicio`),
  KEY `idx_punicoes_academia_usuario` (`id_academia`, `id_usuario`, `tipo`),
  KEY `idx_punicoes_denuncia` (`id_denuncia`),
  KEY `idx_punicoes_desafio` (`id_desafio`),
  KEY `idx_punicoes_aplicada_por` (`aplicada_por`),
  KEY `idx_punicoes_revogada_por` (`revogada_por`),
  CONSTRAINT `fk_punicoes_academia` FOREIGN KEY (`id_academia`) REFERENCES `academias` (`id`),
  CONSTRAINT `fk_punicoes_aplicada_por` FOREIGN KEY (`aplicada_por`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_punicoes_denuncia` FOREIGN KEY (`id_denuncia`) REFERENCES `denuncias` (`id`),
  CONSTRAINT `fk_punicoes_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_punicoes_revogada_por` FOREIGN KEY (`revogada_por`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_punicoes_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `chk_punicoes_anulacao_desafio` CHECK (`tipo` <> 'anulacao_resultado' OR `id_desafio` IS NOT NULL),
  CONSTRAINT `chk_punicoes_bloqueio_escopo` CHECK (`tipo` <> 'bloqueio_academia' OR `escopo` = 'academia'),
  CONSTRAINT `chk_punicoes_escopo_academia` CHECK ((`escopo` = 'academia') = (`id_academia` IS NOT NULL)),
  CONSTRAINT `chk_punicoes_periodo` CHECK (`fim` IS NULL OR `fim` >= `inicio`),
  CONSTRAINT `chk_punicoes_revogacao` CHECK ((`revogada_em` IS NULL) = (`revogada_por` IS NULL)),
  CONSTRAINT `chk_punicoes_suspensao_fim` CHECK (`tipo` <> 'suspensao' OR `fim` IS NOT NULL),
  CONSTRAINT `chk_punicoes_tipo_escopo` CHECK (`tipo` IN ('advertencia','bloqueio_academia') OR `escopo` = 'global')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Punições aplicadas (advertência, suspensão, banimento, bloqueio na academia, anulação de resultado)';

-- ── recursos: 1 por punição ──
CREATE TABLE IF NOT EXISTS `recursos` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_punicao` BIGINT UNSIGNED NOT NULL COMMENT 'Punição recorrida (no máximo 1 recurso por punição)',
  `texto` TEXT NOT NULL COMMENT 'Argumentos do punido',
  `status` ENUM('pendente','deferido','indeferido') NOT NULL DEFAULT 'pendente' COMMENT 'deferido = punição revogada',
  `julgado_por` BIGINT UNSIGNED NULL COMMENT 'Admin que julgou',
  `julgado_em` DATETIME(3) NULL COMMENT 'Quando foi julgado',
  `decisao_texto` TEXT NULL COMMENT 'Justificativa do julgamento',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando o recurso foi aberto',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_recurso_punicao` (`id_punicao`),
  KEY `idx_recursos_fila` (`status`, `criado_em`),
  KEY `idx_recursos_julgado_por` (`julgado_por`),
  CONSTRAINT `fk_recursos_julgado_por` FOREIGN KEY (`julgado_por`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_recursos_punicao` FOREIGN KEY (`id_punicao`) REFERENCES `punicoes` (`id`),
  CONSTRAINT `chk_recursos_julgamento` CHECK ((`status` = 'pendente') = (`julgado_em` IS NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Recursos contra punições (1 por punição)';

-- ── pagamentos_pendentes: créditos do pódio na janela de contestação (fora do saldo até liberar) ──
CREATE TABLE IF NOT EXISTS `pagamentos_pendentes` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_desafio` BIGINT UNSIGNED NOT NULL COMMENT 'Desafio que gerou o pagamento',
  `id_usuario` BIGINT UNSIGNED NULL COMMENT 'Beneficiário (NULL = plataforma)',
  `tipo` ENUM('premio_desafio','pagamento_instrutor','receita_academia','taxa_inscricao') NOT NULL COMMENT 'Tipo do ledger que será gravado na liberação (taxa_inscricao = plataforma_receitas)',
  `valor_centavos` BIGINT NOT NULL COMMENT 'Valor em centavos (sempre > 0)',
  `status` ENUM('a_liberar','retido','liberado','cancelado') NOT NULL DEFAULT 'a_liberar' COMMENT 'retido = denúncia aberta sobre o desafio; liberado = já está no ledger',
  `liberar_em` DATETIME(3) NOT NULL COMMENT 'Fim da janela de contestação (podio_lancado_em + PREMIO_JANELA_CONTESTACAO_HORAS)',
  `id_denuncia_retencao` BIGINT UNSIGNED NULL COMMENT 'Denúncia que causou a retenção (para devolver a a_liberar ao ser julgada)',
  `liberado_em` DATETIME(3) NULL COMMENT 'Quando foi liberado ao ledger',
  `id_transacao` BIGINT UNSIGNED NULL COMMENT 'Linha do ledger gerada na liberação (NULL em plataforma, pendente e cancelado)',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando foi criado (lançamento do pódio)',
  `atualizado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'Última mudança de status',
  PRIMARY KEY (`id`),
  KEY `idx_pendentes_job` (`status`, `liberar_em`),
  KEY `idx_pendentes_desafio` (`id_desafio`, `status`),
  KEY `idx_pendentes_usuario` (`id_usuario`, `status`),
  KEY `idx_pendentes_denuncia` (`id_denuncia_retencao`),
  KEY `idx_pendentes_transacao` (`id_transacao`),
  CONSTRAINT `fk_pendentes_denuncia` FOREIGN KEY (`id_denuncia_retencao`) REFERENCES `denuncias` (`id`),
  CONSTRAINT `fk_pendentes_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_pendentes_transacao` FOREIGN KEY (`id_transacao`) REFERENCES `transacoes` (`id`),
  CONSTRAINT `fk_pendentes_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `chk_pendentes_beneficiario` CHECK ((`tipo` = 'taxa_inscricao') = (`id_usuario` IS NULL)),
  CONSTRAINT `chk_pendentes_liberado` CHECK ((`status` = 'liberado') = (`liberado_em` IS NOT NULL)),
  CONSTRAINT `chk_pendentes_valor` CHECK (`valor_centavos` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Créditos retidos na janela de contestação do prêmio (fora do saldo até liberar)';

-- ── usuario_debitos: o que faltou estornar numa anulação após a liberação ──
CREATE TABLE IF NOT EXISTS `usuario_debitos` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_usuario` BIGINT UNSIGNED NOT NULL COMMENT 'Devedor',
  `id_desafio` BIGINT UNSIGNED NOT NULL COMMENT 'Desafio cuja anulação gerou o débito',
  `valor_centavos` BIGINT NOT NULL COMMENT 'Valor original do débito (o que faltou estornar do saldo), em centavos',
  `saldo_restante_centavos` BIGINT NOT NULL COMMENT 'Quanto ainda falta pagar; 0 = quitado',
  `motivo` VARCHAR(255) NOT NULL COMMENT 'Motivo (ex.: anulação do resultado do desafio X)',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando o débito nasceu',
  `quitado_em` DATETIME(3) NULL COMMENT 'Quando o saldo_restante chegou a 0',
  PRIMARY KEY (`id`),
  KEY `idx_debitos_usuario_aberto` (`id_usuario`, `quitado_em`, `id`),
  KEY `idx_debitos_desafio` (`id_desafio`),
  CONSTRAINT `fk_debitos_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`),
  CONSTRAINT `fk_debitos_usuario` FOREIGN KEY (`id_usuario`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `chk_debitos_quitado` CHECK ((`saldo_restante_centavos` = 0) = (`quitado_em` IS NOT NULL)),
  CONSTRAINT `chk_debitos_restante` CHECK (`saldo_restante_centavos` BETWEEN 0 AND `valor_centavos`),
  CONSTRAINT `chk_debitos_valor` CHECK (`valor_centavos` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Débitos de usuários (anulação após liberação); com débito aberto o saque é bloqueado';

-- ── moderacao_eventos: linha do tempo append-only ──
CREATE TABLE IF NOT EXISTS `moderacao_eventos` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `id_ator` BIGINT UNSIGNED NULL COMMENT 'Quem agiu (NULL = sistema/job)',
  `ator_papel` ENUM('admin','academia','usuario','sistema') NOT NULL COMMENT 'Papel com que o ator agiu',
  `acao` VARCHAR(60) NOT NULL COMMENT 'Ação, ex.: denuncia_criada, denuncia_assumida, punicao_aplicada, podio_lancado, pagamento_liberado',
  `entidade` ENUM('denuncia','punicao','recurso','desafio','pagamento','debito','usuario') NOT NULL COMMENT 'Tipo da entidade afetada',
  `id_entidade` BIGINT UNSIGNED NOT NULL COMMENT 'ID da entidade afetada (na tabela correspondente a `entidade`)',
  `id_desafio` BIGINT UNSIGNED NULL COMMENT 'Desafio relacionado, quando houver (consulta da linha do tempo do desafio)',
  `justificativa` TEXT NULL COMMENT 'Justificativa (obrigatória nas ações de decisão; validado na API)',
  `dados` JSON NULL COMMENT 'Valores antes/depois e contexto: {"antes":{...},"depois":{...}}',
  `criado_em` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'Quando ocorreu',
  PRIMARY KEY (`id`),
  KEY `idx_eventos_entidade` (`entidade`, `id_entidade`, `id`),
  KEY `idx_eventos_desafio` (`id_desafio`, `id`),
  KEY `idx_eventos_ator` (`id_ator`, `criado_em`),
  CONSTRAINT `fk_eventos_ator` FOREIGN KEY (`id_ator`) REFERENCES `usuarios` (`id`),
  CONSTRAINT `fk_eventos_desafio` FOREIGN KEY (`id_desafio`) REFERENCES `desafios` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Auditoria append-only de moderação e do lançamento do pódio';

DROP PROCEDURE `_goose_00013_add`;

-- +goose Down
-- Reversão com perda de informação: denúncias, evidências (registros; os arquivos não são apagados aqui),
-- punições, recursos, pagamentos pendentes, débitos e eventos são descartados. Pagamentos pendentes ainda
-- não liberados NÃO voltam ao ledger — só rodar Down em banco descartável. Lançamentos estorno_anulacao /
-- abatimento_debito viram 'ajuste' e contas perfil 'admin' viram 'atleta' antes de encolher os ENUMs.

-- +goose StatementBegin
DROP PROCEDURE IF EXISTS `_goose_00013_drop`;
-- +goose StatementEnd
-- +goose StatementBegin
-- Remove coluna/índice/FK/CHECK somente se existir. p_tipo: coluna | indice | fk | check.
CREATE PROCEDURE `_goose_00013_drop`(IN p_tabela VARCHAR(64), IN p_tipo VARCHAR(10), IN p_nome VARCHAR(64))
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

DROP TABLE IF EXISTS `moderacao_eventos`;
DROP TABLE IF EXISTS `usuario_debitos`;
DROP TABLE IF EXISTS `pagamentos_pendentes`;
DROP TABLE IF EXISTS `recursos`;
DROP TABLE IF EXISTS `punicoes`;
DROP TABLE IF EXISTS `denuncia_evidencias`;
DROP TABLE IF EXISTS `denuncias`;

UPDATE `transacoes` SET `tipo` = 'ajuste' WHERE `tipo` IN ('estorno_anulacao','abatimento_debito');
ALTER TABLE `transacoes`
  MODIFY COLUMN `tipo` ENUM('deposito','saque','entrada_desafio','estorno_desafio','premio_desafio','ajuste','reserva_desafio','estorno_reserva','pagamento_instrutor','receita_academia','tarifa_saque') NOT NULL DEFAULT 'deposito' COMMENT 'Tipo da movimentação';

CALL `_goose_00013_drop`('desafios', 'check', 'chk_desafios_anulacao');
CALL `_goose_00013_drop`('desafios', 'fk', 'fk_desafios_podio_lancado_por');
CALL `_goose_00013_drop`('desafios', 'indice', 'fk_desafios_podio_lancado_por');
CALL `_goose_00013_drop`('desafios', 'coluna', 'resultado_anulacao_modo');
CALL `_goose_00013_drop`('desafios', 'coluna', 'resultado_anulado_em');
CALL `_goose_00013_drop`('desafios', 'coluna', 'podio_lancado_em');
CALL `_goose_00013_drop`('desafios', 'coluna', 'podio_lancado_por');

CALL `_goose_00013_drop`('usuarios', 'coluna', 'motivo_bloqueio');
CALL `_goose_00013_drop`('usuarios', 'coluna', 'banido_em');
CALL `_goose_00013_drop`('usuarios', 'coluna', 'suspenso_ate');
UPDATE `usuarios` SET `perfil` = 'atleta' WHERE `perfil` = 'admin';
ALTER TABLE `usuarios`
  MODIFY COLUMN `perfil` ENUM('atleta','academia','instrutor') NOT NULL DEFAULT 'atleta' COMMENT 'Perfil RBAC da conta';

DROP PROCEDURE `_goose_00013_drop`;
