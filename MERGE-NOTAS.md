# MERGE-NOTAS — resolução dos conflitos DEV_BACKEND

Autor: dev-backend. Branch: `merge-reestruturacao` (merge de `main` em `resestruturacao-projeto`, ainda sem commit).

Status: os 20 conflitos de `gymScore-backend/` estão resolvidos e `git add`ados. `go build ./...`,
`go vet ./...` (com e sem a tag `integration`) e `go test ./internal/services/... ./internal/middlewares/... ./internal/config/...`
passam. Não toquei em `gymScore-frontend/` nem em `database/migrations/`.

**Atualização (mesmo dia): gamificação da apuração portada de volta.** Ao descartar o `desafio_service.go`
antigo da branch (seção 1), a pontuação de Elo que ele concedia ao encerrar um desafio (vencedor +30,
demais participantes +5) ficou de fora — a versão da main não tinha esse conceito porque, na main,
desafio não dava pontos. Portado para dentro de `apurarNaTransacao` (seção 5).

**Atualização (mesmo dia): bug de `data_nascimento` encontrado pela suíte de integração do dev-db,
corrigido.** `GET /api/usuarios/me` devolvia `data_nascimento` como `"1995-03-15T00:00:00-03:00"` em
vez de `"1995-03-15"`. Causa: ao tornar `Usuario.DataNascimento` nullable no merge (seção 1), o campo
virou `*string`; escanear uma coluna `DATE` direto para `*string` faz o driver devolver a data já
formatada como datetime com fuso, não a data pura. Corrigido (seção 6): o campo agora é `*time.Time`,
e o DTO formata com `"2006-01-02"` na borda da API. No processo, achei e corrigi um SEGUNDO bug (fuso):
`time.Parse` (sem `ParseInLocation`) assume UTC, e a conexão com o banco usa fuso local — uma data
enviada como `"1995-03-15"` virava `1995-03-14` na coluna em qualquer fuso negativo. Ambos confirmados
com a suíte de integração completa rodando de verdade contra MySQL 8 (Docker local).

**Atualização (mesmo dia): `recomendados`/`historico` vazavam o model cru, achado pelo dev-frontend —
corrigido (seção 7).** `GET /api/desafios/recomendados` e `GET /api/usuarios/:id/historico` devolviam
`models.Desafio` direto (PascalCase, sem DTO). Agora usam o mesmo `DesafioResponse` snake_case do resto
da API (recomendados) e um DTO dedicado `HistoricoDesafioResponse` (histórico, que expõe só o resultado
— vencedor/prêmio — do usuário consultado, não de todo mundo que participou do desafio).

## 1. Decisões por arquivo conflitante

