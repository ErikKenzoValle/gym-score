package routes

import (
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/controllers"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// Controllers agrupa os controllers registrados nas rotas.
type Controllers struct {
	Usuario      *controllers.UsuarioController
	Desafio      *controllers.DesafioController
	Amizade      *controllers.AmizadeController
	PIX          controllers.PIXController
	Treino       *controllers.TreinoController
	Webhook      *controllers.WebhookController
	Carteira     *controllers.CarteiraController
	Notificacao  *controllers.NotificacaoController
	Ranking      *controllers.RankingController
	Instrutor    *controllers.InstrutorController
	Academia     *controllers.AcademiaController
	Recomendacao *controllers.RecomendacaoController
	Plataforma   *controllers.PlataformaController
	// Sessoes confere o claim `tv` do JWT com o banco (revogação)
	Sessoes middlewares.VersaoTokenReader
	// UsuarioRepo alimenta o middleware de RBAC (RequirePerfil)
	UsuarioRepo repositories.UsuarioRepository
	// Admin / AdminEstados: painel admin (/api/admin) e a validação do token admin (versão/ativo/papel)
	Admin        *controllers.AdminController
	AdminEstados middlewares.AdminEstadoReader
	// Moderacao: denúncias, punições e recursos (usuário, academia e o recurso público do banido)
	Moderacao *controllers.ModeracaoController
}

// Setup registra todas as rotas da aplicação no servidor Fiber
func Setup(app *fiber.App, cfg *config.Config, h Controllers) {
	fp := cfg.FrontendPath
	htmlDir := fp + "/html/"

	// Rotas de páginas HTML (clean URLs)
	app.Get("/login", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "login.html") })
	app.Get("/signup", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "criar-conta.html") })
	app.Get("/menu", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "menu-principal.html") })
	app.Get("/desafios", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "desafios-detalhado.html") })
	app.Get("/amigos", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "amigos.html") })
	app.Get("/perfil", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "perfil.html") })
	app.Get("/alterar-perfil", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "alterar-perfil.html") })
	app.Get("/alterar-senha", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "alterar-senha.html") })
	app.Get("/esqueci-senha", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "esqueci-senha.html") })
	app.Get("/redefinir-senha", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "redefinir-senha.html") })
	app.Get("/privacidade", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "privacidade.html") })
	app.Get("/treinos", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "treinos.html") })
	app.Get("/carteira", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "carteira.html") })
	app.Get("/depositar", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "depositar.html") })
	app.Get("/extrato", func(c *fiber.Ctx) error { return c.Redirect("/carteira", fiber.StatusFound) })
	app.Get("/academia", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-dashboard.html") })
	app.Get("/academia/podio", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-podio.html") })
	app.Get("/academia/denuncias", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-denuncias.html") })
	app.Get("/denunciar", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "denunciar.html") })
	app.Get("/minhas-denuncias", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "minhas-denuncias.html") })
	app.Get("/minhas-punicoes", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "minhas-punicoes.html") })
	app.Get("/perfil-publico", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "perfil-publico.html") })
	app.Get("/cadastro-instrutor", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "cadastro-instrutor.html") })
	app.Get("/signup-academia", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "cadastro-academia.html") })
	app.Get("/ingressos", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "ingressos.html") })
	app.Get("/academia/criar-desafio", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-criar-desafio.html") })
	app.Get("/academia/instrutores", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-instrutores.html") })
	app.Get("/instrutor", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "instrutor-agenda.html") })
	app.Get("/instrutor/checkin", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "instrutor-checkin.html") })
	app.Get("/instrutor/podio", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "instrutor-podio.html") })
	// Sprint 6: gestão (academia lista/candidatos; instrutor vagas e ganhos são abas da agenda)
	app.Get("/academia/desafios", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-desafios.html") })
	app.Get("/academia/candidatos", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "academia-candidatos.html") })
	app.Get("/instrutor/vagas", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "instrutor-agenda.html") })
	app.Get("/instrutor/ganhos", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "instrutor-agenda.html") })

	// Painel admin (P12): páginas e arquivos sob /admin com CSP restritiva (sem inline), sem frames e
	// sem cache. Middleware global (decide pelo caminho decodificado/normalizado, não pelo prefixo cru)
	// e antes do Static, para valer também para /admin/js, /admin/css e formas como /%61dmin/.
	app.Use(middlewares.AdminPaginasHeadersPorCaminho())
	app.Get("/admin/login", func(c *fiber.Ctx) error { return c.SendFile(fp + "/admin/login.html") })
	app.Get("/admin", func(c *fiber.Ctx) error { return c.SendFile(fp + "/admin/index.html") })

	// Fotos de check-in: nomes aleatórios de 128 bits; nosniff evita interpretar como HTML
	app.Use("/uploads", func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		return c.Next()
	})
	app.Static("/uploads", cfg.UploadsDir, fiber.Static{Browse: false, Index: "__sem_index__"})

	// Arquivos estáticos (css, js, img)
	app.Static("/", fp, fiber.Static{Index: "index.html", Browse: false})

	// Rota de health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "GymScore API",
			"version":     "1.0.0",
			"asaas_env":   cfg.AsaasEnv,
			"asaas_teste": cfg.IsAsaasSandbox(),
		})
	})

	// Grupo de rotas da API
	api := app.Group("/api")

	// ─── Rotas públicas (sem autenticação) ───────────────────────────────────────

	// Rate limit por IP nas rotas sensíveis (RATE_LIMIT_MAX/min; 0 desativa). Respostas de autenticação
	// nunca vão para cache (semCache). Logout só por POST: nenhuma rota GET altera a sessão.
	semCache := middlewares.SemCache()
	// Login e recurso do banido conferem senha: compartilham o MESMO contador por IP (R3-1).
	limiteSenha := middlewares.RateLimitMiddleware(cfg.RateLimitMax)
	api.Post("/login", semCache, limiteSenha, h.Usuario.Login)
	api.Post("/logout", semCache, middlewares.CSRFMiddleware(cfg), h.Usuario.Logout)
	api.Post("/usuarios", semCache, middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.CriarUsuario)
	api.Post("/auth/esqueci-senha", semCache, middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.EsqueciSenha)
	api.Post("/auth/redefinir-senha", semCache, middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.RedefinirSenha)

	// Webhook Asaas — DEVE ficar antes do grupo protected para não herdar o JWT middleware.
	// Responde 404 quando ASAAS_WEBHOOK_TOKEN está vazio.
	api.Post("/webhooks/asaas", h.Webhook.ReceberWebhookAsaas)

	// Recurso do banido (Q1): sem sessão; reconfere e-mail/senha a cada chamada (rate limit por IP compartilhado com /api/login).
	if h.Moderacao != nil {
		api.Post("/recursos/banimento", semCache, limiteSenha, h.Moderacao.RecursoBanimento)
	}

	// Academias — cadastro público (nasce pendente de aprovação do admin)
	api.Post("/academias", middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Academia.CriarAcademia)

	// ─── Painel admin (/api/admin) ───────────────────────────────────────────────
	// Registrado ANTES do grupo protegido de usuários: o token de usuário não vale aqui e vice-versa.
	if h.Admin != nil {
		setupAdmin(api, cfg, h)
	}

	// ─── Rotas protegidas (requerem JWT) ─────────────────────────────────────────
	// CSRF antes da autenticação: requisição que muda estado autenticada só pelo cookie precisa de
	// Origin/Referer confiável (ver CSRFMiddleware).
	csrf := middlewares.CSRFMiddleware(cfg)
	protected := api.Group("", csrf, middlewares.AuthMiddleware(cfg, h.Sessoes))
	// susp: ações de participação/gestão recusadas à conta suspensa (403 conta_suspensa — Q2). Leitura,
	// carteira, notificações, recurso e as próprias denúncias continuam liberados.
	susp := middlewares.BloquearSuspenso()

	// Usuários — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/usuarios/me", semCache, h.Usuario.Me)
	// Duas buscas distintas: "busca" exclui o próprio usuário (ex.: adicionar amigos);
	// "buscar" é a busca geral por nome/username (sem exclusão).
	protected.Get("/usuarios/busca", h.Usuario.BuscarUsuarios)
	protected.Get("/usuarios/buscar", h.Usuario.BuscarPorTermo)
	protected.Get("/usuarios/ingressos", h.Usuario.Ingressos)
	protected.Patch("/usuarios/ultima-vez", h.Usuario.AtualizarUltimaVez)
	protected.Patch("/usuarios/perfil", h.Usuario.AtualizarPerfil)
	protected.Patch("/usuarios/senha", semCache, h.Usuario.AlterarSenha)
	protected.Get("/usuarios/:id/publico", h.Usuario.BuscarPublico)
	protected.Get("/usuarios/:id/historico", h.Usuario.HistoricoDesafios)
	protected.Get("/usuarios/:id", h.Usuario.BuscarUsuario)

	// Desafios — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/desafios", h.Desafio.ListarDesafios)
	protected.Get("/desafios/meus", h.Desafio.ListarMeus)
	protected.Get("/desafios/categorias", h.Desafio.Categorias)
	protected.Get("/desafios/recomendados", h.Recomendacao.Recomendados)
	protected.Post("/desafios", susp, h.Desafio.CriarDesafio) // só perfil academia (checado no service)
	protected.Get("/desafios/:id", h.Desafio.BuscarDesafio)
	protected.Patch("/desafios/:id", susp, h.Desafio.EditarDesafio)
	protected.Post("/desafios/:id/entrar", susp, h.Desafio.Entrar())
	protected.Post("/desafios/:id/sair", h.Desafio.Sair())
	protected.Post("/desafios/:id/cancelar", susp, h.Desafio.Cancelar())
	protected.Post("/desafios/:id/iniciar", susp, h.Desafio.Iniciar())
	protected.Post("/desafios/:id/convidar", susp, h.Desafio.Convidar)
	protected.Post("/desafios/:id/presenca", susp, h.Desafio.MarcarPresenca)
	protected.Get("/desafios/:id/presentes", h.Desafio.ListarPresentes)
	protected.Get("/desafios/:id/podio", h.Desafio.ConsultarPodio)
	protected.Post("/desafios/:id/podio", susp, h.Desafio.LancarPodio)
	protected.Post("/desafios/:id/avaliar", susp, h.Desafio.AvaliarDesafio)
	// Vaga de instrutor (leilão com teto): candidatura, retirada, lista (academia dona) e escolha
	protected.Post("/desafios/:id/candidaturas", susp, h.Desafio.Candidatar)
	protected.Delete("/desafios/:id/candidaturas", h.Desafio.RetirarCandidatura)
	protected.Get("/desafios/:id/candidaturas", h.Desafio.ListarCandidaturas)
	protected.Post("/desafios/:id/candidaturas/:cid/escolher", susp, h.Desafio.EscolherCandidatura)

	// Taxas vigentes da plataforma (máximo do pote, tarifa de saque)
	protected.Get("/plataforma/taxas", h.Plataforma.Taxas)

	// Carteira
	protected.Get("/carteira", h.Carteira.Saldo)
	protected.Get("/carteira/extrato", h.Carteira.Extrato)
	protected.Get("/carteira/pendentes", h.Carteira.Pendentes)
	protected.Get("/carteira/debitos", h.Carteira.Debitos)
	protected.Post("/carteira/saque", h.Carteira.Sacar)
	protected.Get("/carteira/saque/:id", h.Carteira.ConsultarSaque)

	// Notificações — rotas estáticas antes das com :id
	protected.Get("/notificacoes", h.Notificacao.Listar)
	protected.Get("/notificacoes/nao-lidas", h.Notificacao.NaoLidas)
	protected.Post("/notificacoes/lidas", h.Notificacao.MarcarTodasLidas)
	protected.Post("/notificacoes/:id/lida", h.Notificacao.MarcarLida)

	// Ranking
	protected.Get("/ranking/amigos", h.Ranking.Amigos)

	// Amigos (sempre do usuário do token)
	protected.Get("/amigos", h.Amizade.ListarAmigos)
	protected.Post("/amigos/adicionar", susp, h.Amizade.AdicionarAmigo())
	protected.Post("/amigos/aceitar", susp, h.Amizade.AceitarAmizade())
	protected.Post("/amigos/recusar", h.Amizade.RecusarAmizade())
	protected.Post("/amigos/remover", h.Amizade.RemoverAmigo())

	// Pagamento PIX (Depósito de Saldo)
	protected.Post("/pagamento/pix", h.PIX.GerarPagamento)
	protected.Get("/pagamento/pix/:asaas_id", h.PIX.ConsultarPagamento)
	protected.Post("/pagamento/pix/:asaas_id/simular", h.PIX.SimularPagamento)

	// Academia — tudo escopado pela academia do JWT (RequirePerfil + service)
	soAcademia := middlewares.RequirePerfil(h.UsuarioRepo, "academia")
	protected.Get("/academia/dashboard", soAcademia, h.Academia.Dashboard)
	protected.Get("/academia/desafios", soAcademia, h.Academia.ListarDesafios)
	protected.Get("/academia/instrutores", soAcademia, h.Academia.ListarInstrutores)
	// POST envia um CONVITE de vínculo; o instrutor aceita/recusa em /instrutor/convite
	protected.Post("/academia/instrutores", soAcademia, susp, h.Academia.VincularInstrutor)
	protected.Get("/academia/instrutores/convites", soAcademia, h.Academia.ListarConvites)
	protected.Delete("/academia/instrutores/convites/:id", soAcademia, h.Academia.CancelarConvite)
	protected.Delete("/academia/instrutores/:id", soAcademia, h.Academia.DesvincularInstrutor)

	// Moderação — denúncias próprias, punições e recurso; academia julgadora (jurisdição dela)
	if m := h.Moderacao; m != nil {
		protected.Post("/denuncias", susp, m.CriarDenuncia)
		protected.Get("/denuncias/minhas", semCache, m.MinhasDenuncias)
		// Única rota com corpo grande (multipart): o limite próprio está em LimiteCorpo (app.go).
		protected.Post("/denuncias/:id/evidencias", m.EnviarEvidencia)
		protected.Get("/denuncias/:id/evidencias/:ordem", m.EvidenciaDoDenunciante)
		protected.Get("/denuncias/:id", semCache, m.MinhaDenuncia)
		protected.Get("/punicoes/minhas", semCache, m.MinhasPunicoes)
		protected.Post("/punicoes/:id/recurso", m.AbrirRecurso)

		protected.Get("/academia/denuncias", soAcademia, semCache, m.FilaAcademia)
		protected.Get("/academia/denuncias/:id", soAcademia, semCache, m.DenunciaAcademia)
		protected.Get("/academia/denuncias/:id/evidencias/:ordem", soAcademia, m.EvidenciaAcademia)
		protected.Post("/academia/denuncias/:id/assumir", soAcademia, susp, m.AssumirAcademia)
		protected.Post("/academia/denuncias/:id/encaminhar", soAcademia, susp, m.EncaminharAcademia)
		protected.Post("/academia/denuncias/:id/decidir", soAcademia, susp, m.DecidirAcademia)
		protected.Get("/academia/punicoes", soAcademia, semCache, m.PunicoesAcademia)
	}

	// Instrutor — o usuário logado ativa o próprio perfil (sem id_usuario/id_academia no corpo)
	protected.Post("/instrutores", susp, h.Instrutor.CriarInstrutor)
	protected.Get("/instrutores", h.Instrutor.Listar)
	protected.Get("/instrutor/agenda", middlewares.RequirePerfil(h.UsuarioRepo, "instrutor"), h.Instrutor.Agenda)
	protected.Get("/instrutor/vagas", middlewares.RequirePerfil(h.UsuarioRepo, "instrutor"), h.Desafio.ListarVagas)
	// O próprio instrutor encerra o vínculo com a academia (vale também com cadastro pendente)
	protected.Delete("/instrutor/vinculo", h.Instrutor.DesvincularAcademia)
	// Convite de vínculo enviado por uma academia (vale também com cadastro pendente)
	protected.Get("/instrutor/convite", h.Instrutor.Convite)
	protected.Post("/instrutor/convite/aceitar", susp, h.Instrutor.AceitarConvite)
	protected.Post("/instrutor/convite/recusar", h.Instrutor.RecusarConvite)

	// Treinos — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/treinos/radar", h.Treino.Radar)
	// código de 6 caracteres: limita tentativas de adivinhação
	protected.Post("/treinos/importar", middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Treino.ImportarTreino)
	protected.Post("/treinos/concluir", h.Treino.ConcluirExercicio)
	protected.Get("/treinos", h.Treino.ListarTreinos)
	protected.Post("/treinos", h.Treino.CriarTreino)
	protected.Get("/treinos/:id", h.Treino.BuscarTreino)
	protected.Delete("/treinos/:id", h.Treino.DeletarTreino)
}

