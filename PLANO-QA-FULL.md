# PLANO — Rodada de correção e QA completa (todas as telas)
Data: 2026-09-27
Diretório: /cod/gym-score

## Contexto
Projeto GymScore já tem Sprints 1-5 codados (backend Go/Fiber completo, IA de
recomendação via Anthropic já implementada, ~25 telas HTML/CSS/JS). Há um
volume grande de mudanças não commitadas desde o último commit (aed30f9),
incluindo trabalho em andamento em vários controllers/services e telas
(amigos, criar-conta, depositar, desafios-detalhado, login, menu-principal,
perfil). Backend compila limpo (`go build ./...`) e sobe conectando no MySQL
remoto (RDS) sem erro — `/health` responde 200.

Decisão do usuário: manter a IA de recomendação em modo heurístico por ora
(sem ANTHROPIC_API_KEY no .env) — o fallback já é parte do design e funciona
100% sem a chave. Não bloquear a entrega por isso.

Objetivo desta rodada: garantir que TODAS as telas funcionem de ponta a ponta
(especialmente perfil: ver dados do usuário, logout) e fazer QA completo com
navegador antes de entregar.

## Arquitetura
- Stack: Go 1.21 + Fiber + GORM + MySQL (backend) | HTML/CSS/JS puro (frontend, sem framework)
- Backend: `gymScore-backend/internal/{controllers,services,repositories,models,middlewares}` + `routes/` + `pkg/utils`
- Frontend: `gymScore-frontend/public/{html,css,js}` — `js/api.js` é o único módulo compartilhado (Auth, Privacy, API, helpers)
- Backend serve o frontend como estático (`FRONTEND_PATH`) + rotas de clean URL (`/perfil`, `/login` etc. em `routes.go`)
- Padrão de resposta: `{success, message, data}`

## DEV_BACKEND
- [x] Revisar os diffs não commitados — sem trabalho pela metade, wiring de DI 100% coerente
- [x] Confirmar que `GET /api/usuarios/:id` retorna nome, sobrenome, elo, pontos, criado_em, perfil — confirmado
- [x] Rodar `go test ./...` e `go vet ./...` — 100% limpo
- [x] Checar RBAC — só 3 rotas restritas a perfil "academia", nenhuma bloqueia atleta comum
- [x] Garantir mensagens de erro reais — corrigido em usuario_controller.go (Ingressos) e pix_controller.go (ConsultarPagamento)
- [x] (achado em QA) Criar suporte a cadastro de academia — model `Academia`, `POST /api/academias`, CPF/DataNascimento/Genero nullable
- [x] (achado em QA) Adicionar elo/pontos a `AmigoResponse`
- [x] (achado em QA) Unificar escala de Elo (6 níveis) entre `CalcularElo` e o frontend, derivar sempre de `Pontos` nas respostas

## DEV_FRONTEND
- [x] Auditar TODAS as telas em `gymScore-frontend/public/html/*.html` quanto a: chamadas de API corretas (bater com `routes.go`), tratamento de erro (try/catch com mensagem real, nunca tela quebrada em branco), navegação (links/bottom nav) funcionando entre todas as telas
- [x] Tela de perfil (`perfil.html`): confirmar que os dados exibidos batem com o usuário logado (hoje usa `Auth.getUser()` do localStorage — avaliar se deve re-buscar via `GET /api/usuarios/:id` para refletir Elo/Pontos atualizados) e que "SAIR DA CONTA" (`Auth.logout()`) funciona em todas as situações — implementado `Auth.refresh()` em api.js, usado em perfil.html/menu-principal.html/depositar.html
- [x] Garantir que toda tela autenticada tem acesso a logout (via bottom nav ou botão em perfil) — não deixar telas "sem saída"
- [x] Corrigir especificamente as telas com diff pendente: `amigos.html`, `criar-conta.html`, `depositar.html`, `desafios-detalhado.html`, `login.html`, `menu-principal.html`, `perfil.html` — terminar qualquer integração pela metade (amigos/desafios-detalhado/login já estavam completos; criar-conta/depositar/menu-principal/perfil corrigidos)
- [x] Revisar as telas por perfil (academia-*, instrutor-*, cadastro-*) para garantir que redirecionam corretamente conforme `perfil` do usuário logado — guard adicionado nas 7 telas academia-*/instrutor-*; cadastro-academia.html tem gap de contrato no backend (ver mensagem ao tech-lead / QA-REPORT)
- [x] Não usar `npm` — projeto é HTML/JS puro servido estático, sem build step

## QA
- [x] Subir o backend (`go run main.go` a partir de `gymScore-backend/`) e validar `/health`
- [x] Rodar `go test ./...` novamente como gate final — 100% passando
- [x] Gerar `QA-REPORT-FULL.md` na raiz cobrindo: build, testes automatizados, e checklist manual de cada tela testada
- [x] Reportar qualquer bug encontrado para fix imediato antes da entrega — 8 bugs corrigidos, ver QA-REPORT-FULL.md

## QA — Navegador (feito pelo tech-lead diretamente, via claude-in-chrome)
- [x] Fluxo completo: criar conta → login → menu principal → perfil (ver dados, logout) → login novamente → desafios → amigos → depositar → outras telas relevantes — 16 telas testadas ao vivo, ver checklist no QA-REPORT-FULL.md
- [x] Checar console do navegador em cada tela (sem erros JS não tratados) — 1 crash encontrado e corrigido (menu-principal.html)
- [x] Checar requests de rede (sem 4xx/5xx inesperados) — PIX 400 real (chave Asaas inválida, config, não bug de código)

## Achados extras não previstos no plano original (ver QA-REPORT-FULL.md para detalhes)
- [x] Elo com escalas divergentes entre backend e frontend — unificado
- [x] Cadastro de academia 100% quebrado — endpoint dedicado criado, tela religada
- [x] Botão "GARANTIR VAGA" inacessível por sobreposição de z-index — corrigido
- [x] Mensagens de erro/status invisíveis por bug de CSS inline (~12 telas, incluindo login) — corrigido
- [x] Sessão expirada não desloga o usuário (401 sem tratamento) — corrigido
