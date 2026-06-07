package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config armazena todas as configurações da aplicação carregadas do .env
type Config struct {
	AppPort      string
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	JWTSecret    string
	AppEnv       string
	FrontendPath string
	PIXChave     string
	PIXNome      string
	PIXCidade    string
	AsaasEnv          string // "sandbox" ou "prod"
	AsaasAPIKey       string
	AsaasBaseURL      string
	AsaasWebhookToken string
}

// IsAsaasSandbox indica se estamos no ambiente de homologação do Asaas.
func (c *Config) IsAsaasSandbox() bool {
	return c.AsaasEnv != "prod" && c.AsaasEnv != "production"
}

// Load carrega as variáveis de ambiente do arquivo .env
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("[CONFIG] Arquivo .env não encontrado, usando variáveis de ambiente do sistema")
	}

	cfg := &Config{
		AppPort:      getEnv("APP_PORT", "3000"),
		DBHost:       getEnv("DB_HOST", "localhost"),
		DBPort:       getEnv("DB_PORT", "3306"),
		DBUser:       getEnv("DB_USER", "root"),
		DBPassword:   getEnv("DB_PASSWORD", ""),
		DBName:       getEnv("DB_NAME", "gymscore"),
		JWTSecret:    getEnv("JWT_SECRET", "gymscore_secret_key_change_in_production"),
		AppEnv:       getEnv("APP_ENV", "development"),
		FrontendPath: getEnv("FRONTEND_PATH", "../gymScore-frontend/public"),
		PIXChave:     getEnv("PIX_CHAVE", "sua-chave-pix@email.com"),
		PIXNome:      getEnv("PIX_NOME_RECEBEDOR", "GYMSCORE SISTEMA"),
		PIXCidade:    getEnv("PIX_CIDADE_RECEBEDOR", "SAO PAULO"),
		AsaasWebhookToken: getEnv("ASAAS_WEBHOOK_TOKEN", ""),
	}

	// ─── Asaas: um único toggle (ASAAS_ENV) define URL e chave ───────────────────
	asaasEnv := getEnv("ASAAS_ENV", "sandbox")
	if asaasEnv == "prod" || asaasEnv == "production" {
		cfg.AsaasEnv = "prod"
		cfg.AsaasBaseURL = getEnv("ASAAS_BASE_URL", "https://api.asaas.com")
		cfg.AsaasAPIKey = firstNonEmpty(getEnv("ASAAS_API_KEY_PROD", ""), getEnv("ASAAS_API_KEY", ""))
	} else {
		cfg.AsaasEnv = "sandbox"
		cfg.AsaasBaseURL = getEnv("ASAAS_BASE_URL", "https://api-sandbox.asaas.com")
		cfg.AsaasAPIKey = firstNonEmpty(getEnv("ASAAS_API_KEY_SANDBOX", ""), getEnv("ASAAS_API_KEY", ""))
	}
	log.Printf("[CONFIG] Asaas em modo: %s (%s)", cfg.AsaasEnv, cfg.AsaasBaseURL)
	return cfg
}

// firstNonEmpty retorna o primeiro valor não vazio.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ConnectDB inicializa a conexão com o banco de dados MySQL via GORM
func ConnectDB(cfg *Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)

	logLevel := logger.Silent
	if cfg.AppEnv == "development" {
		logLevel = logger.Info
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar ao banco de dados: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter instância do banco: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log.Println("[CONFIG] Conexão com o banco de dados estabelecida com sucesso")
	return db, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
