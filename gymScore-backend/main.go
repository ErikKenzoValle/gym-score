package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gynScore-backend/internal/client"
	"gynScore-backend/internal/config"
	"gynScore-backend/internal/controllers"
	"gynScore-backend/internal/middlewares"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/internal/routes"
	"gynScore-backend/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"

	_ "gynScore-backend/docs"
)

// @title GymScore API
// @version 1.0
// @description API para o sistema GymScore de desafios e treinos.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:3000
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Digite "Bearer " seguido do seu token JWT. Exemplo: "Bearer eyJhbGciOiJIUzI1..."
func main() {
	// ─── Carregamento de configurações ───────────────────────────────────────────
	cfg := config.Load()

	// ─── Conexão com o banco de dados ────────────────────────────────────────────
	db, err := config.ConnectDB(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Não foi possível conectar ao banco de dados: %v", err)
	}

	// ─── Auto-migração ativada conforme solicitação do usuário ───────────────────
	log.Println("[DB] Iniciando auto-migração das tabelas...")
	err = db.AutoMigrate(
		&models.Usuario{},
		&models.Desafio{},
		&models.DesafioParticipante{},
		&models.Amizade{},
		&models.Transacao{},
		&models.Treino{},
		&models.TreinoExercicio{},
		&models.TreinoConclusao{},
	)
	if err != nil {
		log.Fatalf("[FATAL] Erro ao realizar auto-migração: %v", err)
	}
	log.Println("[DB] Auto-migração concluída com sucesso")

	// ─── Injeção de dependências ──────────────────────────────────────────────────

	// Repositories
	usuarioRepo := repositories.NovoUsuarioRepository(db)
	desafioRepo := repositories.NovoDesafioRepository(db)
	amizadeRepo := repositories.NovoAmizadeRepository(db)
	transacaoRepo := repositories.NovoTransacaoRepository(db)
	treinoRepo := repositories.NovoTreinoRepository(db)

	// Clients
	asaasClient := client.NewAsaasClient(cfg)

	// Services
	usuarioSvc := services.NovoUsuarioService(usuarioRepo)
	desafioSvc := services.NovoDesafioService(desafioRepo, usuarioRepo)
	amizadeSvc := services.NovoAmizadeService(amizadeRepo, usuarioRepo)
	pixSvc := services.NovoPIXService(asaasClient, usuarioRepo, transacaoRepo)
	treinoSvc := services.NovoTreinoService(treinoRepo, usuarioRepo)

	// Controllers
	usuarioCtrl := controllers.NovoUsuarioController(usuarioSvc, cfg)
	desafioCtrl := controllers.NovoDesafioController(desafioSvc)
	amizadeCtrl := controllers.NovoAmizadeController(amizadeSvc)
	pixCtrl := controllers.NovoPIXController(pixSvc, cfg)
	treinoCtrl := controllers.NovoTreinoController(treinoSvc)
	webhookCtrl := controllers.NovoWebhookController(db, transacaoRepo, usuarioRepo, cfg)

	// ─── Configuração do servidor Fiber ──────────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      "GymScore API v1.0.0",
		ErrorHandler: errorHandler,
	})

	// Middlewares globais
	app.Use(middlewares.RecoverMiddleware())
	app.Use(middlewares.LoggerMiddleware())
	app.Use(middlewares.CORSMiddleware())

	// Rota do Swagger
	app.Get("/swagger/*", swagger.HandlerDefault)

	// Registro das rotas
	routes.Setup(app, cfg, usuarioCtrl, desafioCtrl, amizadeCtrl, pixCtrl, treinoCtrl, webhookCtrl)

	// ─── Inicialização do servidor ────────────────────────────────────────────────
	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("[SERVER] GymScore API iniciando na porta %s (ambiente: %s)", cfg.AppPort, cfg.AppEnv)
	log.Printf("[SERVER] Swagger UI disponível em: http://localhost:%s/swagger/", cfg.AppPort)

	// Graceful shutdown
	go func() {
		if err := app.Listen(addr); err != nil {
			log.Fatalf("[FATAL] Erro ao iniciar servidor: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("[SERVER] Encerrando servidor...")
	if err := app.Shutdown(); err != nil {
		log.Printf("[SERVER] Erro ao encerrar servidor: %v", err)
	}
	log.Println("[SERVER] Servidor encerrado.")
}

// errorHandler é o handler global de erros do Fiber
func errorHandler(c *fiber.Ctx, err error) error {
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
