package routes

import (
	"gynScore-backend/internal/config"
	"gynScore-backend/internal/controllers"
	"gynScore-backend/internal/middlewares"

	"github.com/gofiber/fiber/v2"
)

// Setup registra todas as rotas da aplicação no servidor Fiber
func Setup(
	app *fiber.App,
	cfg *config.Config,
	usuarioCtrl *controllers.UsuarioController,
	desafioCtrl *controllers.DesafioController,
	amizadeCtrl *controllers.AmizadeController,
	pixCtrl controllers.PIXController,
	treinoCtrl *controllers.TreinoController,
	webhookCtrl *controllers.WebhookController,
) {
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
	app.Get("/privacidade", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "privacidade.html") })
	app.Get("/treinos", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "treinos.html") })
	app.Get("/depositar", func(c *fiber.Ctx) error { return c.SendFile(htmlDir + "depositar.html") })

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

	// Autenticação
	api.Post("/login", usuarioCtrl.Login)

	// Cadastro de usuário
	api.Post("/usuarios", usuarioCtrl.CriarUsuario)

	// Recuperação de senha (público — valida e-mail + CPF)
	api.Post("/usuarios/recuperar-senha", usuarioCtrl.RecuperarSenha)

	// Webhook Asaas — DEVE ficar antes do grupo protected para não herdar o JWT middleware
	api.Post("/webhooks/asaas", webhookCtrl.ReceberWebhookAsaas)

	// ─── Rotas protegidas (requerem JWT) ─────────────────────────────────────────
	protected := api.Group("", middlewares.AuthMiddleware(cfg))

	// Usuários — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/usuarios", usuarioCtrl.ListarUsuarios)
	protected.Patch("/usuarios/perfil", usuarioCtrl.AtualizarPerfil)
	protected.Patch("/usuarios/senha", usuarioCtrl.AlterarSenha)
	protected.Get("/usuarios/:id", usuarioCtrl.BuscarUsuario)

	// Desafios — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/desafios/view", desafioCtrl.ListarDesafios)
	protected.Post("/desafios/aceitar_desafio", desafioCtrl.AceitarDesafio)
	protected.Post("/desafios/iniciar", desafioCtrl.IniciarDesafio)
	protected.Post("/desafios/encerrar", desafioCtrl.EncerrarDesafio)
	protected.Post("/desafios/cancelar", desafioCtrl.CancelarDesafio)
	protected.Post("/desafios", desafioCtrl.CriarDesafio)
	protected.Get("/desafios/:id", desafioCtrl.ListarDesafiosPorUsuario)

	// Amigos — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Post("/amigos/adicionar", amizadeCtrl.AdicionarAmigo)
	protected.Post("/amigos/aceitar", amizadeCtrl.AceitarAmizade)
	protected.Post("/amigos/remover", amizadeCtrl.RemoverAmigo)
	protected.Get("/amigos/:id", amizadeCtrl.ListarAmigos)

	// Pagamento PIX (Depósito de Saldo)
	protected.Post("/pagamento/pix", pixCtrl.GerarPagamento)
	protected.Get("/pagamento/pix/:asaas_id", pixCtrl.ConsultarPagamento)
	protected.Post("/pagamento/pix/:asaas_id/simular", pixCtrl.SimularPagamento)

	// Treinos — rotas específicas ANTES das rotas com parâmetro dinâmico
	protected.Get("/treinos/radar", treinoCtrl.Radar)
	protected.Post("/treinos/importar", treinoCtrl.ImportarTreino)
	protected.Post("/treinos/concluir", treinoCtrl.ConcluirExercicio)
	protected.Get("/treinos", treinoCtrl.ListarTreinos)
	protected.Post("/treinos", treinoCtrl.CriarTreino)
	protected.Get("/treinos/:id", treinoCtrl.BuscarTreino)
	protected.Delete("/treinos/:id", treinoCtrl.DeletarTreino)

}
