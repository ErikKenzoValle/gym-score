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

Regras de conversão importantes:
- Saldos: `ROUND(saldo * 100)`, nunca negativo, `NULL` vira 0. Saldo legado negativo vira 0 e é registrado numa linha `ajuste` (descrição com o valor original).
- Ledger de abertura: para cada usuário com saldo > 0 (ou saldo legado negativo, ou ledger divergente) é criada uma linha `ajuste`/`received`, "Saldo migrado", com `valor_centavos = saldo_centavos - SUM(depósitos legados received)`, de modo que `saldo == SUM(ledger)` vale também para dados antigos.
- Transações antigas: viram `tipo='deposito'`, `valor_centavos = ROUND(valor*100)`, status preservado.
- Desafios antigos **não encerrados** (`pendente`, `aberto`, `em_andamento`) viram `cancelado` **sem movimentar saldo**
  (no modelo antigo não havia reserva). Encerrados continuam `encerrado`; o `id_vencedor` vira participante com `vencedor=1`
  e `premio_centavos = valor_entrada_centavos`.
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
- Para criar nova migration: `goose create nome sql -dir migrations` (numeração sequencial `0000N_nome.sql`).
