# QA-REPORT — GymScore Sprint 4 (v7)
Data: 2026-09-19
Avaliação: APROVADO COM RESSALVAS

---

## Build e Testes

- [x] `go build ./...` — PASS (sem erros de compilação)
- [x] `go test ./...` — 9/9 testes passando

**Detalhes:**
- `gynScore-backend/internal/services` — 4 testes (TestCriarUsuario_Sucesso, TestCriarUsuario_EmailInvalido, TestCriarUsuario_EmailDuplicado, TestBuscarUsuarioPorID_NaoEncontrado) — todos PASS
- `gynScore-backend/pkg/utils` — 5 testes (TestValidarEmail_Validos, TestValidarEmail_Invalidos, TestValidarSaldo_Suficiente, TestValidarSaldo_Insuficiente, TestCalcularSaldosAposDesafio) — todos PASS
- Demais pacotes: sem arquivos de teste

---

## Estrutura de Arquivos

### Backend — novos arquivos

- [x] `internal/models/instrutor.go` — presente
- [x] `internal/models/saque.go` — presente
- [x] `internal/models/podio.go` — presente (contém também `AvaliacaoDesafio` e `AvaliarDesafioRequest`)
- [~] `internal/models/avaliacao.go` — AUSENTE (conteúdo incorporado em `podio.go`; funcional mas diverge da nomenclatura do PLANO)
- [x] `internal/repositories/instrutor_repository.go` — presente
- [x] `internal/repositories/saque_repository.go` — presente
- [x] `internal/repositories/podio_repository.go` — presente
- [x] `internal/repositories/avaliacao_repository.go` — presente
- [x] `internal/services/instrutor_service.go` — presente
- [x] `internal/services/saque_service.go` — presente
- [x] `internal/services/recomendacao_service.go` — presente
- [x] `internal/controllers/instrutor_controller.go` — presente
- [x] `internal/controllers/saque_controller.go` — presente
- [x] `internal/controllers/academia_controller.go` — presente
- [x] `internal/controllers/recomendacao_controller.go` — presente
- [x] `internal/middlewares/rbac.go` — presente

### Frontend — novos arquivos

- [x] `html/extrato.html` — presente
- [x] `html/academia-dashboard.html` — presente
- [x] `html/academia-podio.html` — presente
- [x] `html/perfil-publico.html` — presente
- [x] `html/cadastro-instrutor.html` — presente
- [x] `css/extrato.css` — presente
- [x] `css/academia.css` — presente
- [x] `css/perfil-publico.css` — presente
- [x] `css/cadastro-instrutor.css` — presente

---

## Endpoints

- [x] `POST /api/instrutores` — mapeado para `instrutorCtrl.CriarInstrutor`
- [x] `GET /api/usuarios/buscar` — mapeado para `usuarioCtrl.BuscarUsuarios`
- [x] `GET /api/usuarios/:id/publico` — mapeado para `usuarioCtrl.BuscarPublico`
- [x] `GET /api/usuarios/:id/historico` — mapeado para `usuarioCtrl.HistoricoDesafios`
- [x] `PATCH /api/usuarios/ultima-vez` — mapeado para `usuarioCtrl.AtualizarUltimaVez`
- [x] `GET /api/pagamento/extrato` — mapeado para `pixCtrl.Extrato`
- [x] `POST /api/pagamento/saque` — mapeado para `saqueCtrl.SolicitarSaque`
- [x] `GET /api/pagamento/saque` — mapeado para `saqueCtrl.ListarSaques`
- [x] `GET /api/academia/dashboard` — mapeado para `academiaCtrl.Dashboard` (requer perfil academia)
- [x] `GET /api/desafios/:id/presentes` — mapeado para `academiaCtrl.ListarPresentes`
- [x] `POST /api/desafios/:id/presenca` — mapeado para `academiaCtrl.MarcarPresenca` (requer perfil academia)
- [x] `GET /api/desafios/:id/podio` — mapeado para `academiaCtrl.ConsultarPodio`
- [x] `POST /api/desafios/:id/podio` — mapeado para `academiaCtrl.LancarPodio` (requer perfil academia)
- [x] `POST /api/desafios/:id/avaliar` — mapeado para `academiaCtrl.AvaliarDesafio`
- [x] `GET /api/desafios/recomendados` — mapeado para `recomendacaoCtrl.Recomendados`

