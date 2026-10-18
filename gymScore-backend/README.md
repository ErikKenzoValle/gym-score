# GymScore — Backend (Go)

API do **GymScore**: desafios de treino com entrada em dinheiro e prêmio pelo voto dos participantes, amigos, ranking entre amigos, treinos compartilháveis, carteira (depósito e saque via PIX/Asaas) e notificações in-app. Projeto de TCC (Asaas em sandbox).

Stack: **Go 1.26 + Fiber v2 + GORM + MySQL 8**, JWT + bcrypt, migrations **goose** embutidas, Swagger em `/swagger/` (somente fora de produção).

```
main.go                  # config -> banco -> migrations -> servidor
database/                # embed.go (goose) + migrations/*.sql (versionadas)
internal/
  app/                   # monta o Fiber (DI + middlewares + rotas); usado também pelos testes
  config/ client/        # variáveis de ambiente; cliente Asaas
  controllers/ services/ repositories/ models/ middlewares/ routes/
  integration/           # testes de integração (build tag `integration`)
pkg/utils/               # JWT, validação, helpers
docs/                    # swagger gerado (swag init)
```

Camadas: **controller -> service -> repository**. Erros de negócio (`services.ErroNegocio`) mostram a mensagem ao cliente; qualquer outro erro vai só para o log e o cliente recebe "Erro interno do servidor".

## Configuração e execução

```bash
cp .env.example .env      # edite; todas as variáveis estão documentadas no arquivo
go run main.go            # ou: air
```

A API sobe em `http://localhost:3000` (frontend estático servido pelo mesmo servidor; Swagger em `/swagger/`).

Pontos de atenção:
- `APP_ENV=production` **exige** `JWT_SECRET` forte (o app não sobe com vazio/padrão), ativa cookie `Secure` e desliga o stack trace do `recover`.
- `CORS_ORIGINS` (lista separada por vírgula). Vazio: `*` em desenvolvimento e CORS desligado em produção (mesma origem).
- `RATE_LIMIT_MAX` (padrão 10/min/IP) em login, cadastro, esqueci-senha e redefinir-senha; `0` desativa.
- `MIGRATE_ON_BOOT` (padrão `true`) aplica as migrations ao subir. **Faça backup antes de apontar para um banco com dados reais.**
- **Sessão**: o JWT carrega o claim `tv` (versão); logout, troca e reset de senha incrementam `usuarios.token_versao` e os tokens anteriores passam a dar 401 imediatamente.
- **Fuso**: a sessão MySQL é configurada com `time_zone` = offset do fuso do Go (`config.AplicarFusoDSN`), então `DEFAULT CURRENT_TIMESTAMP`/`NOW()` (inclusive nas migrations do boot) saem na hora local. Se o fuso tiver horário de verão, reinicie o processo na troca.
- `ENV_FILE` aponta outro arquivo `.env`; `UPLOADS_DIR` define a pasta das fotos de check-in.
- Recuperação de senha: com `SMTP_HOST` + `SMTP_FROM` envia e-mail (STARTTLS, porta 587); sem eles entra em **modo demo** e escreve o link no log do console.
- Asaas: `ASAAS_ENV=sandbox|prod` escolhe URL e chave. `ASAAS_WEBHOOK_TOKEN` vazio **desativa** o webhook (404); os pagamentos são confirmados por polling.

## Migrations (goose)

Arquivos em `database/migrations/` são embutidos no binário (`database.Migrar`). `AutoMigrate` não é mais usado. Para rodar à mão com a CLI, veja `database/README.md`:

```bash
go run github.com/pressly/goose/v3/cmd/goose@latest -dir database/migrations mysql "$DSN" status
```

## Dinheiro e carteira

- Tudo em **centavos inteiros** (`saldo_centavos`, `valor_centavos`); os campos em reais (`saldo`, `valor`, `pote`...) existem só para exibição.
- **Toda** movimentação de saldo passa pelo `CarteiraRepository`: `UPDATE usuarios SET saldo_centavos = saldo_centavos ± ?` (débito com `WHERE saldo_centavos >= ?`) e uma linha no ledger `transacoes` na mesma transação. Tipos: `deposito`, `saque`, `entrada_desafio`, `estorno_desafio`, `premio_desafio`.
- **Invariante**: saldo == soma do ledger (linhas `received`/`concluido` + saques `pending`).
- **Depósito PIX**: gera cobrança (`pending`); a confirmação por polling (`GET /api/pagamento/pix/:id`) ou webhook faz `pending -> received` e credita na mesma transação (idempotente). Em sandbox há `POST .../simular`.
- **Saque PIX** (`POST /api/carteira/saque`, mín. R$ 5,00, máx. R$ 10.000,00): debita **antes** de chamar `POST /v3/transfers`; se o Asaas recusar, reembolsa na hora; depois o `GET /api/carteira/saque/:id` consulta o status (`DONE` conclui; `FAILED`/`CANCELLED` reembolsam). O reembolso é idempotente. Saque `pending` sem transferência criada há mais de 5 min é reembolsado pelo polling.

