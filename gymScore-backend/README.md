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
- `APP_ENV=production` **exige** `JWT_SECRET` e `ADMIN_JWT_SECRET` fortes e diferentes (o app não sobe com vazio/padrão), ativa cookie `Secure` e desliga o stack trace do `recover`.
- `CORS_ORIGINS` (lista separada por vírgula). Vazio: `*` em desenvolvimento e CORS desligado em produção (mesma origem).
- `RATE_LIMIT_MAX` (padrão 10/min/IP) em login, cadastro, esqueci-senha e redefinir-senha; `0` desativa.
  O login do painel admin tem limite próprio, `ADMIN_LOGIN_RATE_LIMIT_MAX` (padrão 5/min/IP), que não desliga (`0` = 5).
- **Cookies de sessão** (o JWT nunca vem no corpo das respostas): usuário `jwt` (Path=/, HttpOnly, SameSite=Lax) e
  admin `admin_jwt` (Path=/api/admin, HttpOnly, SameSite=Strict) em desenvolvimento HTTP; com `APP_ENV=production` **ou**
  `APP_BASE_URL` https viram `__Host-jwt` e `__Secure-admin_jwt`, com `Secure`. `Expires` = `exp` do JWT; logout/troca/reset
  apagam com os mesmos atributos. Clientes que não são navegador leem o `Set-Cookie` e podem mandar o valor como `Authorization: Bearer`.
- **Migrations em várias réplicas**: `database.Migrar` usa a trava do goose (tabela `goose_lock`); réplicas que sobem
  juntas migram uma de cada vez. Erros comuns no RDS: ver `database/README.md`.
- `MIGRATE_ON_BOOT` (padrão `true`) aplica as migrations ao subir. **Faça backup antes de apontar para um banco com dados reais.**
- **Sessão**: o JWT carrega o claim `tv` (versão); logout, troca e reset de senha incrementam `usuarios.token_versao` e os tokens anteriores passam a dar 401 imediatamente.
- **Fuso**: a sessão MySQL é configurada com `time_zone` = offset do fuso do Go (`config.AplicarFusoDSN`), então `DEFAULT CURRENT_TIMESTAMP`/`NOW()` (inclusive nas migrations do boot) saem na hora local. Se o fuso tiver horário de verão, reinicie o processo na troca.
- `ENV_FILE` aponta outro arquivo `.env`; `UPLOADS_DIR` define a pasta das fotos de check-in.
- Recuperação de senha: com `SMTP_HOST` + `SMTP_FROM` envia e-mail (STARTTLS, porta 587); sem eles entra em **modo demo** e escreve o link no log do console.
- Asaas: `ASAAS_ENV=sandbox|prod` escolhe URL e chave. `ASAAS_WEBHOOK_TOKEN` vazio **desativa** o webhook (404); os pagamentos são confirmados por polling.

## Painel admin

Admins ficam na tabela própria `administradores` (papéis `superadmin` e `admin`), com autenticação separada da dos usuários:

- `ADMIN_JWT_SECRET` (obrigatório em produção, >= 32 caracteres e diferente de `JWT_SECRET`) assina os tokens do painel; claim `aud=gymscore-admin` e `tv` = `administradores.token_versao`. Token de usuário nunca vale em `/api/admin` e vice-versa.
- `ADMIN_TOKEN_TTL` (padrão `8h`, máx. `24h`): validade do token e do cookie `admin_jwt` (HttpOnly, SameSite=Strict, `Secure` em produção, Path `/api/admin`).
- Requisições não-GET em `/api/admin` sem header `Authorization` exigem `Origin`/`Referer` da própria origem, `APP_BASE_URL` ou `CORS_ORIGINS`.
- Toda ação de escrita grava `admin_auditoria` na mesma transação, com `motivo` obrigatório.
- Moderação: `GET /denuncias`, `GET /denuncias/:id`, `GET /denuncias/:id/evidencias/:ordem`, `POST /denuncias/:id/{assumir,decidir}`, `GET|POST /punicoes`, `POST /punicoes/:id/revogar`, `GET /recursos`, `POST /recursos/:id/julgar`, `GET /moderacao/eventos`, `GET /pagamentos-pendentes`, `GET /debitos`, `POST /desafios/:id/anular-resultado` (superadmin). `POST /usuarios/:id/bloquear` aplica um **banimento** (punição) e `/desbloquear` revoga os banimentos vigentes.
- Páginas: `/admin/login` e `/admin` (arquivos em `gymScore-frontend/public/admin/`), servidas com CSP restritiva, `X-Frame-Options: DENY` e `Cache-Control: no-store`.

