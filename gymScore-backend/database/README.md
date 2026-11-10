# Banco de dados — migrations

O schema é versionado com [goose](https://github.com/pressly/goose) (arquivos SQL em `migrations/`,
com `-- +goose Up` / `-- +goose Down`). O `AutoMigrate` do GORM não é mais usado. O antigo
`schema.sql` foi removido: a fonte da verdade são as migrations.

## Migrations

| Versão | Arquivo | O que faz |
|---|---|---|
| 1 | `00001_baseline.sql` | Schema que o AutoMigrate gerava (`CREATE TABLE IF NOT EXISTS`, idempotente: em bancos existentes não altera nada) |
| 2 | `00002_dinheiro_centavos.sql` | `usuarios.saldo_centavos` BIGINT + `CHECK >= 0`, convertido de `saldo` (a coluna `saldo` só é removida na 00003) |
| 3 | `00003_ledger.sql` | `transacoes` vira ledger (`tipo` incl. `ajuste`, `valor_centavos` assinado, `id_desafio`, `asaas_transfer_id`, `descricao`); cria linha de abertura `ajuste` por usuário com saldo; remove `usuarios.saldo` |
| 4 | `00004_desafios_v2.sql` | Modelo de desafio v2 (entrada em centavos, novos status, prêmio/vencedor em participantes) |
| 5 | `00005_checkins_votos.sql` | `desafio_checkins` e `desafio_votos` |
| 6 | `00006_notificacoes.sql` | `notificacoes` |
| 7 | `00007_reset_senha.sql` | `password_reset_tokens` |
| 8 | `00008_amizades_unicas.sql` | Remove duplicatas de amizade e impede par duplicado em qualquer direção |
| 9 | `00009_token_versao.sql` | `usuarios.token_versao` (revogação de JWT) |
| 10 | `00010_perfis_academia_instrutor.sql` | Perfis RBAC (`usuarios.perfil`/`username`/`elo`/`pontos`/`ultima_vez`), `cpf`/`data_nascimento`/`genero` nullable (conta "academia"), `desafio_participantes.presente` (check-in presencial), tabelas `academias`, `instrutores`, `podio_desafios`, `avaliacoes_desafios` |
| 11 | `00011_desafios_academia.sql` | Sprint 6 (Academia -> Instrutor -> Atleta): `desafios.id_academia`/`id_instrutor`/`data_evento`/`categoria`/`posicoes_podio`, status sem `em_votacao`, `desafio_participantes.codigo_ingresso` (QR) e `presente_em`, `podio_desafios.premio_centavos`; cancela e estorna desafios legados criados por não-academia |
| 12 | `00012_premiacao_financeiro.sql` | Sprint 6 (financeiro): em `desafios` prêmio fixo, pote (`dividir_pote`/`percentual_pote`), `taxa_plataforma_pct`, pagamento e seleção do instrutor (`designado`/`vaga_aberta` + teto), `reserva_academia_centavos`, todas com CHECK; tabelas `desafio_candidaturas_instrutor` e `plataforma_receitas`; novos tipos no ledger (`reserva_desafio`, `estorno_reserva`, `pagamento_instrutor`, `receita_academia`, `tarifa_saque`) |
| 13 | `00013_moderacao.sql` | Moderação (Sprint 7, reconstruída fiel ao RDS de testes, onde já está registrada): `denuncias` (sigilo do denunciante, `chave_aberta` gerada + UNIQUE = 1 aberta por trio, jurisdição admin/academia, prazo), `denuncia_evidencias` (até 3), `punicoes` (advertência/suspensão/banimento/bloqueio_academia/anulação_resultado, 7 CHECKs), `recursos` (1 por punição), `pagamentos_pendentes` (janela de contestação do pódio), `usuario_debitos`, `moderacao_eventos` (append-only); `usuarios.suspenso_ate`/`banido_em`/`motivo_bloqueio`; `desafios.podio_lancado_por`/`_em`, `resultado_anulado_em`/`resultado_anulacao_modo`; ledger + `estorno_anulacao`, `abatimento_debito` |
| 14 | `00014_administradores.sql` | Painel admin: tabelas `administradores` (papel `superadmin`/`admin`, `ativo`, `token_versao`, `criado_por` FK self) e `admin_auditoria` (append-only, `motivo` obrigatório, `detalhes` JSON); remove a procedure órfã `_goose_00014_add` deixada no RDS |
| 15 | `00015_moderacao_aprovacao.sql` | `usuarios.bloqueado*` (removidas na 19); `academias.status` e `instrutores.status` (`pendente`/`aprovada`/`rejeitada`, existentes viram `aprovada`, novos nascem `pendente`) + `motivo_rejeicao`, `aprovado_por` (FK `administradores`), `aprovado_em`. Idempotente sobre o estado parcial do RDS (antiga 00014 que falhou no meio) |
| 16 | `00016_ledger_integridade.sql` | Ledger: UNIQUE de `premio_desafio`/`pagamento_instrutor`/`receita_academia` por (desafio, usuário, tipo) via coluna gerada VIRTUAL `id_desafio_credito_unico`; CHECK `chk_transacoes_sinal` (sinal por tipo, `ajuste` livre; `estorno_anulacao` e `abatimento_debito` negativos); índices `(tipo, status, created_at)` e `(status, updated_at)`; `id_transacao_origem` (FK self, tarifa -> saque, backfill pela descrição, UNIQUE com `tipo`); marca `em_revisao`/`revisao_motivo` de saque. Aborta com SIGNAL se os dados existentes violarem as regras |
| 17 | `00017_academia_fora_de_desafios.sql` | N1: em desafios `aberto`/`em_andamento`, estorna pelo ledger e remove a inscrição de contas `academia` (procedure transacional, reaplicável); Down é no-op |
| 18 | `00018_convite_instrutor_presenca.sql` | Convite de vínculo academia→instrutor (`instrutores.id_academia_convite` FK `academias` ON DELETE SET NULL + `convite_em`, índice); presença com autoria: `desafio_participantes.presenca_marcada_por` (FK `usuarios` ON DELETE SET NULL) e `presenca_modo` ENUM(`qr`,`manual`) |
| 19 | `00019_moderacao_integracao.sql` | Integra a moderação às decisões da rodada 3: atores admin em `administradores` (`punicoes.aplicada_por_admin` + CHECK de exclusividade, `revogada_por` e `recursos.julgado_por` com FK para `administradores`, `denuncias.id_responsavel_admin`, `moderacao_eventos.id_admin` + CHECK por `ator_papel`); `usuarios.perfil` sem `admin`; bloqueio unificado (`bloqueado = 1` vira punição `banimento` + `banido_em`, colunas `bloqueado*` removidas); `plataforma_receitas.origem` + `estorno_taxa` (valor negativo, CHECK `chk_receita_sinal`); índices `idx_punicoes_expiracao`, `idx_usuarios_suspenso_ate`. Aborta (SIGNAL) se houver ação de admin gravada com id de `usuarios` |
| 20 | `00020_modo_instrutor_endereco.sql` | Rodada 4: `usuarios.perfil` vira tipo de conta (`atleta`/`academia`; contas `instrutor` voltam a `atleta`, o modo instrutor vem de `instrutores`); `academias` ganha `cep` (CHAR(8) ascii_bin, CHECK 8 dígitos), `logradouro`, `numero`, `complemento`, `bairro`, `cidade`, `uf` (CHAR(2) ascii_bin, CHECK das 27 UFs) + índice `(uf, cidade)`, `endereco` passa a VARCHAR(500) (texto formatado, sem backfill). Aborta (SIGNAL) se houver conta de academia com registro de instrutor; Down volta `perfil='instrutor'` para quem tem registro |
| 21 | `00021_desafios_equipe_local_horario.sql` | Rodada 5: `desafios` ganha endereço estruturado igual ao de `academias` (`cep`/`logradouro`/`numero`/`complemento`/`bairro`/`cidade`/`uf` + CHECKs, índice `(uf, cidade)`; `local` VARCHAR(500) = texto montado), `data_fim_evento` (backfill `data_evento + 2h`, CHECK fim > início, índice `(id_academia, status, data_evento, data_fim_evento)` para o choque de horário), `instrutores_vagas` 1..20 e CHECK de soma por instrutor (`chk_desafios_soma_pct_equipe` substitui `chk_desafios_soma_pct`); tabela `desafio_instrutores` (equipe, UNIQUE desafio+instrutor, 1 responsável via coluna gerada `responsavel_desafio` + UNIQUE). `id_instrutor` vira membro responsável (fica como espelho LEGADO); vaga aberta ativa sem instrutor → fixo = teto; `lance_centavos` NULL (legado), status de candidatura + `removida`. `instrutor_selecao`/`instrutor_teto_centavos` legados |
| 22 | `00022_premiacao_regras_filtros_nivel.sql` | Rodada 6: premiação configurável — `posicoes_podio` e `podio_desafios.posicao` 1..100 (`chk_desafios_posicoes_podio_100`/`chk_podio_posicao_100` substituem os de 1..5), tabela `desafio_premio_posicoes` (PK desafio+posição, `pct_bp` em pontos-base 1..10000; soma = 10000 e não crescente validados na API; backfill de todos os desafios com a tabela fixa antiga 100 / 70-30 / 60-30-10 / 50-25-15-10 / 40-25-15-12-8); `desafios.resumo` VARCHAR(280) e `regras` TEXT (≤ 10000, CHECK) com backfill de `descricao` (legado; resumo cai para o título); filtros `nivel_min`/`nivel_max` (1..10) e `idade_min`/`idade_max` (0..120), NULL = sem, min ≤ max; `usuarios.pontuacao_gymscore` (≥ 0), `nivel_gymscore` (1..10), `gymscore_calculado_em` (NULL = nunca calculado; sem backfill — o job do backend calcula); índices para o recálculo (`treino_conclusoes (id_usuario, concluido_em)`, `podio_desafios (id_participante, posicao, id_desafio)`, `usuarios (gymscore_calculado_em)`). Down descarta divisão/textos/filtros/cache e só recoloca os CHECKs 1..5 se os dados couberem |

