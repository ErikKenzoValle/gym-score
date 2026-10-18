# PLANO — GymScore v7 (Sprint 4)
Data: 2026-09-19
Diretório: /cod/gym-score

## Arquitetura
- Stack: Go 1.21 + Fiber + GORM + MySQL (backend) | HTML/CSS/JS puro (frontend)
- Backend: `gymScore-backend/internal/{controllers,services,repositories,models,middlewares,routes}` + `pkg/utils`
- Frontend: `gymScore-frontend/public/{html,css,js}`
- Padrão de resposta: `{success, message, data}` em todos os endpoints
- Auth: JWT HMAC-SHA256, claims `{UserID, Email}` — acrescentar `Perfil` no response do login
- Migrations: auto-migração GORM em `main.go` — idempotente por natureza

## DEV_DB
- [x] Adicionar colunas em `usuarios`: Perfil (enum atleta/academia/instrutor), Username (varchar 50 unique nullable), Elo (varchar 20 default 'Bronze'), Pontos (int default 0), UltimaVez (timestamp nullable)
- [x] Adicionar coluna em `desafio_participantes`: Presente (bool default false)
- [x] Adicionar coluna em `transacoes`: Tipo (enum entrada/saida default entrada)
- [x] Criar tabela `instrutores`: id, id_usuario (FK unique), cref, especialidade, id_academia (nullable)
- [x] Criar tabela `saques`: id, id_usuario (FK), valor, chave_pix, status (pendente/processando/pago/rejeitado), criado_em, atualizado_em
- [x] Criar tabela `podio_desafios`: id, id_desafio (FK), posicao (1-5), id_participante (FK), marca (varchar), criado_em; unique(id_desafio, posicao)
- [x] Criar tabela `avaliacoes_desafios`: id, id_desafio (FK), id_usuario (FK), nota (1-5), criado_em; unique(id_desafio, id_usuario)
- [x] Atualizar `main.go` AutoMigrate com novos models

## DEV_BACKEND
- [x] Criar `internal/middlewares/rbac.go` — RequirePerfil(perfis ...string) middleware
- [x] Atualizar `LoginResponse` para incluir campo Perfil — roteamento no frontend
- [x] Criar `internal/models/instrutor.go` + `internal/repositories/instrutor_repository.go` + `internal/services/instrutor_service.go` + `internal/controllers/instrutor_controller.go`
- [x] Endpoint POST `/api/instrutores` (cadastro público de instrutor)
- [x] Atualizar `usuario_service.Buscar/Listar` para incluir Elo, Pontos, UltimaVez nos responses
- [x] Endpoint GET `/api/usuarios/buscar?q=` — busca por nome ou username (autenticado)
- [x] Endpoint GET `/api/usuarios/:id/publico` — perfil público (sem dados sensíveis)
- [x] Endpoint GET `/api/usuarios/:id/historico` — histórico de desafios encerrados
- [x] Endpoint PATCH `/api/usuarios/ultima-vez` — heartbeat de presença
- [x] Criar `internal/models/saque.go` + repo + service + controller
- [x] Endpoint POST `/api/pagamento/saque` — solicitar saque (cria registro pendente)
- [x] Endpoint GET `/api/pagamento/saque` — listar saques do usuário
- [x] Endpoint GET `/api/pagamento/extrato` — entradas + saídas + totais
- [x] Criar `internal/models/podio.go` + `internal/models/avaliacao.go` + repos + services
- [x] Criar `internal/controllers/academia_controller.go`
- [x] Endpoint GET `/api/academia/dashboard` — indicadores (requer perfil academia)
- [x] Endpoint GET `/api/desafios/:id/presentes` — lista participantes presentes
- [x] Endpoint POST `/api/desafios/:id/presenca` — marcar participante presente (academia)
- [x] Endpoint POST `/api/desafios/:id/podio` — lançar pódio Top 5 (academia)
- [x] Endpoint GET `/api/desafios/:id/podio` — consultar pódio
- [x] Endpoint POST `/api/desafios/:id/avaliar` — avaliar desafio (atleta)
- [x] Criar `internal/services/recomendacao_service.go` + controller
- [x] Endpoint GET `/api/desafios/recomendados` — retorna até 5 desafios com campo `razao`
- [x] Atualizar `desafio_service.EncerrarDesafio` — calcular Elo (+30 vencedor, +5 participantes)
- [x] Faixas de Elo: 0-99=Bronze, 100-249=Prata, 250-499=Ouro, 500-999=Diamante, 1000+=Elite
- [x] Atualizar `internal/routes/routes.go` com todas as novas rotas
- [x] Atualizar Swagger (swag init ou anotações nos controllers)
- [x] Testes: cobrir instrutor_service, saque_service, recomendacao_service, rbac middleware

## DEV_FRONTEND
- [x] Atualizar `html/login.html` — 3 abas (Atleta / Academia / Instrutor), após login redirecionar por perfil
- [x] Criar `html/cadastro-instrutor.html` + `css/cadastro-instrutor.css` — formulário com CREF, especialidade
- [x] Reestruturar `html/depositar.html` — saldo no topo, atalhos R$25/50/100/200, saque, link extrato, QR PIX separado
- [x] Criar `html/extrato.html` + `css/extrato.css` — totalizadores + lista com ↓verde/↑vermelho
- [x] Criar `html/academia-dashboard.html` + `css/academia.css` — cards indicadores, avaliação média
- [x] Criar `html/academia-podio.html` — seleção de presentes, marca por posição, botão habilitar no Top 5 completo
- [x] Atualizar `html/amigos.html` — busca tempo real, online (bolinha verde), Elo colorido, contador pódios, botão Adicionar/Enviado
- [x] Criar `html/perfil-publico.html` + `css/perfil-publico.css` — somente leitura, histórico de desafios do amigo
- [x] Atualizar `html/menu-principal.html` — carrossel "✨ Recomendados para Você" com skeleton/shimmer ~2s
- [x] Atualizar `html/perfil.html` — botão compartilhar por conquista, bottom-sheet modal, canvas card, Web Share API + clipboard
- [x] Atualizar `js/api.js` — adicionar todos os novos métodos de API

## QA — APROVADO COM RESSALVAS (2026-09-19)
- [x] go test ./... — 16/16 testes passando
- [x] AutoMigrate com 4 novos models
- [x] 15 novos endpoints verificados
- [x] Segurança: UsuarioPublicoResponse sem dados sensíveis
- [x] RBAC validado
- [x] Regras de negócio: pódio, saque, Elo, recomendação
- [x] Frontend: login 3 abas, carrossel IA, saque, busca amigos, share modal
- [x] QA-REPORT.md gerado
- Ressalvas restantes: GET /presentes e /podio sem RBAC (intencional — leitura pública)
