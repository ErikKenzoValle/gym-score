// Package app monta o servidor Fiber (injeção de dependências + middlewares + rotas).
// Fica separado do main para que os testes de integração subam a mesma aplicação.
package app

import (
	"log"

	"gymScore-backend/internal/client"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/controllers"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/routes"
	"gymScore-backend/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"

	"gorm.io/gorm"
	_ "gymScore-backend/docs" // registra a especificação do Swagger
)

// Dependencias são as integrações externas substituíveis (testes usam fakes).
type Dependencias struct {
	Asaas  services.AsaasGateway
	Mailer services.Mailer
}

// Construir injeta as dependências e devolve o app Fiber pronto para Listen/Test.
func Construir(cfg *config.Config, db *gorm.DB, dep Dependencias) *fiber.App {
	// Repositories
	usuarioRepo := repositories.NovoUsuarioRepository(db)
	desafioRepo := repositories.NovoDesafioRepository(db)
	amizadeRepo := repositories.NovoAmizadeRepository(db)
	transacaoRepo := repositories.NovoTransacaoRepository(db)
	carteiraRepo := repositories.NovoCarteiraRepository(db)
	treinoRepo := repositories.NovoTreinoRepository(db)
	resetRepo := repositories.NovoResetSenhaRepository(db)
	notifRepo := repositories.NovoNotificacaoRepository(db)
	rankingRepo := repositories.NovoRankingRepository(db)
	instrutorRepo := repositories.NovoInstrutorRepository(db)
	podioRepo := repositories.NovoPodioRepository(db)
	avaliacaoRepo := repositories.NovoAvaliacaoRepository(db)
	academiaRepo := repositories.NovoAcademiaRepository(db)

	// Cliente Anthropic (opcional): sem chave, as recomendações caem na heurística.
	var anthropicClient *client.AnthropicClient
	if cfg.IAHabilitada() {
		anthropicClient = client.NovoAnthropicClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
		log.Println("[Anthropic] cliente inicializado, modelo:", cfg.AnthropicModel)
	} else {
		log.Println("[Anthropic] ANTHROPIC_API_KEY não configurada — recomendações usarão heurística")
	}

	// Services
	notifSvc := services.NovoNotificacaoService(notifRepo)
	usuarioSvc := services.NovoUsuarioService(usuarioRepo, desafioRepo)
	senhaSvc := services.NovoSenhaService(usuarioRepo, resetRepo, dep.Mailer, cfg.AppBaseURL)
	desafioSvc := services.NovoDesafioService(db, desafioRepo, carteiraRepo, amizadeRepo, usuarioRepo, notifSvc,
		services.FotoStorageDisco{Dir: cfg.UploadsDir})
	amizadeSvc := services.NovoAmizadeService(amizadeRepo, usuarioRepo, notifSvc)
	pixSvc := services.NovoPIXService(dep.Asaas, usuarioRepo, transacaoRepo, carteiraRepo, notifSvc)
	carteiraSvc := services.NovoCarteiraService(carteiraRepo, usuarioRepo, dep.Asaas, notifSvc)
	rankingSvc := services.NovoRankingService(rankingRepo)
	treinoSvc := services.NovoTreinoService(treinoRepo, usuarioRepo)
	instrutorSvc := services.NovoInstrutorService(instrutorRepo, usuarioRepo)
	recomendacaoSvc := services.NovoRecomendacaoService(desafioRepo, treinoRepo, usuarioRepo, anthropicClient)
	academiaSvc := services.NovoAcademiaService(db, academiaRepo, usuarioRepo)

	app := fiber.New(fiber.Config{
		AppName:      "GymScore API v1.0.0",
		BodyLimit:    4 * 1024 * 1024, // 3 MB de foto + campos do multipart
		ErrorHandler: ErrorHandler,
	})

	// Middlewares globais
	app.Use(middlewares.RecoverMiddleware(cfg.IsProduction()))
	app.Use(middlewares.LoggerMiddleware())
	if cors := middlewares.CORSMiddleware(cfg); cors != nil {
		app.Use(cors)
	}

	routes.Setup(app, cfg, routes.Controllers{
		Usuario:      controllers.NovoUsuarioController(usuarioSvc, senhaSvc, cfg, desafioRepo),
		Desafio:      controllers.NovoDesafioController(desafioSvc),
		Amizade:      controllers.NovoAmizadeController(amizadeSvc),
		PIX:          controllers.NovoPIXController(pixSvc, cfg),
		Treino:       controllers.NovoTreinoController(treinoSvc),
		Webhook:      controllers.NovoWebhookController(pixSvc, cfg),
		Carteira:     controllers.NovoCarteiraController(carteiraSvc),
		Notificacao:  controllers.NovoNotificacaoController(notifSvc),
		Ranking:      controllers.NovoRankingController(rankingSvc),
		Instrutor:    controllers.NovoInstrutorController(instrutorSvc, desafioRepo),
		Academia:     controllers.NovoAcademiaController(desafioRepo, usuarioRepo, podioRepo, avaliacaoRepo, academiaSvc),
		Recomendacao: controllers.NovoRecomendacaoController(recomendacaoSvc),
		Sessoes:      usuarioRepo,
		UsuarioRepo:  usuarioRepo,
	})
	// Swagger UI: só fora de produção (não expõe o mapa da API)
	if !cfg.IsProduction() {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}
	return app
}

// ErrorHandler é o handler global de erros do Fiber: 4xx do Fiber mantêm a mensagem
// (ex.: 404 de rota); qualquer outro erro vira 500 genérico.
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Erro interno do servidor"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   message,
	})
}