Regras de conversão importantes:
- Saldos: `ROUND(saldo * 100)`, nunca negativo, `NULL` vira 0. Saldo legado negativo vira 0 e é registrado numa linha `ajuste` (descrição com o valor original).
- Ledger de abertura: para cada usuário com saldo > 0 (ou saldo legado negativo, ou ledger divergente) é criada uma linha `ajuste`/`received`, "Saldo migrado", com `valor_centavos = saldo_centavos - SUM(depósitos legados received)`, de modo que `saldo == SUM(ledger)` vale também para dados antigos.
- Transações antigas: viram `tipo='deposito'`, `valor_centavos = ROUND(valor*100)`, status preservado.
- Desafios antigos **não encerrados** (`pendente`, `aberto`, `em_andamento`) viram `cancelado` **sem movimentar saldo**
  (no modelo antigo não havia reserva). Encerrados continuam `encerrado`; o `id_vencedor` vira participante com `vencedor=1`
  e `premio_centavos = valor_entrada_centavos`.
- Desafios legados (00011) **não encerrados criados por quem não é academia** (`usuarios.perfil <> 'academia'`) viram `cancelado`
  **com estorno pelo ledger**: para cada (usuário, desafio) insere uma linha `estorno_desafio`/`concluido` com
  `-SUM(entrada_desafio + estorno_desafio efetivos)` (só se > 0; quem já saiu não é estornado de novo), soma ao
  `saldo_centavos` e cria uma notificação `desafio_cancelado`, tudo numa transação. Os de academia em `em_votacao` viram `em_andamento`.