## Modelo do desafio

Cada participante **paga a entrada ao entrar** (inclusive o criador, ao criar). Pote = soma das entradas.

`aberto` -> `em_andamento` -> `em_votacao` -> `encerrado` (ou `cancelado`)

| Status | O que acontece |
|---|---|
| `aberto` | Inscrições até `vagas`. Criador convida amigos e pode **cancelar** (reembolso integral). Participante não criador pode **sair** (reembolso). |
| `em_andamento` | Iniciado pelo criador (>= 2 participantes). Participantes fazem **check-ins** (1 por dia, texto/foto/localização opcionais). |
| `em_votacao` | Aberta pelo criador. Cada participante vota em **outro** participante (1 voto). Quando todos votam, apura sozinho. Criador pode **apurar** antes com metade+1 dos votos, ou após 24h da abertura, ou após `data_encerramento`. |
| `encerrado` | Mais votado leva o pote; empate divide igualmente (resto de centavos para o menor id); sem votos todos recebem a entrada de volta. |
| `cancelado` | Entradas reembolsadas. |

Garantias: transições com `UPDATE ... WHERE status = <esperado>` + `SELECT ... FOR UPDATE` na linha do desafio (entradas concorrentes não estouram as vagas nem cobram duas vezes).

## Endpoints

Prefixo `/api`. Resposta: `{"success":true,"message":"...","data":...}` ou `{"success":false,"error":"..."}`. Auth: `Authorization: Bearer <jwt>` ou cookie `jwt`. **A identidade vem sempre do token**; nenhum corpo aceita o id de quem age. A documentação completa (corpos e respostas) está no Swagger.

| Grupo | Rotas |
|---|---|
| Auth | `POST /login`, `POST /logout`, `POST /usuarios` (cadastro), `POST /auth/esqueci-senha`, `POST /auth/redefinir-senha` |
| Usuários | `GET /usuarios/me`, `GET /usuarios/busca?q=`, `GET /usuarios/:id` (público), `PATCH /usuarios/perfil`, `PATCH /usuarios/senha` |
| Amigos | `GET /amigos`, `POST /amigos/{adicionar,aceitar,recusar,remover}` (`{id_amigo}`) |
| Desafios | `GET /desafios`, `GET /desafios/meus`, `GET /desafios/:id`, `POST /desafios`, `POST /desafios/:id/{entrar,sair,cancelar,iniciar,abrir-votacao,votar,apurar,convidar}`, `GET\|POST /desafios/:id/checkins` |
| Carteira | `GET /carteira`, `GET /carteira/extrato`, `POST /carteira/saque`, `GET /carteira/saque/:id` |
| PIX | `POST /pagamento/pix`, `GET /pagamento/pix/:asaas_id`, `POST /pagamento/pix/:asaas_id/simular` (sandbox), `POST /webhooks/asaas` |
| Notificações | `GET /notificacoes`, `GET /notificacoes/nao-lidas`, `POST /notificacoes/:id/lida`, `POST /notificacoes/lidas` |
| Ranking | `GET /ranking/amigos?dias=30` |
| Treinos | `GET\|POST /treinos`, `GET /treinos/radar`, `POST /treinos/{importar,concluir}`, `GET\|DELETE /treinos/:id` |

Fotos de check-in ficam em `UPLOADS_DIR/checkins/` com nome aleatório e são servidas em `GET /uploads/checkins/<nome>` (validação por magic bytes; JPG/PNG/WEBP; até 3 MB).

## Testes

```bash
go build ./... && go vet ./...
go test ./...                                  # unitários (sem banco)
```

Os de integração usam **MySQL 8 real e descartável** (nunca aponte para um banco com dados): recriam o banco `gymscore_it` a cada execução.

```bash
docker run -d --name gymscore-mysql-test -e MYSQL_ROOT_PASSWORD=testpw -p 3307:3306 mysql:8
export TEST_DB_DSN='root:testpw@tcp(127.0.0.1:3307)/?parseTime=True&loc=Local'
go test -tags integration ./...                # inclui concorrência (entradas, débitos, saques, check-ins)
go test -tags integration -race ./internal/integration/   # mais lento (~5 min)
docker rm -f gymscore-mysql-test
```

Sem `TEST_DB_DSN` os testes de integração falham de propósito (nada é ignorado em silêncio). Dica: horários são sempre gerados pelo Go; não misture `NOW()` do MySQL (fuso do container) com horários do Go nos testes.

## Swagger

```bash
go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g main.go -o docs --parseInternal
```