- **`.env.example`, `go.mod`**: união simples das duas seções (Anthropic + SMTP; dependências Anthropic + validator/goose). `go.sum` foi regenerado do zero com `go mod tidy` após resolver o código (baixou `testify` e `modernc.org/sqlite`, usados pelos testes de integração).
- **`internal/config/config.go`**: união dos campos (Anthropic + SMTP) e das funções (`IAHabilitada`, `IsProduction`). Sem perda de nenhum dos dois lados.
- **`internal/models/usuario.go`**: campo a campo da entidade `Usuario` — `CPF`, `DataNascimento`, `Genero` ficam `*string` (nulos para contas "academia", pessoa jurídica) **e** `SaldoCentavos`/`TokenVersao` ficam somente-leitura (`gorm:"->"`), e os campos de gamificação (`Perfil`, `Username`, `Elo`, `Pontos`, `UltimaVez`) da branch são mantidos. O campo `Saldo float64` foi removido do model: dinheiro mora só em `SaldoCentavos` (fonte da verdade, lido via `CarteiraRepository`); a conversão para reais acontece só nos DTOs (`UsuarioResponse.Saldo`, via `utils.CentavosParaReais`). Mantive **os dois** tipos de DTO público (`UsuarioPublico` — id/nome/sobrenome, usado por `carteira.go`/`desafio.go`/votos — e `UsuarioPublicoResponse` — com elo/pontos/username, usado no perfil público), porque código não-conflitante dos dois lados já depende de cada um. O método antigo `RecuperarSenha` (fluxo e-mail+CPF, sem envio de e-mail) foi **removido**: foi substituído pelo fluxo `senha_service.go`/`mailer.go` da main (token por e-mail, 30 min, uso único).
- **`internal/models/amizade.go`**: `AmigoResponse` ganhou `Elo`/`Pontos` (úteis para a tela de amigos), mas **não** ganhou `Email` — o comentário do DTO já dizia "sem e-mail/CPF" e não encontrei nenhum uso de `.email` em `amigos.js`, então mantive a garantia de não vazar e-mail de terceiros.
- **`internal/models/desafio.go`**: fiquei com o `Desafio` da main (sem os FKs `Desafiado`/`Vencedor` de usuário único — modelo antigo de 1x1). O model `DesafioParticipante` tinha **duas declarações duplicadas** pós-merge automático do git (uma de cada lado, em blocos não sobrepostos — isso não aparece como conflito mas não compila). Mesclei em uma só: campos de ledger da main (`PremioCentavos`, `Vencedor`) + campo `Presente` da branch (check-in de presença pelo instrutor/academia).
- **`internal/models/transacao.go`**: fiquei 100% com `Transacao` da main (ledger em centavos, `Tipo` enum, `IDDesafio`, `AsaasTransferID`, `Descricao`). O model antigo (`Valor float64`, `Tipo string "entrada"/"saida"`) foi descartado.
- **`internal/models/pix.go`**: havia um segundo `ExtratoResponse` duplicado (pré-existente, fora do conflito marcado) competindo com o de `carteira.go`; removido o antigo.
- **`internal/repositories/usuario_repository.go`**: união da interface. Os dois lados tinham um método `Buscar` com assinaturas diferentes (impossível coexistir): o da main (`Buscar(termo, excluirID, limite)`, usado para achar amigos, exclui o próprio usuário) ficou com o nome `Buscar`; o da branch (`Buscar(q)`, busca geral por nome/sobrenome/**username**, sem exclusão) foi renomeado para `BuscarPorTermo`. `Deletar`/`Listar` genéricos da branch foram descartados (sem nenhum chamador).
- **`internal/repositories/desafio_repository.go`**: a main tinha migrado `DesafioRepository` de **interface** para **struct concreto** (`*DesafioRepository`, sem mocks — os testes que tocam banco usam harness de integração). Adotei esse desenho (é a direção da main) e portei os métodos da branch que faltavam (`ListarHistoricoUsuario`, `MarcarPresente`, `ListarPresentes`, `ListarParticipacoesUsuario`) para dentro do struct. Isso teve efeito cascata: todo código que guardava um `repositories.DesafioRepository` **por valor/interface** (vários arquivos não-conflitantes da branch) teve que passar a guardar `*repositories.DesafioRepository`. Acrescentei também um campo `Todos bool` em `FiltroDesafios` para o dashboard de academia/agenda de instrutor poderem listar **todos** os desafios (sem o filtro de visibilidade pensado para o feed do atleta).
- **`internal/repositories/transacao_repository.go`**: fiquei com a interface da main (`CriarDepositoPendente`/`BuscarDepositoDoUsuario`/`BuscarDeposito`); a da branch (`Criar`/`BuscarPorAsaasID`/`AtualizarStatus`/`MarcarRecebidoSePendente`) não tinha implementação nenhuma no arquivo (teria sido um símbolo morto).
- **`internal/services/usuario_service.go`**: domínio da branch (ingressos, histórico, perfil público, username) sobre a infra da main (logout/token_versao, `AtualizarSenha` atômico). `HistoricoDesafios` e o campo interno `desafioRepo` passaram a usar `*repositories.DesafioRepository` pelo motivo acima.
- **`internal/services/desafio_service.go`**: o conflito era, na prática, "a implementação inteira": a da branch (pagamento em `float64`, sem ledger, 1 vencedor) foi inteiramente descartada em favor da da main (centavos, ledger, votação/apuração por maioria, check-ins, convites). Nenhuma regra de negócio da branch sobrevivia à mudança de dinheiro para centavos sem reescrever do zero, e a da main já cobre o domínio (entrar/sair/cancelar/iniciar/votar/apurar/convidar).
- **`internal/services/amizade_service.go` / `pix_service.go` / `controllers/pix_controller.go`**: adotei o lado main (nil-safe, ledger, `ConfirmarRecebimento`/`EstornarRecebimento` para o webhook) e só recuperei da branch o que é aditivo (Elo/Pontos no amigo).
- **`internal/controllers/usuario_controller.go`**: união dos dois controllers. Havia **duas rotas/métodos `BuscarUsuarios`** (uma de cada lado, novamente sem conflito marcado pelo git por não se sobreporem) — uma delas foi renomeada para `BuscarPorTermo` (ver endpoints novos abaixo). Removi uma validação manual redundante em `CriarUsuario` (os `validate:"required"` do DTO já cobrem).
- **`internal/routes/routes.go`**: adotei o padrão `Controllers` struct + `Setup(app, cfg, h)` da main (mais limpo que a lista de 11 parâmetros posicionais da branch) e estendi o struct com `Instrutor`, `Academia`, `Recomendacao` e `UsuarioRepo` (para o middleware `RequirePerfil`). Rotas de desafio: 100% as da main (o domínio de desafio mudou de arquitetura, não dá pra ter as duas). Rotas de academia/instrutor/pódio/avaliação/ingressos: mantidas por cima, em paths que não colidem com as de desafio da main.
- **`main.go`**: reduzido para carregar config, aplicar migrations (goose) e delegar para `app.Construir` (bootstrap único, reaproveitado pelos testes de integração). `internal/app/app.go` (não estava em conflito, mas precisou ser estendido) passou a instanciar também instrutor/academia/recomendação/cliente Anthropic.
- **Removido por completo** (fluxo de saque antigo, sem ledger, sem Asaas real — `usuario.Saldo -= valor` direto, sem histórico de transferência): `internal/controllers/saque_controller.go`, `internal/services/saque_service.go` (+ teste), `internal/repositories/saque_repository.go`, `internal/models/saque.go`. O saque "de verdade" já existe na main: `CarteiraController.Sacar` → `CarteiraService` → ledger + PIX real via Asaas, com débito atômico e reembolso automático se a transferência falhar. Também corrigi, nos arquivos que sobraram da branch (academia, instrutor, recomendação, etc.), um typo pré-existente de import (`gynScore-backend` em vez de `gymScore-backend`) que já impedia essas peças de compilar antes do merge.
- **`gymScore-backend/tmp/main`**: binário versionado por engano; removido do índice (o `.gitignore` já ignora `gymScore-backend/tmp/`).

## 2. Schema que os models finais exigem (nada aplicado — só a lista para o dev-db)

O estado atual depois das migrations 00001–00009 já tem `usuarios.saldo_centavos`, `usuarios.token_versao`,
o ledger completo em `transacoes`, `desafios.valor_entrada_centavos`, `desafio_participantes.premio_centavos`/`vencedor`,
check-ins/votos, notificações e reset de senha. Falta para os models da branch (academia/instrutor/pódio/avaliação/perfil):

### ALTER `usuarios`
| Coluna | Tipo | Observação |
|---|---|---|
| `cpf` | `VARCHAR(14)` | passa de `NOT NULL` para **nullable** (contas de academia não têm CPF). Mantém a `UNIQUE KEY` já existente — MySQL permite múltiplos `NULL` nela. |
| `data_nascimento` | `DATE` | passa para **nullable** (idem). |
| `genero` | `ENUM('M','F','O')` | passa para **nullable** (idem). |
| `perfil` | `ENUM('atleta','academia','instrutor')` | `NOT NULL DEFAULT 'atleta'` — nova. |
| `username` | `VARCHAR(50)` | nova, `UNIQUE`, nullable. |
| `elo` | `VARCHAR(20)` | `NOT NULL DEFAULT 'Iniciante'` — nova. |
| `pontos` | `INT` | `NOT NULL DEFAULT 0` — nova. |
| `ultima_vez` | `DATETIME(3)` | nullable — nova. |

### ALTER `desafio_participantes`
| Coluna | Tipo | Observação |
|---|---|---|
| `presente` | `TINYINT(1)` | `NOT NULL DEFAULT 0` — marcado pelo instrutor/academia no check-in presencial (distinto do `desafio_checkins` da main, que é o "check-in" com foto/geolocalização feito pelo próprio atleta). |

### Tabela nova `academias`
| Coluna | Tipo |
|---|---|
| `id` | `BIGINT UNSIGNED PK AUTO_INCREMENT` |
| `id_usuario` | `BIGINT UNSIGNED NOT NULL UNIQUE`, FK → `usuarios(id)` |
| `razao_social` | `VARCHAR(150) NOT NULL` |
| `nome_fantasia` | `VARCHAR(150) NOT NULL` |
| `cnpj` | `VARCHAR(18) NOT NULL UNIQUE` |
| `endereco` | `VARCHAR(255) NULL` |
| `telefone` | `VARCHAR(20) NULL` |
| `criado_em`, `atualizado_em` | `DATETIME(3)` |

### Tabela nova `instrutores`
| Coluna | Tipo |
|---|---|
| `id` | `BIGINT UNSIGNED PK AUTO_INCREMENT` |
| `id_usuario` | `BIGINT UNSIGNED NOT NULL UNIQUE`, FK → `usuarios(id)` |
| `cref` | `VARCHAR(20) NOT NULL` |
| `especialidade` | `VARCHAR(100) NULL` |
| `id_academia` | `BIGINT UNSIGNED NULL`, FK → `academias(id)` (autônomo = NULL) |
| `criado_em`, `atualizado_em` | `DATETIME(3)` |

### Tabela nova `podio_desafios`
| Coluna | Tipo |
|---|---|
| `id` | `BIGINT UNSIGNED PK AUTO_INCREMENT` |
| `id_desafio` | `BIGINT UNSIGNED NOT NULL`, FK → `desafios(id)` |
| `posicao` | `INT NOT NULL` — `UNIQUE (id_desafio, posicao)` |
| `id_participante` | `BIGINT UNSIGNED NOT NULL`, FK → `usuarios(id)` |
| `marca` | `VARCHAR(50) NULL` |
| `criado_em` | `DATETIME(3)` |

### Tabela nova `avaliacoes_desafios`
| Coluna | Tipo |
|---|---|
| `id` | `BIGINT UNSIGNED PK AUTO_INCREMENT` |
| `id_desafio` | `BIGINT UNSIGNED NOT NULL`, FK → `desafios(id)` — `UNIQUE (id_desafio, id_usuario)` |
| `id_usuario` | `BIGINT UNSIGNED NOT NULL`, FK → `usuarios(id)` |
| `nota` | `INT NOT NULL CHECK (nota BETWEEN 1 AND 5)` |
| `criado_em` | `DATETIME(3)` |

Não precisa de tabela nova: `ingresso` é só um DTO derivado de `desafios` + `desafio_participantes` (código de ingresso é gerado em memória, não persistido). O fluxo de saque antigo (tabela `saques`) foi removido — não precisa migrar nada, o saque já usa só `transacoes` (ledger).

## 3. Mudanças de contrato de API (para o dev-frontend)

**Carteira/saque/extrato — usar SEMPRE os endpoints da main, em centavos:**
- `GET /api/carteira` → saldo (`saldo_centavos` + `saldo` em reais só para exibição)
- `GET /api/carteira/extrato?pagina=&limite=` → paginado (ledger completo: depósito/saque/entrada_desafio/estorno/prêmio/ajuste)
- `POST /api/carteira/saque` → body `{ valor_centavos, chave_pix, tipo_chave }` (`tipo_chave` ∈ `CPF|CNPJ|EMAIL|PHONE|EVP`)
- `GET /api/carteira/saque/:id` → polling do status do saque
- **Removidos**: `GET /api/pagamento/extrato`, `POST /api/pagamento/saque`, `GET /api/pagamento/saque` (fluxo antigo sem ledger/Asaas real). O `api.js` atual tem chamadas para os dois conjuntos de rotas (`carteira.*` e `pagamento.*` — vi isso ao ler o arquivo, não editei) — as que usam `/api/pagamento/saque*` e `/api/pagamento/extrato` vão passar a dar 404 e precisam ser removidas/redirecionadas para `carteira.*`.
- `POST /api/pagamento/pix` (depósito) continua existindo, mas o body agora é `{ valor_centavos, cpf }` (antes era `valor` em reais).

**Desafio — domínio trocado por completo** (a branch tinha um desafio "1 contra 1"; a main tem desafio em grupo com pote/votação):
- Removidos: `/desafios/view`, `/desafios/aceitar_desafio`, `/desafios/iniciar`, `/desafios/encerrar`, `/desafios/cancelar` (todos POST com corpo), `GET /desafios/:id` (antigo, "por usuário").
- Novos, todos sob `/api/desafios`: `GET /` (lista paginada/filtrada), `GET /meus`, `POST /` (criar), `GET /:id` (detalhe), `POST /:id/entrar`, `/sair`, `/cancelar`, `/iniciar`, `/abrir-votacao`, `/votar`, `/apurar`, `/convidar`, `GET /:id/checkins`, `POST /:id/checkins`.
- Valores em `valor_entrada_centavos` (antes `valor` em reais). Pode haver **mais de um vencedor** por desafio (cada participante tem `vencedor: bool` + `premio_centavos`), não existe mais um único `id_vencedor` no desafio.

**Usuário/perfil:**
- `GET /api/usuarios/busca?q=` — busca por nome, **exclui o próprio usuário** (pensada para "adicionar amigo").
- `GET /api/usuarios/buscar?q=` — busca por nome/sobrenome/**username**, não exclui ninguém (uso geral). As duas rotas coexistem; o `api.js` atual define a função `buscarUsuarios` duas vezes (uma para cada rota) — o dev-frontend precisa escolher nomes distintos ou decidir qual tela usa qual.
- `GET /api/usuarios/:id/publico`, `GET /api/usuarios/:id/historico`, `GET /api/usuarios/ingressos`, `PATCH /api/usuarios/ultima-vez` — mantidos como estavam na branch.
- `UsuarioResponse.cpf` agora pode vir `null` (conta de academia). `AmigoResponse` ganhou `elo`/`pontos`, mas **não tem `email`**.
- Cadastro (`POST /api/usuarios`): senha mínima subiu de 6 para 8 caracteres; `sobrenome` passou a ser obrigatório; `data_nascimento` valida formato estrito `YYYY-MM-DD`.

**Academia/Instrutor/Pódio (sem mudança de contrato, só confirmando que sobreviveram ao merge):**
`POST /api/academias`, `POST|GET /api/instrutores`, `GET /api/academia/dashboard` (perfil academia), `GET /api/instrutor/agenda`,
`GET /api/desafios/:id/presentes`, `POST /api/desafios/:id/presenca` (perfil academia), `GET|POST /api/desafios/:id/podio` (POST = perfil academia),
`POST /api/desafios/:id/avaliar`, `GET /api/desafios/recomendados`.

## 5. Gamificação da apuração (regressão corrigida)

Ao encerrar um desafio (`apurarNaTransacao`, em `internal/services/desafio_service.go`), além de
pagar o(s) prêmio(s) via ledger, agora também pontua:

- Quem está em `premios` (ou seja, `Vencedor=true`, mesma lista usada para `RegistrarPremio` — inclui
  o caso raro de prêmio `0` centavos por divisão inteira entre muitos empatados) recebe **+30 pontos**.
- Os demais participantes do desafio recebem **+5 pontos**.
- O UPDATE é atômico (`UsuarioRepository.IncrementarPontos(tx, id, delta)` → `UPDATE usuarios SET pontos = pontos + ?`),
  na MESMA transação `tx` da apuração — nunca lê o valor antigo em memória (sem lost update), e
  pontuação e prêmio em dinheiro vivem/morrem juntos (rollback de um desfaz o outro).
- Idempotente por construção: a trava de status (`MudarStatus(EmVotacao, Encerrado)`, CAS atômico já
  existente) garante que o bloco de pontuação só executa uma vez por desafio — reapurar é rejeitado
  com 409 antes de chegar lá. Testado explicitamente (`TestDesafio_FluxoCompletoComVencedor`, reapurar
  depois de apurado).
- Desafio cancelado/estornado nunca passa por `apurarNaTransacao` (fluxo de `Cancelar` é outro
  método), então nunca pontua — testado em `TestDesafio_CancelarReembolsaTodos`.
- `Elo` continua sendo derivado de `Pontos` só na borda da API (`utils.CalcularElo`, em
  `toUsuarioResponse`/`toUsuarioPublicoResponse`/`AmigoResponse`) — a coluna `usuarios.elo` nunca é
  escrita por este fluxo (decisão de sprints anteriores, mantida).

Mudança de assinatura: `NovoDesafioService`/`NovoDesafioServiceComRelogio` ganharam um novo parâmetro
`usuarios repositories.UsuarioRepository` (entre `amizades` e `notif`); `internal/app/app.go` já
atualizado. `UsuarioRepository` ganhou o método `IncrementarPontos(tx *gorm.DB, id uint, delta int) error`.

**Testes adicionados** em `internal/integration/desafio_test.go` (build tag `integration`):
`TestDesafio_FluxoCompletoComVencedor` (vencedor único +30, demais +5, idempotência no reapurar),
`TestDesafio_EmpateDivideOPote` (dois empatados na vitória +30 cada, os outros dois +5),
`TestDesafio_CancelarReembolsaTodos` (cancelar não pontua ninguém). Novo helper `pontos(t, u)` em
`harness_test.go`. **Executei de fato** contra um MySQL 8 descartável em Docker local (`TEST_DB_DSN`)
depois que a migration `00010_perfis_academia_instrutor.sql` do dev-db ficou disponível — rodei todo
`TestDesafio*` (18 testes, incluindo concorrência) e passou 100%. `go build ./...`, `go vet ./...`
(com e sem a tag `integration`) e os testes unitários (`internal/services`, `internal/middlewares`,
`internal/config`) continuam passando.

## 6. Bug de `data_nascimento` (regressão corrigida)

Achado pela suíte de integração do dev-db contra MySQL limpo (71/72 passando, `TestUsuario_PerfilPublicoNaoVazaDados` falhando).

- **Sintoma:** `GET /api/usuarios/me` → `data_nascimento: "1995-03-15T00:00:00-03:00"` em vez de `"1995-03-15"`.
- **Causa 1 (tipo do campo):** `Usuario.DataNascimento` é `*string` desde que ficou nullable no merge (seção 1). GORM, ao escanear uma coluna `DATE` para um destino `*string` (em vez de `*time.Time`), deixa o driver (`go-sql-driver/mysql`, com `parseTime=true`) formatar o valor como datetime completo com fuso — não a data pura. Isso nunca dava pra notar sem rodar contra um MySQL real (os testes unitários usam mocks em memória).
- **Correção:** `Usuario.DataNascimento` agora é `*time.Time` (`internal/models/usuario.go`); `UsuarioResponse.DataNascimento` continua `string`, formatado com `t.Format("2006-01-02")` em `toUsuarioResponse` (`internal/services/usuario_service.go`, novo helper `formatarData`). `CriarUsuarioRequest.DataNascimento` não mudou (segue `string`, validada `datetime=2006-01-02` na entrada).
- **Causa 2 (fuso, achada ao corrigir a 1ª):** o parse do input usava `time.Parse("2006-01-02", ...)`, que assume UTC. A conexão com o banco usa o fuso local (`config.AplicarFusoDSN`/`loc=Local`) — um `time.Time` em UTC meia-noite grava um dia ANTES na coluna `DATE` em qualquer fuso negativo (ex.: `1995-03-15T00:00:00Z` é `1995-03-14T21:00:00-03:00`, e só a parte da data é persistida). Troquei para `time.ParseInLocation("2006-01-02", req.DataNascimento, time.Local)` — mesma técnica já usada em `parseDataEncerramento` (desafio_service.go) para o mesmo tipo de campo.
- **Validado:** roda contra MySQL 8 real (Docker local) — `TestUsuario_PerfilPublicoNaoVazaDados`, `TestUsuario_CadastroValidacoes` (confirma que `"15/03/1995"` continua dando 400) e a suíte completa de integração, todos passando. Cadastro ainda aceita só `"YYYY-MM-DD"`; o frontend recebe exatamente o mesmo formato de antes (`"1995-03-15"`).
- Não há outro `time.Parse` sem `ParseInLocation` no backend (grep confirmou) — bug isolado a este campo.

## 7. `GET /desafios/recomendados` e `GET /usuarios/:id/historico` vazavam o model cru (achado pelo dev-frontend)

Os dois endpoints devolviam `models.Desafio` direto (campos do model Go não têm `json` tag nenhuma,
então marshalavam em PascalCase: `"ID"`, `"Titulo"`, `"ValorEntradaCentavos"` — não o contrato
snake_case que o resto da API de desafios usa).

- **`GET /api/desafios/recomendados`**: `models.DesafioRecomendado.Desafio` passou de `Desafio` (model)
  para `*DesafioResponse` (DTO). `recomendacao_service.go` agora chama `montarResposta(&candidato, userID, nil)`
  — a mesma função que `desafio_service.go` usa para `GET /desafios`/`GET /desafios/:id` — em vez de devolver o
  model direto. `votos: nil` é correto aqui: desafio recomendado está sempre em status `aberto`, nunca em votação.
- **`GET /api/usuarios/:id/historico`**: novo DTO `models.HistoricoDesafioResponse` (snake_case: `id`,
  `titulo`, `status`, `local`, `data_encerramento`, `valor_entrada_centavos`, `valor_entrada`, `vencedor`,
  `premio_centavos`, `premio`, `criado_em`). **Não** reusei `DesafioResponse` aqui de propósito: esse DTO lista
  TODOS os participantes do desafio (saldo/prêmio de cada um), e o histórico é visto por QUALQUER usuário
  autenticado olhando o perfil de um terceiro — expor o resultado financeiro dos outros participantes seria
  vazamento de dados. O histórico expõe só o resultado do usuário **consultado** (`:id` da rota), vindo do
  `DesafioParticipante` dele (`Vencedor`/`PremioCentavos`) — não existe mais um `id_vencedor` único no desafio
  (grupo, múltiplos vencedores possíveis por empate/votação).
  `DesafioRepository.ListarHistoricoUsuario` agora faz `Preload("Participantes", "id_usuario = ?", idUsuario)`
  (só a linha do usuário consultado — o criador também é sempre inserido como participante ao criar o
  desafio, então a linha sempre existe). `UsuarioService.HistoricoDesafios` mudou de assinatura:
  `([]models.Desafio, error)` → `([]models.HistoricoDesafioResponse, error)`.
- **Testes adicionados** em `internal/integration/desafio_test.go` (tag `integration`), executados de
  verdade contra MySQL 8 (Docker local): `TestDesafio_RecomendadosUsaDTOPublico` (confirma `valor_entrada_centavos`
  presente e `ID` PascalCase ausente no JSON) e `TestUsuario_HistoricoExpoeResultadoDoUsuarioConsultado`
  (mesmo desafio, dois usuários — vencedor mostra `vencedor:true, premio_centavos:3000`; perdedor mostra
  `vencedor:false, premio_centavos:0`).

## 4. Melhorias/riscos notados (não corrigidos — fora do escopo desta rodada)

- `CriarAcademiaRequest.Senha` ainda aceita mínimo de 6 caracteres (o padrão novo do cadastro de atleta é 8) — inconsistência pré-existente da branch, não toquei por não fazer parte de nenhum conflito.
- `AcademiaController.Dashboard` usa `FiltroDesafios{Todos: true}` (novo, bypassa a visibilidade) para contar todos os desafios do sistema — correto para um dashboard administrativo, mas se um dia existir "dashboard por academia" (hoje não há vínculo desafio↔academia no schema), vai precisar filtrar por academia, não por "todos".
- `InstrutorController.Agenda` segue sendo um MVP: retorna todos os desafios abertos do sistema (`Todos: true` + `Status: aberto`), não filtrados por `id_instrutor` — já era uma limitação documentada da branch (schema não tem vínculo desafio↔instrutor).
- Nenhuma rota de instrutor exige `RequirePerfil(..., "instrutor")` (só a de academia exige `"academia"`) — qualquer usuário autenticado acessa `GET /api/instrutor/agenda` hoje. Pré-existente, só sinalizando.
- `go.sum` foi inteiramente regerado por `go mod tidy`; se o dev-db ou QA baterem em "checksum mismatch" ao rodar `go build` numa máquina diferente, rodem `go mod tidy` de novo — não há nenhuma dependência nova de produção, só de teste (`testify`, `modernc.org/sqlite`, usadas pelo harness de integração).
- Os testes de integração (`internal/integration/*`, build tag `integration`) não foram executados (exigem `TEST_DB_DSN` com MySQL descartável) — só confirmei que compilam (`go vet -tags integration ./...`). Dependem das migrations novas listadas na seção 2 para passar de fato.

## 6. Bugs de contrato encontrados pelo dev-frontend (para o dev-backend)

Ao resolver os conflitos de `gymScore-frontend/public`, encontrei dois endpoints que devolvem o
model GORM `Desafio` **sem DTO/sem json tags**, em vez de `DesafioResponse`. Sem json tags, o
`encoding/json` do Go serializa com o nome do campo Go inalterado (ex.: `Titulo`, `ValorEntradaCentavos`,
`Status`), não em `snake_case`. O frontend (que já espera `snake_case`, como em todo o resto da API)
recebe tudo `undefined` desses dois endpoints:

- **`GET /api/desafios/recomendados`** (`recomendacao_service.go`, `models.DesafioRecomendado.Desafio`) —
  usado no carrossel "Recomendados para você" de `menu-principal.html`. Hoje mostra título/valor
  "undefined" e o link do card fica `/desafios?id=undefined`.
- **`GET /api/usuarios/:id/historico`** (`usuario_service.go:HistoricoDesafios`, devolve `[]models.Desafio`) —
  usado no histórico de conquistas de `perfil.html`. Além da falta de json tags, o campo `id_vencedor`
  que o frontend espera **não existe mais** no model (o domínio de desafio em grupo guarda o vencedor
  por participante, em `Participantes[].Vencedor`, não um único `id_vencedor` no desafio). Também não há
  `Participantes` populado pelo `ListarHistoricoUsuario` hoje (não confirmei se o preload existe).

Não toquei no backend (fora do meu escopo). No frontend, só coloquei fallbacks (`'Desafio'`, `'Participação
registrada'`, contagem de pódio sempre 0) para a tela não quebrar/mostrar "undefined" enquanto isso não é
corrigido. Sugestão para o dev-backend: estes dois métodos deveriam devolver `DesafioResponse` (ou um DTO
equivalente com json tags em snake_case), e `HistoricoDesafios` precisa de uma forma de marcar "venci este
desafio" compatível com o modelo de múltiplos vencedores (ex.: um campo `venceu bool` calculado a partir de
`Participantes[].Vencedor` para o usuário consultado).