Verificação da invariante saldo == ledger (deve retornar 0 linhas, antes e depois de qualquer migration):

```sql
SELECT u.id, u.saldo_centavos,
       COALESCE((SELECT SUM(t.valor_centavos) FROM transacoes t
                  WHERE t.id_usuario = u.id
                    AND (t.status IN ('received','concluido') OR (t.tipo = 'saque' AND t.status = 'pending'))), 0) AS soma_ledger
FROM usuarios u
HAVING saldo_centavos <> soma_ledger;
```
Receita da plataforma (taxa de inscrição + tarifa de saque; a plataforma não tem carteira de usuário):

```sql
-- Total por origem
SELECT origem, COUNT(*) AS qtd, SUM(valor_centavos) AS total_centavos
FROM plataforma_receitas GROUP BY origem;

-- Por mês e origem
SELECT DATE_FORMAT(criado_em, '%Y-%m') AS mes, origem, SUM(valor_centavos) AS total_centavos
FROM plataforma_receitas GROUP BY mes, origem ORDER BY mes, origem;
```

Invariantes financeiras por desafio (cada consulta deve retornar 0 linhas). `reserva_academia_centavos` guarda a reserva
vigente e **não é zerada** ao encerrar nem ao cancelar: quem diz se ela foi consumida ou devolvida é o status do desafio e as
linhas `reserva_desafio`/`estorno_reserva` do ledger. As entradas são **líquidas** (`entrada_desafio` + `estorno_desafio`,
já que atleta que sai do desafio aberto é reembolsado).

