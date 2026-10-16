package routes

import (
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/controllers"
	"gymScore-backend/internal/middlewares"

	"github.com/gofiber/fiber/v2"
)

// Controllers agrupa os controllers registrados nas rotas.
type Controllers struct {
	Usuario     *controllers.UsuarioController
	Desafio     *controllers.DesafioController
	Amizade     *controllers.AmizadeController
	PIX         controllers.PIXController
	Treino      *controllers.TreinoController
	Webhook     *controllers.WebhookController
	Carteira    *controllers.CarteiraController
	Notificacao *controllers.NotificacaoController
	Ranking     *controllers.RankingController
	// Sessoes confere o claim `tv` do JWT com o banco (revogação)
	Sessoes middlewares.VersaoTokenReader
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

	// Rate limit por IP nas rotas sensíveis (RATE_LIMIT_MAX/min; 0 desativa)
	api.Post("/login", middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.Login)
	api.Post("/logout", h.Usuario.Logout)
	api.Post("/usuarios", middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.CriarUsuario)
	api.Post("/auth/esqueci-senha", middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.EsqueciSenha)
	api.Post("/auth/redefinir-senha", middlewares.RateLimitMiddleware(cfg.RateLimitMax), h.Usuario.RedefinirSenha)

	// Webhook Asaas — DEVE ficar antes do grupo protected para não herdar o JWT middleware.
	// Responde 404 quando ASAAS_WEBHOOK_TOKEN está vazio.
	api.Post("/webhooks/asaas", h.Webhook.ReceberWebhookAsaas)

	// ─── Rotas protegidas (requerem JWT) ─────────────────────────────────────────
	protected := api.Group("", middlewares.AuthMiddleware(cfg, h.Sessoes))

	// Usuários — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/usuarios/me", h.Usuario.Me)
	protected.Get("/usuarios/busca", h.Usuario.BuscarUsuarios)
	protected.Patch("/usuarios/perfil", h.Usuario.AtualizarPerfil)
	protected.Patch("/usuarios/senha", h.Usuario.AlterarSenha)
	protected.Get("/usuarios/:id", h.Usuario.BuscarUsuario)

	// Desafios — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/desafios", h.Desafio.ListarDesafios)
	protected.Get("/desafios/meus", h.Desafio.ListarMeus)
	protected.Post("/desafios", h.Desafio.CriarDesafio)
	protected.Get("/desafios/:id", h.Desafio.BuscarDesafio)
	protected.Post("/desafios/:id/entrar", h.Desafio.Entrar())
	protected.Post("/desafios/:id/sair", h.Desafio.Sair())
	protected.Post("/desafios/:id/cancelar", h.Desafio.Cancelar())
	protected.Post("/desafios/:id/iniciar", h.Desafio.Iniciar())
	protected.Post("/desafios/:id/abrir-votacao", h.Desafio.AbrirVotacao())
	protected.Post("/desafios/:id/votar", h.Desafio.Votar)
	protected.Post("/desafios/:id/apurar", h.Desafio.Apurar())
	protected.Post("/desafios/:id/convidar", h.Desafio.Convidar)
	protected.Get("/desafios/:id/checkins", h.Desafio.ListarCheckins)
	protected.Post("/desafios/:id/checkins", h.Desafio.Checkin)

	// Carteira
	protected.Get("/carteira", h.Carteira.Saldo)
	protected.Get("/carteira/extrato", h.Carteira.Extrato)
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
	protected.Post("/amigos/adicionar", h.Amizade.AdicionarAmigo())
	protected.Post("/amigos/aceitar", h.Amizade.AceitarAmizade())
	protected.Post("/amigos/recusar", h.Amizade.RecusarAmizade())
	protected.Post("/amigos/remover", h.Amizade.RemoverAmigo())

	// Pagamento PIX (Depósito de Saldo)
	protected.Post("/pagamento/pix", h.PIX.GerarPagamento)
	protected.Get("/pagamento/pix/:asaas_id", h.PIX.ConsultarPagamento)
	protected.Post("/pagamento/pix/:asaas_id/simular", h.PIX.SimularPagamento)

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