**Ordenação de rotas:** `/desafios/recomendados` registrada antes de `/desafios/:id` — sem colisão de rota.

---

## Segurança

- [x] `UsuarioPublicoResponse` sem dados sensíveis — confirmado: contém apenas `ID, Nome, Sobrenome, Username, Elo, Pontos, UltimaVez, Perfil`. Ausentes: CPF, Email, Saldo, Senha.
- [x] `BuscarPublico` (controller) retorna `UsuarioPublicoResponse` — `usuarioCtrl.BuscarPublico` delega para `usuarioSvc.BuscarPublico(id)` que retorna `*models.UsuarioPublicoResponse`
- [x] `buscarUsuarios` (service) retorna `[]UsuarioPublicoResponse` — sem dados sensíveis
- [x] RBAC validado — `RequirePerfil` recebe `usuarioRepo` e `perfis ...string`, busca o usuário no banco pelo `user_id` do JWT e verifica com `slices.Contains`. Retorna 401 se não autenticado, 403 se perfil errado.

---

## Regras de Negócio

- [x] **Pódio — exige exatamente 5 posições:** `len(req.Posicoes) != 5` retorna 422. Confirmado em `academia_controller.go:121`.
- [x] **Pódio — idempotente (rejeita duplicata):** `podioRepo.ExistePodio(idDesafio)` é chamado antes de inserir; retorna 409 Conflict se já existe. Confirmado em `academia_controller.go:125-128`.
- [x] **Pódio — credita pontos de Elo:** posições 1-5 recebem 15, 10, 5, 3, 1 pontos respectivamente. Faixa recalculada com `calcularFaixaElo`. Confirmado em `academia_controller.go:130-149`.
- [x] **Saque — valida saldo suficiente:** `usuario.Saldo < req.Valor` retorna erro antes de debitar. Confirmado em `saque_service.go:41-44`.
- [x] **Saque — debita e registra transação como saída:** debita `usuario.Saldo` imediatamente e persiste `Transacao{Tipo: "saida"}` com ID sintético `SAQUE-{userID}-{timestamp}`. Confirmado em `saque_service.go:47-59`.
- [x] **Elo — atualizado no encerramento do desafio:** vencedor recebe +30 pontos, demais participantes +5. Faixa calculada via `calcularElo`. Confirmado em `desafio_service.go:226-241`.
- [x] **Recomendação — não retorna desafios do próprio usuário:** exclui desafios onde `participados[d.ID]` (desafios encerrados) e onde `d.IDCriador == userID`. Confirmado em `recomendacao_service.go:76-80`.
- [x] **Recomendação — campo `razao` não vazio:** nunca é vazio; valor mínimo é `"desafio recente"` (score base +5 adiciona essa razão sempre). Confirmado em `recomendacao_service.go:87-109`.

**Observação:** `calcularElo` está duplicada — implementada como `calcularFaixaElo` em `academia_controller.go` e como `calcularElo` em `desafio_service.go`. As implementações são idênticas. Não causa bug mas representa duplicação de código.

---

## Frontend

- [x] **Login com 3 abas** — `login.html` tem abas Atleta / Academia / Instrutor com `switchTab()`. Redireciona para `/academia` quando `perfil === 'academia'`, senão `/menu`.
- [x] **Carrossel IA com skeleton shimmer** — `menu-principal.html` tem `#recomCarousel` com 3 elementos `.recom-skeleton` e animação `@keyframes shimmer`. `carregarRecomendados()` chama `API.recomendados()`.
- [x] **Saque na carteira** — `depositar.html` tem seção `#stepSaque` com atalhos R$25/50/100/200, campo de chave PIX e botão "Solicitar Saque". Link `/extrato` presente no rodapé.
- [x] **Busca de amigos no endpoint correto** — `amigos.html` função `buscarUsuarios(q)` chama `API.buscarUsuarios(q)` que aponta para `GET /api/usuarios/buscar?q=`. Não usa o endpoint antigo `listarUsuarios`.
- [x] **Share modal no perfil** — `perfil.html` tem `#shareSheet` (bottom-sheet), `#shareCanvas` e função `compartilhar()` com `navigator.share`. Fallback para `copiarLink()` quando Web Share API indisponível. `fecharShare()` presente.
- [x] **`id_usuario` no `gerarPix`** — `api.js` linha 259: `body: JSON.stringify({ valor: valor, cpf: cpf, id_usuario: user ? user.id : 0 })`. Campo presente.