```sql
-- 1) Encerrado: entradas líquidas + reserva = prêmios + instrutor + academia + taxa (ao centavo).
--    Créditos do ledger: premio_desafio, pagamento_instrutor, receita_academia; taxa de plataforma_receitas.
SELECT d.id,
       COALESCE(e.entradas, 0) AS entradas, d.reserva_academia_centavos AS reserva,
       COALESCE(s.premios, 0) AS premios, COALESCE(s.instrutor, 0) AS instrutor,
       COALESCE(s.academia, 0) AS academia, COALESCE(r.taxa, 0) AS taxa
FROM desafios d
LEFT JOIN (SELECT id_desafio, -SUM(valor_centavos) AS entradas FROM transacoes
            WHERE tipo IN ('entrada_desafio', 'estorno_desafio') AND status IN ('received','concluido') GROUP BY id_desafio) e ON e.id_desafio = d.id
LEFT JOIN (SELECT id_desafio,
                  SUM(CASE WHEN tipo = 'premio_desafio' THEN valor_centavos END) AS premios,
                  SUM(CASE WHEN tipo = 'pagamento_instrutor' THEN valor_centavos END) AS instrutor,
                  SUM(CASE WHEN tipo = 'receita_academia' THEN valor_centavos END) AS academia
             FROM transacoes WHERE status IN ('received','concluido') GROUP BY id_desafio) s ON s.id_desafio = d.id
LEFT JOIN (SELECT id_desafio, SUM(valor_centavos) AS taxa FROM plataforma_receitas
            WHERE origem = 'taxa_inscricao' GROUP BY id_desafio) r ON r.id_desafio = d.id
WHERE d.status = 'encerrado'
  AND COALESCE(e.entradas, 0) + d.reserva_academia_centavos
      <> COALESCE(s.premios, 0) + COALESCE(s.instrutor, 0) + COALESCE(s.academia, 0) + COALESCE(r.taxa, 0);

-- 2) Desafio não cancelado: a reserva vigente da coluna é o que o ledger tem líquido (reserva_desafio - estorno_reserva).
SELECT d.id, d.reserva_academia_centavos AS reserva_coluna, -COALESCE(SUM(t.valor_centavos), 0) AS reserva_ledger
FROM desafios d
LEFT JOIN transacoes t ON t.id_desafio = d.id AND t.tipo IN ('reserva_desafio', 'estorno_reserva') AND t.status IN ('received','concluido')
WHERE d.status <> 'cancelado'
GROUP BY d.id, d.reserva_academia_centavos
HAVING reserva_coluna <> reserva_ledger;

-- 3) Cancelado: tudo voltou (entradas, reserva) e não há taxa -> o ledger do desafio soma zero.
SELECT d.id, SUM(t.valor_centavos) AS liquido
FROM desafios d JOIN transacoes t ON t.id_desafio = d.id AND t.status IN ('received','concluido')
WHERE d.status = 'cancelado'
GROUP BY d.id HAVING liquido <> 0;
```

