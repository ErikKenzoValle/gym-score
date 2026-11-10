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
  encerrar suspensões e reconciliar o Nível GymScore (`reconciliar-nivel`).
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
| **Instrutor** | Não é outra conta: é o **segundo modo** da conta de atleta. O atleta pede (`/cadastro-instrutor`, CREF) e um admin aprova. | No modo instrutor: agenda, "trabalhos oferecidos" (candidatura às equipes de instrutores, sem lance), presença nos desafios em cuja equipe está, início e pódio quando é o responsável, convites de academias. |
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
inscreve em desafio em cuja **equipe de instrutores** está (qualquer membro, não só o responsável), em desafio da academia
a que está vinculado, nem onde tem candidatura pendente/aceita; e não pode ser indicado/se candidatar em desafio em que
está inscrito. As duas direções são
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
   se desvincular depois (409 enquanto estiver na equipe de algum desafio aberto ou em andamento).

### Desafios
- Só a academia aprovada cria desafios: **local** com endereço estruturado (o formulário vem pré-preenchido com o
  endereço da academia e aceita outro CEP; o backend monta o texto `local`), **início e fim** do evento, vagas, entrada
  (0 a R$ 10.000), **resumo** (20–280 caracteres) e **regras completas** (50–10.000), ambos obrigatórios e em texto puro,
  número de premiados (1–100) com a **divisão do prêmio definida pela academia**, prêmio fixo e/ou percentual do pote,
  requisitos opcionais de **nível GymScore** (1–10) e **idade** (0–120) e a **equipe de instrutores**.
  Uma academia pode ter vários desafios ativos ao mesmo tempo (ex.: corrida de 5, 10 e 15 km).
- **Premiação configurável (rodada 6):** `divisao_premio_bp` tem uma fatia por posição em pontos-base (10000 = 100%,
  permite 33,33%): soma exata 10000, cada uma > 0 e nunca maior que a da posição anterior. Sem ela vale a divisão padrão
  (60/30/10… até 5 posições; acima, pesos lineares decrescentes — `GET /api/desafios/divisao-padrao?posicoes=N`). Com
  menos presentes que premiados, as fatias das posições ocupadas são renormalizadas; o resto de centavos vai para o 1º.
  Divisão, número de premiados e requisitos de nível/idade só mudam **antes da primeira inscrição** (409 depois).