Primeiro superadmin (e reset de senha) pela CLI, que aplica as migrations pendentes e sai sem subir o servidor. A senha é lida do terminal sem eco (duas vezes, mínimo 12 caracteres); nunca por argumento. Sem TTY (scripts/testes) ela é lida do stdin, uma por linha:

```bash
go run . admin criar-superadmin --email admin@exemplo.com --nome "Fulano"
go run . admin resetar-senha --email admin@exemplo.com   # também revoga os tokens do admin
```

Os demais admins são criados no painel por um superadmin (senha temporária exibida uma vez).

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

Só a **academia** cria o desafio e designa um **instrutor** vinculado a ela. O **atleta** paga a entrada ao se inscrever (e recebe um ingresso com QR). Pote = soma das entradas.

`aberto` -> `em_andamento` -> `encerrado` (ou `cancelado`)

| Status | O que acontece |
|---|---|
| `aberto` | Inscrições até `vagas` (só atleta; a entrada sai da carteira na hora, 0 = gratuito). O atleta pode **sair** (reembolso). A academia pode **editar** (não altera a entrada com inscritos) e **cancelar** (reembolso integral). Instrutor/academia podem marcar presença no dia do evento. |
| `em_andamento` | Iniciado pelo instrutor designado ou pela academia dona (>= 1 inscrito; fecha as inscrições). Presença por `codigo_ingresso` (QR) ou `id_usuario`. A academia ainda pode cancelar (estorno a todos). |
| `encerrado` | O pódio foi lançado: `min(posicoes_podio, presentes)` posições, só presentes, marca obrigatória. O pote é dividido por tabela (1: 100; 2: 70/30; 3: 60/30/10; 4: 50/25/15/10; 5: 40/25/15/12/8), com resto de centavos para o 1º e percentuais renormalizados se houver menos presentes que posições. Os créditos do pódio (prêmios, instrutor, academia, taxa) viram `pagamentos_pendentes` e só entram nos saldos depois de `PREMIO_JANELA_CONTESTACAO_HORAS` (job `liberar-pagamentos`, via `CarteiraRepository`); denúncia de desafio aberta retém; pontos: 1º +30, 2º-5º +15, demais presentes +5. |
| `cancelado` | Entradas reembolsadas (ledger `estorno_desafio`). |

Garantias: transições com `UPDATE ... WHERE status = <esperado>` + `SELECT ... FOR UPDATE` na linha do desafio (entradas concorrentes não estouram as vagas nem cobram duas vezes).

## Endpoints

Prefixo `/api`. Resposta: `{"success":true,"message":"...","data":...}` ou `{"success":false,"error":"..."}`. Auth: `Authorization: Bearer <jwt>` ou cookie de sessão (`jwt` em dev HTTP, `__Host-jwt` com Secure). **A identidade vem sempre do token**; nenhum corpo aceita o id de quem age. A documentação completa (corpos e respostas) está no Swagger.