A invariante saldo == ledger (acima) continua valendo com os tipos novos. A tarifa de saque (`tarifa_saque`) entra no
ledger como débito `concluido` e fica `failed` (fora do ledger efetivo, com a linha de `plataforma_receitas` removida) se o
PIX do saque falhar.
- Amizades duplicadas (inclusive em direções opostas): mantém uma linha por par, prioridade `aceita > pendente > recusada`, depois menor `id`.
  Autoamizades são removidas.

## Como rodar com a CLI do goose

```bash
# DSN: usuario:senha@tcp(host:porta)/banco?parseTime=true
export DSN='usuario:senha@tcp(127.0.0.1:3306)/gymscore?parseTime=true'
cd gymScore-backend/database/migrations   # (ou use -dir)

go run github.com/pressly/goose/v3/cmd/goose@latest mysql "$DSN" status
go run github.com/pressly/goose/v3/cmd/goose@latest mysql "$DSN" up
go run github.com/pressly/goose/v3/cmd/goose@latest mysql "$DSN" down        # desfaz a última
go run github.com/pressly/goose/v3/cmd/goose@latest mysql "$DSN" down-to 0   # CUIDADO: apaga tudo
```

No boot do backend as migrations embutidas rodam automaticamente quando `MIGRATE_ON_BOOT=true` (padrão).

## Testando localmente (MySQL descartável)

```bash
docker run -d --name gymscore-mysql-test -e MYSQL_ROOT_PASSWORD=test -e MYSQL_DATABASE=gymscore -p 3307:3306 mysql:8
DSN='root:test@tcp(127.0.0.1:3307)/gymscore?parseTime=true'
# ... goose up / down-to 0 / up
docker rm -f gymscore-mysql-test
```

## Observações

- MySQL não tem DDL transacional: se uma migration falhar no meio, ela fica parcialmente aplicada. **Faça backup
  (`mysqldump`) antes de aplicar em bancos com dados.**
- O `Down` da 11 **não desfaz** os cancelamentos/estornos dos legados (o dinheiro já voltou; desfazer quebraria a invariante) e perde dono/instrutor/data/categoria/pódio configurado, ingressos e prêmios do pódio.
- Os `Down` de 2, 3, 4 e 10 são reversões com perda de informação (ex.: desafios cancelados voltam como `encerrado`,
  vencedores múltiplos viram um só; o `Down` da 10 falha se já existir conta "academia" sem cpf/data_nascimento/genero,
  pois o `MODIFY COLUMN` final exige `NOT NULL`). O `Down` da 1 apaga todas as tabelas.
- A coluna `amizades.par` é gerada (STORED); os models Go não devem mapeá-la para escrita (usar `<-:false` ou omitir).
- A 00010 lida com um banco de dev que já teve `academias`/`instrutores`/`podio_desafios`/`avaliacoes_desafios` e as
  colunas de perfil/gamificação criadas por um `AutoMigrate` legado (antes de existir migration SQL para elas), em
  nulidade/tipos mais soltos que o schema final. Por isso ela usa duas stored procedures temporárias (criadas e
  removidas dentro do próprio Up/Down) para fazer `ADD COLUMN`/`ADD INDEX` condicionais via `information_schema` +
  `PREPARE`/`EXECUTE` — o MySQL não tem `ADD COLUMN IF NOT EXISTS` (diferente do MariaDB) — e depois normaliza
  nulidade/tipo/default com `UPDATE` + `MODIFY COLUMN` incondicional (idempotente por natureza). As tabelas novas
  usam `CREATE TABLE IF NOT EXISTS`, mesmo padrão da 00001.