- **Requisitos de entrada (rodada 6):** nível no momento da inscrição (recalculado na hora) e idade na data do evento;
  quem não cumpre vê `pode_entrar: false` + `motivo_bloqueio_entrada` (ex.: "Este desafio é para nível 3 a 6; você está
  no nível 2") e recebe 409 ao tentar entrar. Sem data de nascimento num desafio com filtro de idade: "Informe sua data de
  nascimento no perfil" (`PATCH /api/usuarios/perfil` aceita `data_nascimento` só para quem ainda não tem). Mudança de
  nível depois não expulsa ninguém. A recomendação nunca sugere desafio inelegível.
- **Equipe de instrutores (rodada 5, sem leilão).** A academia define quantos instrutores o evento terá
  (`instrutores_vagas`, 1–20) e as condições **por instrutor**: fixo, percentual das inscrições ou os dois. Ela pode
  **indicar** já na criação (ou depois) instrutores vinculados e aprovados; as vagas restantes ficam abertas e aparecem
  para qualquer instrutor aprovado em "Trabalhos oferecidos" (`GET /api/instrutor/trabalhos`). O instrutor se
  **candidata aceitando as condições** (mensagem opcional) e a academia aceita ou recusa; ao preencher a última vaga as
  pendentes são recusadas e avisadas. A academia pode remover membros até o desafio iniciar e escolhe **1 responsável**
  (padrão: o primeiro indicado; sem indicados, o primeiro aceito). Todos os membros marcam presença; só o responsável (ou
  a academia) inicia e lança o pódio. O desafio pode iniciar com a equipe incompleta (≥ 1 instrutor e responsável definido).
- **Dinheiro da equipe:** a reserva debitada da academia é `prêmio fixo + instrutores_vagas × fixo por instrutor`
  (ajustada na edição); a soma `percentual do pote + taxa da plataforma + % do instrutor × instrutores_vagas` não passa de
  100%. No encerramento cada membro recebe fixo + % das inscrições; a parte fixa das vagas não preenchidas volta para a
  academia junto com o líquido dela. No cancelamento a reserva volta inteira.
- O atleta entra pagando a inscrição pela carteira e recebe um ingresso com QR; pode sair enquanto está `aberto` (reembolso).
  **Choque de horário:** não dá para se inscrever em dois desafios **da mesma academia** com horários sobrepostos
  (`[início, fim)`); em horários diferentes, ou em academias diferentes, pode. O feed e o detalhe já avisam
  (`pode_entrar: false` + `motivo_bloqueio_entrada`). A academia também não consegue **mudar o horário** de um desafio
  com inscritos para cima de outro desafio dela em que algum inscrito já está (409).
- No dia, a equipe marca presença (QR ou lista; fica registrado quem marcou e como); o responsável ou a academia inicia e
  lança o pódio (só presentes elegíveis, marca obrigatória). Ciclo: `aberto` → `em_andamento` → `encerrado`
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

### Nível GymScore e gráficos de teia (rodada 6)
- **Nível 1–10** a partir de uma pontuação de todo o histórico: pódio (1º 50, 2º 35, 3º 25, demais premiados 15),
  10 por presença confirmada em desafio encerrado e 1 por **dia** com treino registrado (treino é autodeclarado).
  Resultado anulado pela moderação não conta. Faixas: 1 = 0–19, 2 = 20–49, 3 = 50–99, 4 = 100–174, 5 = 175–274,
  6 = 275–399, 7 = 400–574, 8 = 575–799, 9 = 800–1099, 10 = 1100+.
- Cache em `usuarios.pontuacao_gymscore`/`nivel_gymscore`, gravado na mesma transação dos eventos (pódio lançado ou
  relançado, anulação, treino, inscrição) e reconferido depois do commit; o job `reconciliar-nivel` calcula contas
  nunca calculadas e revisita as que não mudam há 24 h.
- `GET /api/usuarios/:id/estatisticas` (qualquer usuário logado, como o perfil público): nível, pontuação, quanto falta
  para o próximo, **6 atributos** (Força, Resistência, Velocidade, Agilidade, Mobilidade, Constância) e **8 categorias**
  (corrida, musculação, crossfit, funcional, ciclismo, natação, calistenia, luta), cada eixo 0–100, sobre os últimos
  180 dias com mais peso no recente, e o texto "como calculamos". A tabela de pesos (grupo muscular / categoria →
  atributo) fica num lugar só: `internal/services/gymscore.go`.

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

**Automático ao subir (B5-6):** com `APP_ENV` definido **explicitamente** (no ambiente ou no `.env`) como `development`,
`dev`, `local` ou `test` (sem diferenciar maiúsculas) o servidor roda este mesmo seed logo depois das migrations, então as
contas abaixo já existem ao abrir o app. É idempotente (nunca duplica contas, o desafio de exemplo nem o crédito
inicial), usa uma trava do MySQL (`GET_LOCK`) para duas instâncias subindo juntas (quem não pega a trava em 30 s pula),
e uma falha só vira log — o servidor sobe mesmo assim. `SEED_TESTE_AUTO=false` desliga; sem `APP_ENV` ou com qualquer
outro valor (`staging`, `production`, vazio…) ele não roda (só um log informativo dizendo por quê). O comando manual
abaixo continua só recusando `APP_ENV=production`. Para rodar à mão:

```bash
cd gymScore-backend
go run . seed-teste         # aplica as migrations pendentes, cria/atualiza as contas e sai
```

O comando é idempotente: rodar de novo reaproveita as contas pelo e-mail (restaura nome, senha, status de aprovação,
vínculo e endereço), não duplica o desafio enquanto ele estiver aberto e com data futura, e não credita o saldo de novo.
Cada execução grava a auditoria `cli_seed`. Todas as contas usam a senha **`teste123`**.

| E-mail | Senha | Papel | O que dá para testar |
|---|---|---|---|
| `atleta@gymscore.test` | `teste123` | Atleta comum, saldo inicial de R$ 500,00 (crédito `ajuste` no ledger), com **histórico** de 6 desafios encerrados (corrida, ciclismo, funcional, natação, crossfit; vários pódios) e ~60 dias de treino nos últimos 6 meses → nível alto | Feed, entrar/sair do desafio de teste (entrada R$ 20,00) e do **desafio avançado** (nível 5+, 18+), ingresso com QR, carteira e extrato, amigos, treinos, denúncias, **nível e gráficos de teia** no perfil. |
| `instrutor@gymscore.test` | `teste123` | Atleta com **modo instrutor aprovado**, vinculado à Academia Teste, **indicado e responsável** na equipe do desafio de teste (e do avançado); ~45 dias de treino de força nos últimos 6 meses (gráfico de atributos puxado para Força) | Seletor de modo; no modo instrutor: agenda (equipe e responsável), iniciar o desafio, marcar presença e lançar o pódio. No modo atleta: ver o desafio da própria academia bloqueado (`motivo_bloqueio_entrada`) e competir em desafios de outras academias. |
| `instrutor.pendente@gymscore.test` | `teste123` | Atleta com **pedido de instrutor pendente** (2 desafios no histórico, nível baixo) | Bloqueio por nível no desafio avançado (`motivo_bloqueio_entrada`). Estado "em análise" no seletor; tudo de atleta funciona normalmente; aprovar o pedido pelo painel admin — depois de aprovado, candidatar-se à vaga aberta da equipe do desafio de teste em "Trabalhos oferecidos". |
| `academia@gymscore.test` | `teste123` | Academia **aprovada**, com endereço estruturado e um desafio **aberto** (evento em 30 dias, das 8h às 10h, no endereço da academia, 20 vagas, pódio de 3, equipe de **2 instrutores** — 1 indicado/responsável e 1 vaga aberta à candidatura —, 10% das inscrições por instrutor, sem reserva) e um 2º desafio aberto **"Funcional avançado (nível 5+)"** (em 40 dias, nível 5–10, 18+, 5 premiados com divisão 35/25/20/12/8%). O histórico (desafios encerrados **gratuitos**: não mexem no ledger) também é desta academia | Painel da academia, criar/editar/cancelar desafios (vários ao mesmo tempo), gerir a equipe (candidatos, aceitar/recusar, remover, responsável), convidar instrutores, editar o endereço com busca de CEP, denúncias da academia. |
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
| `SEED_TESTE_AUTO` | `true` (padrão) roda o seed das contas de teste ao subir, depois das migrations; `false` desliga. Só roda com `APP_ENV` explícito ∈ {`development`, `dev`, `local`, `test`}; sem `APP_ENV` ou com outro valor não roda. |
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
