# GymScore

**GymScore** é uma plataforma de **desafios presenciais de treino com prêmio em dinheiro**. Academias criam
desafios (corrida, crossfit, musculação…), atletas pagam a inscrição pela carteira do app, um instrutor
confere a presença no dia e lança o pódio, e os prêmios são distribuídos depois de uma janela de contestação.
Em volta disso: amigos, ranking entre amigos, treinos compartilháveis, notificações, moderação (denúncias,
punições e recursos) e um painel administrativo.

Projeto de TCC. O PIX usa o gateway **Asaas** (sandbox nos testes).

---

## Sumário

- [Arquitetura](#arquitetura)
- [Contas e papéis](#contas-e-papéis)
- [Fluxos principais](#fluxos-principais)
- [Segurança](#segurança)
- [Como rodar localmente](#como-rodar-localmente)
- [Contas de teste](#contas-de-teste)
- [Variáveis de ambiente](#variáveis-de-ambiente)
- [Migrations](#migrations)
- [Testes e CI](#testes-e-ci)
- [Comandos de linha de comando](#comandos-de-linha-de-comando)
- [Documentação complementar](#documentação-complementar)

---

## Arquitetura

| Pasta | O que é | Stack |
|---|---|---|
| [`gymScore-backend/`](./gymScore-backend) | API REST, regras de negócio, jobs, CLI | Go 1.26, Fiber v2, GORM, MySQL 8, goose (migrations embutidas) |
| [`gymScore-frontend/`](./gymScore-frontend) | App web (atleta, instrutor, academia) e painel admin | HTML, CSS e JavaScript puro, sem build |

- **Um processo só:** o backend serve a API em `/api/*`, o app em `/` (arquivos de `FRONTEND_PATH`) e o painel em
  `/admin`. Swagger em `/swagger/` fora de produção; `GET /health` para checagem.
- **Camadas:** controller → service → repository. Erros de negócio têm status e mensagem próprios; qualquer outro erro
  vai só para o log e o cliente recebe "Erro interno do servidor". Respostas no envelope
  `{"success", "message", "data"}` ou `{"success": false, "error", "codigo"?, "data"?}`.
- **Dinheiro** sempre em centavos inteiros. Toda mudança de saldo passa pelo `CarteiraRepository`: `UPDATE` atômico do
  saldo + linha no ledger `transacoes`, na mesma transação. Invariante: saldo de cada usuário = soma do ledger efetivo.
- **Jobs periódicos** (no mesmo processo, intervalo `JOBS_INTERVALO`, seguros com várias instâncias): cancelar desafios
  vencidos, reconciliar depósitos e saques PIX pendentes, liberar pagamentos do pódio, escalonar denúncias vencidas,
  encerrar suspensões.
- **Admins** ficam numa tabela própria (`administradores`), com login, segredo JWT e cookie separados dos usuários.

```
gymScore-backend/
  main.go              config -> banco -> migrations -> servidor (ou CLI: admin / seed-teste)
  database/            migrations goose embutidas no binário
  internal/            app, cli, client (Asaas, Anthropic), config, controllers, services,
                       repositories, models, middlewares, routes, jobs, integration (testes)
gymScore-frontend/public/
  html/ js/ css/       app (telas de atleta, instrutor e academia)
  admin/               painel admin (CSP sem 'unsafe-inline')
```

---

## Contas e papéis

| Conta | Como nasce | O que faz |
|---|---|---|
| **Atleta** | Cadastro público (`/signup`) | Entra em desafios, carteira, amigos, treinos, denúncias. |
| **Instrutor** | Não é outra conta: é o **segundo modo** da conta de atleta. O atleta pede (`/cadastro-instrutor`, CREF) e um admin aprova. | No modo instrutor: agenda, vagas abertas (lances), presença e pódio dos desafios em que foi designado, convites de academias. |
| **Academia** | Cadastro público próprio (`/signup-academia`, CNPJ e endereço) — nasce **pendente** | Depois de aprovada: cria e gerencia desafios, convida instrutores, julga denúncias da sua jurisdição. |
| **Admin** | Tabela `administradores`; o 1º superadmin pela CLI, os demais pelo painel | Painel `/admin`: aprovações, finanças, saques, usuários, desafios, moderação, auditoria. Papéis `superadmin` e `admin`. |

**Modo atleta × instrutor (rodada 4).** Um login, uma senha. A conta de atleta sempre tem o modo `atleta`; ganha o
modo `instrutor` quando o pedido está `aprovada`. O seletor "Usar como: Atleta | Instrutor" no topo do app chama
`POST /api/sessao/modo`, que reemite o cookie de sessão com o claim `modo` (sem deslogar). A cada requisição o backend
reconfere no banco se o modo ainda é permitido: rotas de instrutor exigem modo instrutor + pedido aprovado (403
`modo_indisponivel` se o pedido foi recusado/revogado), ações de atleta exigem modo atleta (403 `modo_incorreto`). Pedido
pendente ou recusado não muda nada para o atleta (o app mostra "em análise"/"recusado"). O login entra no último modo
usado neste navegador (cookie de preferência), ou em `atleta`; na tela de login, as abas Atleta/Instrutor também escolhem o modo.

**Conflito de interesse.** O instrutor aprovado continua competindo como atleta, **exceto onde trabalha**: não se
inscreve em desafio em que é o instrutor designado, em desafio da academia a que está vinculado, nem onde tem candidatura
ativa à vaga de instrutor; e não pode ser designado/se candidatar em desafio em que está inscrito. As duas direções são
garantidas com trava de linha (409). O detalhe e o feed de desafios já avisam: `pode_entrar: false` +
`motivo_bloqueio_entrada` com a mesma mensagem do 409.

---

## Fluxos principais

### Academia e instrutor
1. A academia se cadastra com CNPJ e **endereço estruturado** (CEP, logradouro, número, complemento, bairro, cidade, UF).
   Ao digitar o CEP o app consulta `GET /api/cep/:cep` (proxy do ViaCEP com timeout de 3 s, cache e rate limit) e preenche
   os campos — tudo continua editável e o cadastro nunca depende do ViaCEP (404/503 liberam o preenchimento manual).
   O backend monta o texto de `academias.endereco`. Academias antigas mantêm só o texto até editarem (`/alterar-perfil`).
2. Um admin aprova (ou rejeita, com motivo) academias e pedidos de instrutor. Pendentes não operam (403 com `status_cadastro`).
3. A academia **convida** um instrutor pelo e-mail; o vínculo só existe quando ele aceita. O instrutor pode recusar ou
   se desvincular depois (409 se for designado em desafio ativo).

### Desafios
- Só a academia aprovada cria desafios: data do evento, vagas, entrada (0 a R$ 10.000), posições do pódio (1–5),
  prêmio fixo e/ou percentual do pote, pagamento do instrutor (fixo, percentual ou ambos) e instrutor **designado** ou
  **vaga aberta** (instrutores dão lances até um teto e a academia escolhe). Prêmio fixo e pagamento fixo do instrutor
  ficam **reservados** do saldo da academia na criação.
- O atleta entra pagando a inscrição pela carteira e recebe um ingresso com QR; pode sair enquanto está `aberto` (reembolso).
- No dia, a academia ou o instrutor designado inicia o desafio, marca presença (QR ou lista; fica registrado quem marcou
  e como) e lança o pódio (só presentes elegíveis, marca obrigatória). Ciclo: `aberto` → `em_andamento` → `encerrado`
  (ou `cancelado`, com estorno integral das entradas e devolução da reserva).
- Desafio vencido sem pódio (data do evento + `DESAFIO_CARENCIA`) é cancelado pelo job, com os mesmos estornos.
- **Pódio com janela de contestação:** os créditos do pódio (prêmios, instrutor, academia e a taxa da plataforma) viram
  `pagamentos_pendentes` "a liberar" e só entram nos saldos depois de `PREMIO_JANELA_CONTESTACAO_HORAS` (job
  `liberar-pagamentos`). Uma denúncia ligada ao desafio aberta nesse período **retém** os pagamentos até ser julgada.
- **Anulação do resultado** (superadmin): cancela o que não foi liberado, estorna o que foi (o que faltar no saldo vira
  débito) e, conforme o modo, devolve o desafio para a academia **relançar o pódio** ou **devolve as inscrições
  integralmente** (com estorno da taxa já liberada).

### Carteira, PIX, saque e débitos
- **Depósito PIX:** a linha `pending` é criada antes da cobrança no Asaas; a confirmação vem por polling
  (`GET /api/pagamento/pix/:id`), webhook (se `ASAAS_WEBHOOK_TOKEN` estiver definido) ou pelo job de reconciliação, e credita
  uma única vez. Em sandbox dá para simular o pagamento.
- **Saque PIX:** debita valor (+ tarifa, no modo `inscricao_saque`) antes de chamar o Asaas, com `externalReference`
  `saque-<id>`. Recusa definitiva do Asaas devolve tudo; resposta ambígua (timeout, rede, 5xx) **não devolve nada**: o saque
  fica pendente e marcado para revisão, e só um superadmin reembolsa ou marca como pago no painel (depois de conferir no Asaas).
- **Débitos:** surgem quando uma anulação não consegue estornar tudo do saldo. Débito aberto **bloqueia o saque** (409
  `debito_aberto`) e é abatido automaticamente de qualquer crédito seguinte (depósito, prêmio liberado, estorno).
- A carteira mostra saldo, "a receber" (a liberar / retido), débitos e extrato.
- **Receita da plataforma:** taxa sobre as inscrições (`PLATAFORMA_TAXA_INSCRICAO_PCT`, sai da parte da academia) e,
  opcionalmente, tarifa fixa de saque. Fica em `plataforma_receitas`.

### Moderação
- Denúncia (de pessoa ou de desafio), com até 3 imagens de evidência (JPEG/PNG/WebP conferidos por magic bytes, guardadas
  em pasta privada). O denunciante é **sigiloso** (só o admin vê) e o denunciado não vê a denúncia. Limite diário por denunciante.
- Jurisdição: suborno de instrutor em desafio de academia é julgado pela academia dentro de um prazo; vencido, o job sobe ao
  admin. O resto vai direto ao admin.
- Punições: advertência, suspensão (com fim), banimento, bloqueio na academia e anulação de resultado. **Suspenso** faz login
  e lê, mas não participa nem gerencia (403 `conta_suspensa`). **Banido** não entra (403 `conta_banida`) e pode recorrer
  pela própria tela de login. Punições valem para a conta nos dois modos.
- Recurso (um por punição, dentro de `RECURSO_PRAZO_DIAS`), julgado pelo admin. Toda etapa vai para uma linha do tempo
  (`moderacao_eventos`).

### Painel admin (`/admin`)
Painel com conferência financeira (Σ saldos + custódia + pendentes − débitos + receitas = depósitos − saques + ajustes,
com a divergência destacada), pendências, transações, saques (reconciliar; reembolsar/marcar pago só superadmin),
usuários (aplicar/revogar punição), desafios (cancelar, anular resultado), academias e instrutores (aprovar/rejeitar, com
o endereço estruturado da academia), receita e reservas, denúncias, punições, recursos, pagamentos pendentes, débitos,
administradores (só superadmin) e auditoria. Toda ação de escrita exige **motivo** e grava `admin_auditoria` na mesma
transação.

### Social e treinos
Amigos (solicitações), ranking entre amigos, perfil público, convites para desafios, notificações in-app, recomendações
de desafios (heurística; com `ANTHROPIC_API_KEY` o texto da razão é gerado pela API da Anthropic, sem ela o app funciona
igual) e treinos próprios com compartilhamento por código, modo guiado e radar muscular.

---

## Segurança

- **Sessão só por cookie** (o JWT nunca aparece no corpo das respostas). Usuário: `jwt` (HTTP de desenvolvimento) ou
  `__Host-jwt` (com `Secure`), `HttpOnly`, `SameSite=Lax`, `Path=/`. Admin: `admin_jwt` / `__Secure-admin_jwt`,
  `HttpOnly`, `SameSite=Strict`, `Path=/api/admin`, validade `ADMIN_TOKEN_TTL`. `Secure` liga com `APP_ENV=production` ou
  `APP_BASE_URL` em `https://`. O cookie expira junto com o JWT; logout e troca/reset de senha o apagam com os mesmos
  atributos e revogam todas as sessões (`token_versao`). Respostas de autenticação saem com `Cache-Control: no-store`.
- Token de usuário nunca vale em `/api/admin` e vice-versa (segredo e audiência diferentes). Em produção o servidor não
  sobe com `JWT_SECRET`/`ADMIN_JWT_SECRET` vazios, curtos, de exemplo ou iguais.
- **CSRF:** requisições que mudam estado autenticadas por cookie exigem `Origin`/`Referer` da própria origem,
  `APP_BASE_URL` ou `CORS_ORIGINS` (o login admin também).
- **CSP** restritiva no painel admin (`script-src 'self'`, sem `unsafe-inline`, `frame-ancestors 'none'`,
  `X-Frame-Options: DENY`, `no-store`), aplicada a qualquer forma do caminho (`/%61dmin`, `/ADMIN/`…). O front inteiro não tem
  JS inline nem `on*=` e renderiza dados da API com `textContent`. (O app ainda usa atributos `style=` estáticos, então
  ele não recebe a mesma CSP.)
- Rate limit em login (usuário e admin, contadores separados; o do admin nunca desliga), cadastro, recuperação de senha,
  recurso de banimento e consulta de CEP. Senhas com bcrypt e comparação em tempo constante.

---

## Como rodar localmente

Pré-requisitos: Go 1.26, Docker (para o MySQL) e, opcionalmente, conta sandbox no Asaas.

```bash
# 1. MySQL 8 descartável (senha aleatória, porta só no localhost)
export SENHA_DB=$(openssl rand -hex 16)
docker run -d --name gymscore-db -e MYSQL_ROOT_PASSWORD="$SENHA_DB" -e MYSQL_DATABASE=gymscore \
  -p 127.0.0.1:3306:3306 mysql:8.4

# 2. Configuração
cd gymScore-backend
cp .env.example .env        # ajuste DB_PASSWORD (= $SENHA_DB) e, se quiser PIX, a chave sandbox do Asaas

# 3. Sobe API + app + painel (aplica as migrations ao subir)
go run .                    # ou: air
```

Acesse **http://localhost:3000** (app), **http://localhost:3000/admin/login** (painel) e
**http://localhost:3000/swagger/** (API). Para criar contas de teste prontas, veja a próxima seção.

Para apagar tudo: `docker rm -f gymscore-db`.

---

## Contas de teste

> **Atenção:** estas contas **só existem depois de rodar `go run . seed-teste`** e **nunca em produção** — o comando
> recusa rodar com `APP_ENV=production`. Os e-mails são fictícios (domínio reservado `.test`).

```bash
cd gymScore-backend
go run . seed-teste         # aplica as migrations pendentes, cria/atualiza as contas e sai
```

O comando é idempotente: rodar de novo reaproveita as contas pelo e-mail (restaura nome, senha, status de aprovação,
vínculo e endereço), não duplica o desafio enquanto ele estiver aberto e com data futura, e não credita o saldo de novo.
Cada execução grava a auditoria `cli_seed`. Todas as contas usam a senha **`teste123`**.

| E-mail | Senha | Papel | O que dá para testar |
|---|---|---|---|
| `atleta@gymscore.test` | `teste123` | Atleta comum, saldo inicial de R$ 500,00 (crédito `ajuste` no ledger) | Feed, entrar/sair do desafio de teste (entrada R$ 20,00), ingresso com QR, carteira e extrato, amigos, treinos, denúncias. |
| `instrutor@gymscore.test` | `teste123` | Atleta com **modo instrutor aprovado**, vinculado à Academia Teste e designado no desafio de teste | Seletor de modo; no modo instrutor: agenda, iniciar o desafio, marcar presença e lançar o pódio. No modo atleta: ver o desafio da própria academia bloqueado (`motivo_bloqueio_entrada`) e competir em desafios de outras academias. |
| `instrutor.pendente@gymscore.test` | `teste123` | Atleta com **pedido de instrutor pendente** | Estado "em análise" no seletor; tudo de atleta funciona normalmente; aprovar o pedido pelo painel admin. |
| `academia@gymscore.test` | `teste123` | Academia **aprovada**, com endereço estruturado e um desafio **aberto** (evento em 30 dias, 20 vagas, pódio de 3, instrutor por percentual, sem reserva) | Painel da academia, criar/editar/cancelar desafios, convidar instrutores, editar o endereço com busca de CEP, denúncias da academia. |
| `academia.pendente@gymscore.test` | `teste123` | Academia **pendente de aprovação** | Aviso "cadastro em análise" nas telas de gestão; aprovar/rejeitar pelo painel admin. |
| `admin@gymscore.test` | `teste123` | **Superadmin** do painel (tabela `administradores`) | Login em `/admin/login`, conferência financeira, aprovações, saques, moderação, auditoria (inclusive as linhas `cli_seed`). |

Fora do seed, o painel exige senha de admin com no mínimo 12 caracteres (CLI e troca de senha); a senha curta do admin
de teste só existe porque o seed é proibido em produção.

---

## Variáveis de ambiente

Todas estão documentadas em [`gymScore-backend/.env.example`](./gymScore-backend/.env.example). `ENV_FILE` aponta
outro arquivo. As principais:

| Variável | Para quê |
|---|---|
| `APP_ENV`, `APP_PORT`, `APP_BASE_URL` | Ambiente (`production` endurece cookies e segredos e proíbe o `seed-teste`), porta e URL pública (links de e-mail, CSRF, `Secure` quando https). |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `MIGRATE_ON_BOOT` | MySQL e se as migrations rodam ao subir. |
| `JWT_SECRET`, `ADMIN_JWT_SECRET`, `ADMIN_TOKEN_TTL` | Segredos (obrigatórios e diferentes em produção; gere com `openssl rand -hex 32`) e validade do login admin. |
| `CORS_ORIGINS`, `RATE_LIMIT_MAX`, `ADMIN_LOGIN_RATE_LIMIT_MAX` | Origens permitidas e limites de tentativas. |
| `FRONTEND_PATH`, `UPLOADS_DIR`, `EVIDENCIAS_DIR` | Arquivos do front; fotos antigas de check-in; evidências de denúncia (pasta **privada**, com volume e backup em produção). |
| `ASAAS_ENV`, `ASAAS_API_KEY_SANDBOX`, `ASAAS_API_KEY_PROD`, `ASAAS_WEBHOOK_TOKEN` | Gateway PIX (`sandbox`/`prod`); webhook vazio = desligado (confirmação por polling e jobs). |
| `PLATAFORMA_TAXA_MODO`, `PLATAFORMA_TAXA_INSCRICAO_PCT`, `PLATAFORMA_TARIFA_SAQUE_CENTAVOS` | Receita da plataforma. |
| `JOBS_INTERVALO`, `DESAFIO_CARENCIA` | Intervalo dos jobs (0 desliga) e carência antes de cancelar desafio vencido. |
| `PREMIO_JANELA_CONTESTACAO_HORAS` | Janela de contestação do pódio. |
| `DENUNCIA_PRAZO_ACADEMIA_HORAS`, `DENUNCIA_LIMITE_DIA`, `DENUNCIA_EVIDENCIA_MAX_BYTES`, `RECURSO_PRAZO_DIAS` | Moderação. |
| `VIACEP_URL`, `CEP_RATE_LIMIT_MAX` | Busca de CEP. |
| `SMTP_*` | E-mail de recuperação de senha (sem SMTP o link sai no log, modo demo). |
| `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` | Opcional: texto das recomendações. |

---

## Migrations

O schema é versionado com **goose** em `gymScore-backend/database/migrations/` (00001 a 00020), embutido no binário e
aplicado ao subir (`MIGRATE_ON_BOOT=true`), pela CLI `admin`/`seed-teste` ou à mão com a CLI do goose. Réplicas que sobem
juntas migram uma de cada vez (trava `goose_lock`). As migrations a partir da 00010 usam procedures temporárias para
ADD/DROP condicional e são reaplicáveis depois de uma falha parcial.

- Tabela de versões, regras de conversão de dados antigos e consultas de conferência do ledger:
  [`gymScore-backend/database/README.md`](./gymScore-backend/database/README.md).
- **Problemas ao migrar no RDS** (erros 1304/1305, 1644, 3748 GTID, out-of-order/1824, 1370/1044, 1205), com como
  detectar e corrigir cada um: seção "Problemas comuns ao migrar no RDS" do mesmo arquivo.
- Faça backup (`mysqldump`) antes de apontar o app para um banco com dados: DDL do MySQL não é transacional.

---

## Testes e CI

```bash
cd gymScore-backend
make build vet test            # go build, go vet (normal e -tags integration), go test -race ./...
```

Os testes de integração usam um **MySQL 8 real e descartável** (nunca um banco com dados): recriam o banco `gymscore_it`
(ou `TEST_DB_NAME`) a cada execução.

```bash
SENHA=$(openssl rand -hex 16)
docker run -d --name gymscore-it -e MYSQL_ROOT_PASSWORD="$SENHA" -p 127.0.0.1:3307:3306 mysql:8.4
TEST_DB_DSN="root:$SENHA@tcp(127.0.0.1:3307)/?parseTime=True&loc=Local" make test-integration
docker rm -f gymscore-it
```

A suíte cobre os fluxos de ponta a ponta (dinheiro, concorrência, moderação, painel admin, modos, endereço/CEP, seed) e
as migrations (do zero, Down/Up e sobre o estado parcial do RDS). O CI (`.github/workflows/ci.yml`) roda gofmt, build,
vet, `go test -race`, a integração contra um serviço MySQL 8 e `govulncheck`; `make ci` roda build, vet, testes,
integração e `govulncheck` localmente (o gofmt é `make fmt`).

---

## Comandos de linha de comando

Todos aplicam as migrations pendentes e saem sem subir o servidor.

```bash
go run . admin criar-superadmin --email voce@exemplo.com --nome "Seu Nome"   # 1º superadmin
go run . admin resetar-senha --email voce@exemplo.com                         # nova senha; derruba as sessões
go run . seed-teste                                                           # contas de teste (nunca em produção)
```

A senha dos comandos `admin` é pedida no terminal sem eco (duas vezes, mínimo 12 caracteres) e nunca aceita por
argumento; sem terminal ela é lida do stdin, uma por linha. Ambos gravam auditoria (`cli_criar_superadmin`,
`cli_resetar_senha`).

---

## Documentação complementar

- Backend (configuração, regras do dinheiro e do desafio, endpoints): [`gymScore-backend/README.md`](./gymScore-backend/README.md)
- Banco e migrations: [`gymScore-backend/database/README.md`](./gymScore-backend/database/README.md)
- Frontend: [`gymScore-frontend/README.md`](./gymScore-frontend/README.md)
- API completa: Swagger em `http://localhost:3000/swagger/` (fora de produção)
- Especificação da moderação: [`analise/moderacao-spec.md`](./analise/moderacao-spec.md)