- A 00011 usa duas procedures temporárias (`_goose_00011_add`/`_drop`, removidas no fim) para ADD/DROP condicional de coluna, índice, FK e CHECK (também tolera colunas pré-criadas por AutoMigrate legado); o estorno roda dentro da procedure `_goose_00011_estornar_legados` (transação atômica).
- A 00012 usa as procedures temporárias `_goose_00012_add`/`_drop`. Os CHECKs `chk_desafios_soma_pct` (`percentual_pote` + `taxa_plataforma_pct` + `instrutor_pct` <= 100) valem sempre, mesmo com `dividir_pote = 0` (nesse caso grave `percentual_pote = 0`). O `Down` da 12 apaga configuração financeira, candidaturas e receitas, e converte as linhas de ledger com tipos novos em `ajuste` (mesmo valor), preservando saldo == ledger; o dinheiro movimentado não é desfeito.
- Rodada 3 (2026-11-07): o RDS de testes tinha a versão 13 registrada por uma sprint de moderação cujo código se
  perdeu. Ela foi reconstruída como `00013_moderacao.sql` (mesmo número: no RDS é pulada; num banco novo recria o
  schema) e as nossas 00013–00017 viraram 00014–00018. Prova de aceite: o clone do RDS (dump restaurado) sobe até a
  19 pelo `database.Migrar` — teste `TestMigracao_CloneDoRDS` (`RDS_CLONE_DB=<banco restaurado>`). Na rodada 4 o mesmo
  teste sobe o clone até a 20.
- Contas de teste: `go run . seed-teste` (nunca em produção) só insere/atualiza dados pelas tabelas existentes e pelo
  ledger (`ajuste`); não é migration. Ver o README da raiz.
- Para criar nova migration: `goose create nome sql -dir migrations` (numeração sequencial `0000N_nome.sql`).

## Problemas comuns ao migrar no RDS

O erro do goose diz a versão e o código do MySQL: `partial migration error (type:sql,version:NN): Error XXXX ...`.
Depois de uma falha a migration fica **parcial** (DDL do MySQL não é transacional), mas as 00011+ são reaplicáveis
(ADD/DROP condicionais): corrija a causa e suba de novo. Ordem de diagnóstico sugerida:
`SELECT @@version, @@gtid_mode, @@binlog_format, @@sql_mode;`, `SHOW GRANTS;`, `SELECT * FROM goose_db_version ORDER BY id;`
e as consultas abaixo.