| Grupo | Rotas |
|---|---|
| Auth | `POST /login`, `POST /logout`, `POST /usuarios` (cadastro), `POST /auth/esqueci-senha`, `POST /auth/redefinir-senha` |
| Usuários | `GET /usuarios/me`, `GET /usuarios/busca?q=`, `GET /usuarios/:id` (público), `PATCH /usuarios/perfil`, `PATCH /usuarios/senha` |
| Amigos | `GET /amigos`, `POST /amigos/{adicionar,aceitar,recusar,remover}` (`{id_amigo}`) |
| Desafios | `GET /desafios` (feed), `GET /desafios/meus`, `GET /desafios/categorias`, `GET /desafios/:id`, `POST /desafios` (só academia), `PATCH /desafios/:id`, `POST /desafios/:id/{entrar,sair,cancelar,iniciar,presenca,podio,avaliar,convidar}`, `GET /desafios/:id/{presentes,podio}` |
| Carteira | `GET /carteira` (+ `a_liberar_centavos`, `retido_centavos`, `debito_aberto_centavos`, `saque_bloqueado`), `GET /carteira/extrato`, `GET /carteira/pendentes`, `GET /carteira/debitos`, `POST /carteira/saque` (409 `debito_aberto` com débito), `GET /carteira/saque/:id` |
| PIX | `POST /pagamento/pix`, `GET /pagamento/pix/:asaas_id`, `POST /pagamento/pix/:asaas_id/simular` (sandbox), `POST /webhooks/asaas` |
| Notificações | `GET /notificacoes`, `GET /notificacoes/nao-lidas`, `POST /notificacoes/:id/lida`, `POST /notificacoes/lidas` |
| Ranking | `GET /ranking/amigos?dias=30` |
| Treinos | `GET\|POST /treinos`, `GET /treinos/radar`, `POST /treinos/{importar,concluir}`, `GET\|DELETE /treinos/:id` |
| Moderação (usuário) | `POST /denuncias`, `POST /denuncias/:id/evidencias` (multipart `arquivo`), `GET /denuncias/minhas`, `GET /denuncias/:id`, `GET /denuncias/:id/evidencias/:ordem`, `GET /punicoes/minhas`, `POST /punicoes/:id/recurso`, `POST /recursos/banimento` (público, banido — sem sessão) |
| Moderação (academia) | `GET /academia/denuncias`, `GET /academia/denuncias/:id`, `GET /academia/denuncias/:id/evidencias/:ordem`, `POST /academia/denuncias/:id/{assumir,encaminhar,decidir}`, `GET /academia/punicoes` |

**Moderação** (`analise/moderacao-spec.md`): denúncia sigilosa (o denunciante só aparece para o admin); suborno de instrutor em desafio de academia é julgado pela academia até `DENUNCIA_PRAZO_ACADEMIA_HORAS` (depois o job `escalonar-denuncias` sobe ao admin); punições advertência/suspensão/banimento/bloqueio na academia; o estado da conta (`usuarios.banido_em`/`suspenso_ate`) é sempre recalculado das punições vigentes. Banido: login e tokens recusados com `403 {"codigo":"conta_banida"}` e recurso por `POST /api/recursos/banimento`. Suspenso: faz login e lê, mas ações de participação/gestão dão `403 {"codigo":"conta_suspensa","data":{"suspenso_ate":...}}`. Evidências ficam em `EVIDENCIAS_DIR` (privada). Pódio com janela de contestação: créditos ficam "a liberar" até `PREMIO_JANELA_CONTESTACAO_HORAS`; a anulação do resultado (superadmin) cancela os não liberados, estorna os liberados (o que faltar vira débito, que bloqueia o saque e é abatido por qualquer crédito seguinte) e relança o pódio (academia dona) ou devolve as inscrições integrais (com estorno da taxa já liberada).

Fotos de check-in antigas continuam servidas em `GET /uploads/checkins/<nome>` (histórico); o check-in do atleta foi removido no Sprint 6 e não há mais upload.

## Testes

```bash
go build ./... && go vet ./...
go test ./...                                  # unitários (sem banco)
```

Os de integração usam **MySQL 8 real e descartável** (nunca aponte para um banco com dados): recriam o banco `gymscore_it` (ou o de `TEST_DB_NAME`, para duas execuções simultâneas não se atropelarem) a cada execução.

```bash
docker run -d --name gymscore-mysql-test -e MYSQL_ROOT_PASSWORD=testpw -p 3307:3306 mysql:8
export TEST_DB_DSN='root:testpw@tcp(127.0.0.1:3307)/?parseTime=True&loc=Local'
go test -tags integration ./...                # inclui concorrência (inscrições, débitos, saques, pódio duplicado)
go test -tags integration -race ./internal/integration/   # mais lento (~5 min)
docker rm -f gymscore-mysql-test
```

Sem `TEST_DB_DSN` os testes de integração falham de propósito (nada é ignorado em silêncio). Dica: horários são sempre gerados pelo Go; não misture `NOW()` do MySQL (fuso do container) com horários do Go nos testes.

## Swagger

```bash
go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g main.go -o docs --parseInternal
```