// limiteLoginAdmin é o rate limit (por IP/minuto) do login admin. Independe de RATE_LIMIT_MAX (que
// pode ser 0 para desligar o limite geral): vem de ADMIN_LOGIN_RATE_LIMIT_MAX e nunca é desligado —
// valor ausente, zero ou negativo vira o piso config.AdminLoginRateLimitPadrao (5).
func limiteLoginAdmin(cfg *config.Config) int {
	if cfg.AdminLoginRateLimitMax <= 0 {
		return config.AdminLoginRateLimitPadrao
	}
	return cfg.AdminLoginRateLimitMax
}

// setupAdmin registra a API do painel admin. Autenticação própria (ADMIN_JWT_SECRET, aud, cookie
// admin_jwt), checagem de origem em toda requisição não-GET sem Authorization e ações de dinheiro e de
// gestão de admins restritas a superadmin.
func setupAdmin(api fiber.Router, cfg *config.Config, h Controllers) {
	a := h.Admin
	adm := api.Group("/admin", middlewares.AdminAPIHeaders(), middlewares.AdminCSRFMiddleware(cfg))
	adm.Post("/auth/login", middlewares.RateLimitMiddleware(limiteLoginAdmin(cfg)), a.Login)

	auth := adm.Group("", middlewares.AdminAuthMiddleware(cfg, h.AdminEstados))
	super := middlewares.RequireSuperadmin()

	auth.Post("/auth/logout", a.Logout)
	auth.Get("/auth/me", a.Me)
	auth.Post("/auth/senha", a.TrocarSenha)

	auth.Get("/dashboard", a.Dashboard)
	auth.Get("/transacoes", a.ListarTransacoes)
	auth.Get("/transacoes/:id", a.BuscarTransacao)
	auth.Get("/receitas", a.ListarReceitas)
	auth.Get("/reservas", a.ListarReservas)

	auth.Get("/saques", a.ListarSaques)
	auth.Post("/saques/:id/reconciliar", a.ReconciliarSaque)
	auth.Post("/saques/:id/reembolsar", super, a.ReembolsarSaque)
	auth.Post("/saques/:id/marcar-pago", super, a.MarcarSaquePago)

	auth.Get("/usuarios", a.ListarUsuarios)
	auth.Get("/usuarios/:id", a.BuscarUsuario)
	auth.Post("/usuarios/:id/bloquear", a.BloquearUsuario)
	auth.Post("/usuarios/:id/desbloquear", a.DesbloquearUsuario)

	auth.Get("/desafios", a.ListarDesafios)
	auth.Get("/desafios/:id", a.BuscarDesafio)
	auth.Post("/desafios/:id/cancelar", a.CancelarDesafio)
	auth.Post("/desafios/:id/anular-resultado", super, a.AnularResultado) // mexe em dinheiro: superadmin

	auth.Get("/academias", a.ListarAcademias)
	auth.Post("/academias/:id/aprovar", a.AprovarAcademia)
	auth.Post("/academias/:id/rejeitar", a.RejeitarAcademia)
	auth.Get("/instrutores", a.ListarInstrutores)
	auth.Post("/instrutores/:id/aprovar", a.AprovarInstrutor)
	auth.Post("/instrutores/:id/rejeitar", a.RejeitarInstrutor)

	// Moderação: denúncias, punições, recursos e linha do tempo (escrita sempre com motivo + auditoria)
	auth.Get("/denuncias", a.ListarDenuncias)
	auth.Get("/denuncias/:id", a.BuscarDenuncia)
	auth.Get("/denuncias/:id/evidencias/:ordem", a.EvidenciaDenuncia)
	auth.Post("/denuncias/:id/assumir", a.AssumirDenuncia)
	auth.Post("/denuncias/:id/decidir", a.DecidirDenuncia)
	auth.Get("/punicoes", a.ListarPunicoes)
	auth.Post("/punicoes", a.AplicarPunicao)
	auth.Post("/punicoes/:id/revogar", a.RevogarPunicao)
	auth.Get("/recursos", a.ListarRecursos)
	auth.Post("/recursos/:id/julgar", a.JulgarRecurso)
	auth.Get("/moderacao/eventos", a.ListarEventosModeracao)
	auth.Get("/pagamentos-pendentes", a.ListarPagamentosPendentes)
	auth.Get("/debitos", a.ListarDebitos)

	auth.Get("/auditoria", a.ListarAuditoria)

	auth.Get("/administradores", super, a.ListarAdmins)
	auth.Post("/administradores", super, a.CriarAdmin)
	auth.Post("/administradores/:id/desativar", super, a.DesativarAdmin)
	auth.Post("/administradores/:id/reativar", super, a.ReativarAdmin)
	auth.Post("/administradores/:id/papel", super, a.TrocarPapelAdmin)
	auth.Post("/administradores/:id/resetar-senha", super, a.ResetarSenhaAdmin)

	// Rota admin inexistente: 404 aqui (sem cair no grupo de usuários, que responderia 401).
	adm.Use(func(c *fiber.Ctx) error { return utils.Error(c, fiber.StatusNotFound, "Rota não encontrada") })
}