| Erro | Causa | Como detectar | Como corrigir |
|---|---|---|---|
| `Error 1304 PROCEDURE _goose_000NN_add already exists` / `Error 1305 PROCEDURE ... does not exist` | Duas instâncias migrando ao mesmo tempo (2+ réplicas/rolling deploy com `MIGRATE_ON_BOOT=true`) | log de boot de mais de uma instância no mesmo minuto | Desde o R2 o `database.Migrar` usa a trava do goose (tabela `goose_lock`, lease de 30 s renovado por heartbeat; a outra instância espera até 10 min). Em versões antigas: reiniciar a instância que falhou (reaplicar é seguro) ou `MIGRATE_ON_BOOT=false` + migrar num passo único do deploy. Se uma instância morreu segurando a trava, a próxima assume depois que o lease expira. |
| `version:16` `Error 1644 (45000): 00016: ... credito de encerramento duplicado` | Prêmio/pagamento/receita repetido por (desafio, usuário, tipo) — pódio antigo sem UNIQUE | `SELECT id_desafio,id_usuario,tipo,COUNT(*) FROM transacoes WHERE tipo IN ('premio_desafio','pagamento_instrutor','receita_academia') AND id_desafio IS NOT NULL GROUP BY 1,2,3 HAVING COUNT(*)>1;` | Corrigir pelo ledger: `ajuste` negativo do excedente + `UPDATE usuarios` do saldo na mesma transação (ou estornar a linha duplicada com contrapartida); subir de novo |
| `version:16` `Error 1644 (45000): 00016: ... sinal incoerente com o tipo` | `deposito`/`estorno`/`premio` com valor 0, `saque` positivo, ou `tipo=''` (ENUM inválido gravado com `sql_mode` não estrito) | `SELECT * FROM transacoes WHERE NOT ((tipo IN ('deposito','estorno_desafio','premio_desafio','estorno_reserva','pagamento_instrutor','receita_academia') AND valor_centavos>0) OR (tipo IN ('saque','entrada_desafio','reserva_desafio','tarifa_saque') AND valor_centavos<0) OR tipo='ajuste');` | Linhas de valor 0 sem efeito no saldo: apagar ou virar `ajuste`; sinal trocado: corrigir com `ajuste`; `tipo=''`: descobrir a origem e reclassificar |
| `version:16` `Error 1644 (45000): 00016: ... tarifa_saque duplicada para o mesmo saque` (antes do R3: `Error 1062 ... uq_transacoes_origem_tipo`) | Duas tarifas com a mesma descrição "Tarifa do saque #id" | `SELECT descricao,id_usuario,COUNT(*),GROUP_CONCAT(id) FROM transacoes WHERE tipo='tarifa_saque' AND descricao REGEXP '^Tarifa do saque #[0-9]+$' GROUP BY 1,2 HAVING COUNT(*)>1;` | Estornar a tarifa extra pelo ledger e mudar a descrição dela (ex.: sufixo " - duplicada"); subir de novo |
| `version:11` (ou 17) `Error 3748 Statement violates GTID consistency: CREATE TEMPORARY TABLE ... inside a transaction` | `gtid_mode=ON` + `binlog_format=STATEMENT` no parameter group | `SELECT @@gtid_mode, @@enforce_gtid_consistency, @@binlog_format;` | `binlog_format=ROW` (ou MIXED, padrão do RDS 8.0) no parameter group; reiniciar e subir de novo |
| `detected N missing (out-of-order) migrations lower than database version` / `version:15` `Error 1824 Failed to open the referenced table 'administradores'` | `goose_db_version` com versões de outra branch (número maior que a última migration do repositório — hoje 20 —, ou 00013 aplicada sem as tabelas da moderação) | `SELECT * FROM goose_db_version ORDER BY id;` e `SHOW TABLES LIKE 'administradores';` | Conferir o schema, remover/renumerar as linhas órfãs do `goose_db_version` e reaplicar (nunca use `allow-missing` sem entender o que a outra versão fez) |
| `version:19` `Error 1644 (45000): 00019: moderacao tem acoes de admin gravadas com id de usuarios` | Banco onde o código da Sprint 7 gravou ações de admin apontando para `usuarios` | `SELECT * FROM punicoes WHERE revogada_por IS NOT NULL OR escopo='global'; SELECT * FROM recursos WHERE julgado_por IS NOT NULL;` | Converter manualmente para `administradores` (colunas `*_admin`) e subir de novo |
| `version:10+` `Error 1370 alter routine command denied` / `Error 1044` | Usuário do app sem privilégio de rotina (as migrations usam procedures temporárias desde a 00010) | `SHOW GRANTS;` com o usuário do app | `GRANT CREATE ROUTINE, ALTER ROUTINE, EXECUTE, CREATE TEMPORARY TABLES, REFERENCES ON gymscore.* TO 'app'@'%';` (além de SELECT/INSERT/UPDATE/DELETE/CREATE/ALTER/INDEX/DROP). Desde o R2 também é preciso poder criar a tabela `goose_lock` (CREATE) |
| `Error 1205 Lock wait timeout exceeded` ou boot travado | Versão antiga ainda servindo (rolling deploy): `ALTER TABLE transacoes` espera metadata lock | `SHOW PROCESSLIST` (`Waiting for table metadata lock`) | Parar as instâncias antigas antes de migrar, ou migrar em janela |

Não são causa (testado em 8.0.46 com GTID/sem SUPER e em 8.4): `log_bin_trust_function_creators=0` (as migrations
criam só PROCEDURES sem DEFINER explícito), `sql_require_primary_key=ON`, `sql_mode` estrito ou não.
