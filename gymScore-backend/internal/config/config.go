package config

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// JWTSecretPadrao é o valor de desenvolvimento; proibido em produção (ver Validar).
const JWTSecretPadrao = "gymscore_secret_key_change_in_production"

// Config armazena todas as configurações da aplicação carregadas do .env
type Config struct {
	AppPort       string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	JWTSecret     string
	AppEnv        string
	AppBaseURL    string
	FrontendPath  string
	UploadsDir    string // pasta das fotos de check-in (servida em /uploads)
	CORSOrigins   []string
	MigrateOnBoot bool
	RateLimitMax  int // requisições/min por IP nas rotas sensíveis (0 desativa)

	AsaasEnv          string // "sandbox" ou "prod"
	AsaasAPIKey       string
	AsaasBaseURL      string
	AsaasWebhookToken string
	// ─── Anthropic / Claude ───────────────────────────────────────────────────
	AnthropicAPIKey string
	AnthropicModel  string

	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

// IAHabilitada indica se há chave da Anthropic configurada.
func (c *Config) IAHabilitada() bool {
	return c.AnthropicAPIKey != ""
}

// IsProduction indica se a aplicação roda em produção.
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production" || c.AppEnv == "prod"
}

// IsAsaasSandbox indica se estamos no ambiente de homologação do Asaas.
func (c *Config) IsAsaasSandbox() bool {
	return c.AsaasEnv != "prod" && c.AsaasEnv != "production"
}

// Validar falha quando a configuração é insegura para o ambiente (ex.: produção sem JWT_SECRET).
func (c *Config) Validar() error {
	if c.IsProduction() && (c.JWTSecret == "" || c.JWTSecret == JWTSecretPadrao) {
		return errors.New("JWT_SECRET deve ser definido com um valor forte quando APP_ENV=production")
	}
	return nil
}

// Load carrega as variáveis de ambiente do arquivo .env (ou de ENV_FILE, se definido)
func Load() *Config {
	envFile := getEnv("ENV_FILE", ".env")
	if err := godotenv.Load(envFile); err != nil {
		log.Println("[CONFIG] Arquivo .env não encontrado, usando variáveis de ambiente do sistema")
	}

	cfg := &Config{
		AppPort:           getEnv("APP_PORT", "3000"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "3306"),
		DBUser:            getEnv("DB_USER", "root"),
		DBPassword:        getEnv("DB_PASSWORD", ""),
		DBName:            getEnv("DB_NAME", "gymscore"),
		JWTSecret:         getEnv("JWT_SECRET", JWTSecretPadrao),
		AppEnv:            getEnv("APP_ENV", "development"),
		AppBaseURL:        strings.TrimRight(getEnv("APP_BASE_URL", "http://localhost:3000"), "/"),
		FrontendPath:      getEnv("FRONTEND_PATH", "../gymScore-frontend/public"),
		UploadsDir:        getEnv("UPLOADS_DIR", "uploads"),
		CORSOrigins:       splitCSV(getEnv("CORS_ORIGINS", "")),
		MigrateOnBoot:     getEnv("MIGRATE_ON_BOOT", "true") != "false",
		AsaasWebhookToken: getEnv("ASAAS_WEBHOOK_TOKEN", ""),

		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  getEnvOrDefault("ANTHROPIC_MODEL", "claude-haiku-4-5-20251001"),

		SMTPHost: getEnv("SMTP_HOST", ""),
		SMTPPort: getEnv("SMTP_PORT", "587"),
		SMTPUser: getEnv("SMTP_USER", ""),
		SMTPPass: getEnv("SMTP_PASS", ""),
		SMTPFrom: getEnv("SMTP_FROM", ""),
	}
	cfg.RateLimitMax = 10
	if v, err := strconv.Atoi(getEnv("RATE_LIMIT_MAX", "10")); err == nil && v >= 0 {
		cfg.RateLimitMax = v
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

// splitCSV separa uma lista separada por vírgulas, descartando itens vazios.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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

// FusoMySQL devolve o offset UTC do fuso informado no formato do MySQL ("+HH:MM"/"-HH:MM").
func FusoMySQL(loc *time.Location, quando time.Time) string {
	_, seg := quando.In(loc).Zone()
	sinal := "+"
	if seg < 0 {
		sinal, seg = "-", -seg
	}
	return fmt.Sprintf("%s%02d:%02d", sinal, seg/3600, (seg%3600)/60)
}

// AplicarFusoDSN acrescenta ao DSN o parâmetro `time_zone` da sessão MySQL com o MESMO fuso que o
// driver usa para ler/gravar datas (`loc`). Sem isso, valores gerados pelo servidor (DEFAULT
// CURRENT_TIMESTAMP, NOW() inclusive nas migrations) usam o fuso do servidor (UTC em containers)
// e aparecem deslocados quando lidos com loc=Local. Usa o offset vigente na conexão
// (se o fuso tiver horário de verão, reinicie o processo na troca).
func AplicarFusoDSN(dsn string, loc *time.Location) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "time_zone=" + url.QueryEscape("'"+FusoMySQL(loc, time.Now())+"'")
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

	dsn = AplicarFusoDSN(dsn, time.Local)

	logLevel := logger.Silent
	if cfg.AppEnv == "development" {
		logLevel = logger.Warn
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

// getEnvOrDefault difere de getEnv por também aplicar o fallback quando a
// variável existe mas está vazia (ex.: `ANTHROPIC_MODEL=` no .env).
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
