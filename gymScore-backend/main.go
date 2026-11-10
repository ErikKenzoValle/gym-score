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
	"gymScore-backend/internal/cli"
	"gymScore-backend/internal/client"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/jobs"
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

// @securityDefinitions.apikey AdminAuth
// @in header
// @name Authorization
// @description Token do painel admin ("Bearer <token>"; no navegador vale o cookie admin_jwt). Assinado com ADMIN_JWT_SECRET, aud=gymscore-admin.
func main() {
	// ─── CLI: `admin criar-superadmin` / `admin resetar-senha` (não sobe o servidor) ─────────
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		os.Exit(executarCLIAdmin(os.Args[2:]))
	}
	// ─── CLI: `seed-teste` (contas fictícias de teste; recusado em produção) ─────────────────
	if len(os.Args) > 1 && os.Args[1] == "seed-teste" {
		os.Exit(executarSeedTeste(os.Args[2:]))
	}

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

	// ─── Seed das contas de teste (B5-6/QA56-2): só com APP_ENV explícito development|dev|local|test; SEED_TESTE_AUTO=false desliga; falha não derruba ──
	if res, err := cli.SeedTesteAuto(context.Background(), cfg.AppEnvExplicito, cfg.SeedTesteAuto, db, cfg.PlataformaTaxaInscricaoPct, os.Stdout); err != nil {
		log.Printf("[SEED] Seed automático das contas de teste falhou (o servidor sobe mesmo assim): %v", err)
	} else if !res.Executado {
		log.Printf("[SEED] Seed automático das contas de teste não executado (%s)", res.Motivo)
	}

	// ─── Servidor ────────────────────────────────────────────────────────────────
	server, tarefas := app.ConstruirComJobs(cfg, db, app.Dependencias{
		Asaas:  client.NewAsaasClient(cfg),
		Mailer: services.NovoMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom),
	})

	// Jobs periódicos (expiração de desafios + reconciliação PIX); JOBS_INTERVALO=0 desativa
	agendador := jobs.Iniciar(cfg.JobsIntervalo, tarefas...)

	// ─── Inicialização do servidor ────────────────────────────────────────────────
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
	agendador.Parar()
	if err := server.Shutdown(); err != nil {
		log.Printf("[SERVER] Erro ao encerrar servidor: %v", err)
	}
	log.Println("[SERVER] Servidor encerrado.")
}

// executarCLIAdmin conecta no banco, aplica as migrations pendentes e roda o comando admin.
// A senha é lida do terminal sem eco (ou do stdin, sem TTY); nunca de argumentos.
func executarCLIAdmin(args []string) int {
	cfg := config.Load()
	db, err := config.ConnectDB(cfg)
	if err != nil {
		log.Printf("[CLI] Não foi possível conectar ao banco de dados: %v", err)
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("[CLI] Erro ao obter conexão SQL: %v", err)
		return 1
	}
	defer sqlDB.Close()
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		log.Printf("[CLI] Erro ao aplicar migrations: %v", err)
		return 1
	}
	if err := cli.Executar(args, db, cli.LeitorPadrao(os.Stdin, os.Stderr), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Erro:", err)
		return 1
	}
	return 0
}

// executarSeedTeste cria/atualiza as contas fictícias de teste (`go run . seed-teste`). Recusa
// APP_ENV=production ANTES de conectar no banco; aplica as migrations pendentes e sai.
func executarSeedTeste(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "Uso: seed-teste (sem argumentos)")
		return 2
	}
	cfg := config.Load()
	if cli.EhProducao(cfg.AppEnv) {
		fmt.Fprintln(os.Stderr, "Erro:", cli.ErrSeedEmProducao)
		return 1
	}
	db, err := config.ConnectDB(cfg)
	if err != nil {
		log.Printf("[CLI] Não foi possível conectar ao banco de dados: %v", err)
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("[CLI] Erro ao obter conexão SQL: %v", err)
		return 1
	}
	defer sqlDB.Close()
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		log.Printf("[CLI] Erro ao aplicar migrations: %v", err)
		return 1
	}
	if _, err := cli.SeedTeste(cfg.AppEnv, db, cfg.PlataformaTaxaInscricaoPct, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Erro:", err)
		return 1
	}
	return 0
}
