package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gymScore-backend/database"
	"gymScore-backend/internal/app"
	"gymScore-backend/internal/client"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/services"
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
	if err := cfg.Validar(); err != nil {
		log.Fatalf("[FATAL] Configuração inválida: %v", err)
	}
	if !cfg.IsProduction() && cfg.JWTSecret == config.JWTSecretPadrao {
		log.Println("[CONFIG] AVISO: usando JWT_SECRET padrão (aceitável apenas em desenvolvimento)")
	}

	// ─── Conexão com o banco de dados ────────────────────────────────────────────
	db, err := config.ConnectDB(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Não foi possível conectar ao banco de dados: %v", err)
	}

	// ─── Migrations (goose embutido) ─────────────────────────────────────────────
	if cfg.MigrateOnBoot {
		sqlDB, err := db.DB()
		if err != nil {
			log.Fatalf("[FATAL] Erro ao obter conexão SQL: %v", err)
		}
		log.Println("[DB] Aplicando migrations pendentes...")
		if err := database.Migrar(context.Background(), sqlDB); err != nil {
			log.Fatalf("[FATAL] Erro ao aplicar migrations: %v", err)
		}
		log.Println("[DB] Migrations em dia")
	} else {
		log.Println("[DB] MIGRATE_ON_BOOT=false: migrations NÃO aplicadas")
	}

	// ─── Servidor ────────────────────────────────────────────────────────────────
	server := app.Construir(cfg, db, app.Dependencias{
		Asaas:  client.NewAsaasClient(cfg),
		Mailer: services.NovoMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom),
	})

	// Rota do Swagger

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("[SERVER] GymScore API iniciando na porta %s (ambiente: %s)", cfg.AppPort, cfg.AppEnv)
	if !cfg.IsProduction() {
		log.Printf("[SERVER] Swagger UI disponível em: http://localhost:%s/swagger/", cfg.AppPort)
	}

	// Graceful shutdown
	go func() {
		if err := server.Listen(addr); err != nil {
			log.Fatalf("[FATAL] Erro ao iniciar servidor: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("[SERVER] Encerrando servidor...")
	if err := server.Shutdown(); err != nil {
		log.Printf("[SERVER] Erro ao encerrar servidor: %v", err)
	}
	log.Println("[SERVER] Servidor encerrado.")
}