---

## Cobertura de Testes

Cobertura estimada: **15%** (muito baixa)

- Cobertos: `usuario_service` (criação e busca por ID), `pkg/utils` (validação email e saldo)
- Não cobertos: `instrutor_service`, `saque_service`, `recomendacao_service`, `rbac` middleware, `academia_controller`, `desafio_service`, todos os repositories, todos os controllers

O PLANO.md especificava cobrir `instrutor_service`, `saque_service`, `recomendacao_service` e o middleware `rbac`. Nenhum desses testes foi implementado.

---

## AutoMigrate (main.go)

- [x] `&models.Instrutor{}` — presente
- [x] `&models.Saque{}` — presente
- [x] `&models.PodioDesafio{}` — presente
- [x] `&models.AvaliacaoDesafio{}` — presente (definido em `models/podio.go`)

---

## .env.example

- [x] Variáveis críticas presentes: `APP_PORT`, `APP_ENV`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `FRONTEND_PATH`, `JWT_SECRET`, `PIX_CHAVE`, `PIX_NOME_RECEBEDOR`, `PIX_CIDADE_RECEBEDOR`, `ASAAS_ENV`, `ASAAS_API_KEY_SANDBOX`, `ASAAS_API_KEY_PROD`, `ASAAS_WEBHOOK_TOKEN`
- Sem variáveis ausentes detectadas em relação ao código.

---

## Problemas encontrados

### Avisos (não bloqueiam)

| # | Severidade | Camada | Descrição | Arquivo |
|---|-----------|--------|-----------|---------|
| 1 | Amarelo | Backend | `calcularElo` duplicada com nomes diferentes (`calcularElo` e `calcularFaixaElo`), implementações idênticas | `internal/services/desafio_service.go`, `internal/controllers/academia_controller.go` |
| 2 | Amarelo | Backend | Testes faltando para `instrutor_service`, `saque_service`, `recomendacao_service` e middleware `rbac` — previstos no PLANO.md | `internal/services/`, `internal/middlewares/` |
| 3 | Amarelo | Backend | `models/avaliacao.go` ausente como arquivo separado — `AvaliacaoDesafio` foi colocado em `podio.go`. Diverge da nomenclatura do PLANO mas não quebra nada | `internal/models/podio.go` |
| 4 | Amarelo | Frontend | `login.html` linhas 93-95: condição de validação de aba tem bloco `if` interno redundante (if A && B { if A { ... } }) — código morto, não causa bug funcional | `html/login.html` |
| 5 | Amarelo | Frontend | `GET /api/desafios/:id/presentes` e `GET /api/desafios/:id/podio` não exigem RBAC de academia (apenas os POSTs exigem). Qualquer autenticado pode consultar — pode ser intencional mas vale confirmar | `internal/routes/routes.go` |

### Críticos (bloqueiam uso)

Nenhum.

---

## Recomendações

1. Centralizar `calcularElo` em `pkg/utils` e importar nos dois locais, eliminando duplicação.
2. Criar os arquivos de teste faltantes: `instrutor_service_test.go`, `saque_service_test.go`, `recomendacao_service_test.go` e `rbac_test.go` — esses eram itens explícitos do PLANO.md para este sprint.
3. Mover `AvaliacaoDesafio` e `AvaliarDesafioRequest` para um arquivo `internal/models/avaliacao.go` dedicado, mantendo consistência com os demais models.
4. Definir intencionalmente se `GET /api/desafios/:id/presentes` e `GET /api/desafios/:id/podio` devem ser restritos a perfil academia ou permanecer públicos para todos autenticados.
5. Remover a condição `if _activeTab !== perfil` redundante dentro do bloco externo em `login.html`.
