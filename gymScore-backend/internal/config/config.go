package config

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
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

// AdminJWTSecretPadrao é o segredo admin de desenvolvimento (ADMIN_JWT_SECRET vazio fora de produção).
// Diferente de JWTSecretPadrao para que token de usuário e de admin nunca se validem um no outro.
const AdminJWTSecretPadrao = "gymscore_admin_dev_secret_nao_usar_em_producao"

// AdminTokenTTLPadrao é a validade padrão do token do painel admin (ADMIN_TOKEN_TTL).
const AdminTokenTTLPadrao = 8 * time.Hour

// AdminLoginRateLimitPadrao: tentativas/min por IP no login do painel admin quando
// ADMIN_LOGIN_RATE_LIMIT_MAX está ausente, é 0 ou negativo (esse limite nunca é desligado).
const AdminLoginRateLimitPadrao = 5

// CEPRateLimitPadrao é o limite padrão (por IP/minuto) da consulta de CEP.
const CEPRateLimitPadrao = 30

// SegredoMinimo é o tamanho mínimo de JWT_SECRET / ADMIN_JWT_SECRET em produção.
const SegredoMinimo = 32

// segredosPublicados são valores que já apareceram no repositório (constante padrão e versões do
// .env.example). Qualquer um deles em produção permite forjar tokens.
var segredosPublicados = map[string]bool{
	JWTSecretPadrao:      true,
	AdminJWTSecretPadrao: true,
	"gymscore_mude_esta_chave_em_producao_para_algo_seguro": true,
	"gymscore_admin_mude_esta_chave_em_producao":            true,
}

// validarSegredo recusa segredo vazio, publicado no repositório ou curto demais.
func validarSegredo(nome, v string) error {
	switch {
	case strings.TrimSpace(v) == "":
		return fmt.Errorf("%s é obrigatório quando APP_ENV=production (gere com: openssl rand -hex 32)", nome)
	case segredosPublicados[v]:
		return fmt.Errorf("%s usa um valor de exemplo publicado no repositório; gere outro com: openssl rand -hex 32", nome)
	case len(v) < SegredoMinimo:
		return fmt.Errorf("%s deve ter pelo menos %d caracteres em produção (gere com: openssl rand -hex 32)", nome, SegredoMinimo)
	}
	return nil
}

// Config armazena todas as configurações da aplicação carregadas do .env
type Config struct {
	AppPort    string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
	// AdminJWTSecret assina os tokens do painel admin (obrigatório e diferente de JWTSecret em produção).
	AdminJWTSecret string
	// AdminTokenTTL é a validade do JWT/cookie do painel admin (ADMIN_TOKEN_TTL, padrão 8h).
	AdminTokenTTL time.Duration
	AppEnv        string
	AppBaseURL    string
	FrontendPath  string
	UploadsDir    string // pasta das fotos de check-in (servida em /uploads)
	CORSOrigins   []string
	MigrateOnBoot bool
	RateLimitMax  int // requisições/min por IP nas rotas sensíveis (0 desativa)
	// AdminLoginRateLimitMax: tentativas/min por IP no login do painel admin (ADMIN_LOGIN_RATE_LIMIT_MAX,
	// padrão 5). Não depende de RATE_LIMIT_MAX e não pode ser desligado: <= 0 vira o padrão.
	AdminLoginRateLimitMax int
	// ViaCEPURL é a base da API de CEP (VIACEP_URL, padrão https://viacep.com.br/ws).
	ViaCEPURL string
	// CEPRateLimitMax: consultas/min por IP em GET /api/cep/:cep (CEP_RATE_LIMIT_MAX, padrão 30). Rota
	// pública que chama serviço externo: nunca desligada (<= 0 vira o padrão).
	CEPRateLimitMax int

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

	// ─── Taxas da plataforma ──────────────────────────────────────────────────
	// PlataformaTaxaModo: "inscricao" (só a taxa sobre as inscrições) ou "inscricao_saque" (também a tarifa de saque).
	PlataformaTaxaModo            string
	PlataformaTaxaInscricaoPct    int   // % das inscrições retido (sai da parte da academia); gravado no desafio na criação
	PlataformaTarifaSaqueCentavos int64 // tarifa debitada junto do saque; só vale no modo inscricao_saque

	// ─── Jobs periódicos ──────────────────────────────────────────────────────
	// JobsIntervalo é o intervalo do job de expiração de desafios e da reconciliação PIX (0 desativa).
	JobsIntervalo time.Duration
	// DesafioCarencia é quanto tempo depois de data_evento um desafio aberto/em andamento sem
	// encerramento é cancelado automaticamente (entradas e reserva devolvidas).
	DesafioCarencia time.Duration

	// ─── Moderação (denúncias, punições, recursos) ───────────────────────────
	// DenunciaPrazoAcademiaHoras: prazo da academia para julgar a denúncia da jurisdição dela antes de
	// subir ao admin (DENUNCIA_PRAZO_ACADEMIA_HORAS, padrão 72, mínimo 1).
	DenunciaPrazoAcademiaHoras int
	// DenunciaLimiteDia: denúncias por denunciante em 24h (DENUNCIA_LIMITE_DIA, padrão 5; 0 = sem limite,
	// recusado em produção).
	DenunciaLimiteDia int
	// DenunciaEvidenciaMaxBytes: tamanho máximo de cada imagem de evidência (padrão 2 MiB).
	DenunciaEvidenciaMaxBytes int64
	// EvidenciasDir: pasta PRIVADA das evidências (nunca servida estaticamente; padrão "evidencias").
	EvidenciasDir string
	// RecursoPrazoDias: dias, contados do início da punição, para o punido recorrer (padrão 15).
	RecursoPrazoDias int
	// PremioJanelaContestacaoHoras: horas entre o lançamento do pódio e a liberação dos créditos
	// (PREMIO_JANELA_CONTESTACAO_HORAS, padrão 48; 0 = libera no próximo ciclo do job, só fora de produção).
	PremioJanelaContestacaoHoras int
}

// Padrões da moderação (analise/moderacao-spec.md §5).
const (
	DenunciaPrazoAcademiaHorasPadrao = 72
	DenunciaLimiteDiaPadrao          = 5
	DenunciaEvidenciaMaxBytesPadrao  = 2 * 1024 * 1024
	EvidenciasDirPadrao              = "evidencias"
	RecursoPrazoDiasPadrao           = 15
	PremioJanelaContestacaoPadrao    = 48
)

// JanelaContestacao devolve a janela de contestação do pódio. Negativo vira o padrão; 0 só vale fora de
// produção (em produção vira o padrão, para nunca liberar prêmio sem janela por engano).
func (c *Config) JanelaContestacao() time.Duration {
	h := c.PremioJanelaContestacaoHoras
	if h < 0 || (h == 0 && c.IsProduction()) {
		h = PremioJanelaContestacaoPadrao
	}
	return time.Duration(h) * time.Hour
}

// PrazoAcademiaDenuncia devolve o prazo da academia (valor <= 0 vira o padrão).
func (c *Config) PrazoAcademiaDenuncia() time.Duration {
	h := c.DenunciaPrazoAcademiaHoras
	if h <= 0 {
		h = DenunciaPrazoAcademiaHorasPadrao
	}
	return time.Duration(h) * time.Hour
}

// LimiteDenunciasDia devolve o limite de denúncias por denunciante em 24h (0 = sem limite, só fora de
// produção; em produção 0 vira o padrão).
func (c *Config) LimiteDenunciasDia() int {
	if c.DenunciaLimiteDia <= 0 && c.IsProduction() {
		return DenunciaLimiteDiaPadrao
	}
	if c.DenunciaLimiteDia < 0 {
		return 0
	}
	return c.DenunciaLimiteDia
}

// EvidenciaMaxBytes devolve o limite por imagem (valor <= 0 vira o padrão).
func (c *Config) EvidenciaMaxBytes() int64 {
	if c.DenunciaEvidenciaMaxBytes <= 0 {
		return DenunciaEvidenciaMaxBytesPadrao
	}
	return c.DenunciaEvidenciaMaxBytes
}

// DirEvidencias devolve a pasta privada das evidências (vazio vira o padrão).
func (c *Config) DirEvidencias() string {
	if strings.TrimSpace(c.EvidenciasDir) == "" {
		return EvidenciasDirPadrao
	}
	return c.EvidenciasDir
}

// PrazoRecurso devolve o prazo para recorrer de uma punição (valor <= 0 vira o padrão).
func (c *Config) PrazoRecurso() time.Duration {
	d := c.RecursoPrazoDias
	if d <= 0 {
		d = RecursoPrazoDiasPadrao
	}
	return time.Duration(d) * 24 * time.Hour
}

// Modos de taxa da plataforma.
const (
	TaxaModoInscricao      = "inscricao"
	TaxaModoInscricaoSaque = "inscricao_saque"
)

// TarifaSaqueAtiva devolve a tarifa de saque em vigor (0 fora do modo inscricao_saque).
func (c *Config) TarifaSaqueAtiva() int64 {
	if c.PlataformaTaxaModo == TaxaModoInscricaoSaque && c.PlataformaTarifaSaqueCentavos > 0 {
		return c.PlataformaTarifaSaqueCentavos
	}
	return 0
}

// SegredoAdmin devolve o segredo dos tokens do painel admin. Fora de produção, vazio cai no padrão de
// desenvolvimento (em produção Validar já recusou vazio/igual ao JWT_SECRET).
func (c *Config) SegredoAdmin() string {
	if c.AdminJWTSecret == "" && !c.IsProduction() {
		return AdminJWTSecretPadrao
	}
	return c.AdminJWTSecret
}

// TTLAdmin devolve a validade do token admin (padrão 8h quando não configurado).
func (c *Config) TTLAdmin() time.Duration {
	if c.AdminTokenTTL <= 0 {
		return AdminTokenTTLPadrao
	}
	return c.AdminTokenTTL
}

// IAHabilitada indica se há chave da Anthropic configurada.
func (c *Config) IAHabilitada() bool {
	return c.AnthropicAPIKey != ""
}

// IsProduction indica se a aplicação roda em produção.
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production" || c.AppEnv == "prod"
}

// CookieSeguro diz se os cookies de sessão saem com `Secure` (e com os prefixos __Host-/__Secure-):
// em produção OU quando APP_BASE_URL é https (homologação/staging atrás de TLS também).
func (c *Config) CookieSeguro() bool {
	return c.IsProduction() || strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.AppBaseURL)), "https://")
}

// IsAsaasSandbox indica se estamos no ambiente de homologação do Asaas.
func (c *Config) IsAsaasSandbox() bool {
	return c.AsaasEnv != "prod" && c.AsaasEnv != "production"
}

// Validar falha quando a configuração é insegura para o ambiente (ex.: produção sem JWT_SECRET).
func (c *Config) Validar() error {
	if c.IsProduction() {
		if err := validarSegredo("JWT_SECRET", c.JWTSecret); err != nil {
			return err
		}
		if err := validarSegredo("ADMIN_JWT_SECRET", c.AdminJWTSecret); err != nil {
			return err
		}
		if c.AdminJWTSecret == c.JWTSecret {
			return errors.New("ADMIN_JWT_SECRET deve ser diferente de JWT_SECRET")
		}
	}
	if c.AdminTokenTTL < 0 || c.AdminTokenTTL > 24*time.Hour {
		return errors.New("ADMIN_TOKEN_TTL deve estar entre 1m e 24h")
	}
	if c.JobsIntervalo < 0 || c.DesafioCarencia < 0 {
		return errors.New("JOBS_INTERVALO e DESAFIO_CARENCIA não podem ser negativos")
	}
	switch c.PlataformaTaxaModo {
	case "", TaxaModoInscricao, TaxaModoInscricaoSaque:
	default:
		return fmt.Errorf("PLATAFORMA_TAXA_MODO inválido %q (use %s ou %s)", c.PlataformaTaxaModo, TaxaModoInscricao, TaxaModoInscricaoSaque)
	}
	if c.PlataformaTaxaInscricaoPct < 0 || c.PlataformaTaxaInscricaoPct > 100 {
		return errors.New("PLATAFORMA_TAXA_INSCRICAO_PCT deve estar entre 0 e 100")
	}
	if c.PlataformaTarifaSaqueCentavos < 0 {
		return errors.New("PLATAFORMA_TARIFA_SAQUE_CENTAVOS não pode ser negativa")
	}
	if c.DenunciaLimiteDia < 0 {
		return errors.New("DENUNCIA_LIMITE_DIA não pode ser negativo")
	}
	if c.DenunciaEvidenciaMaxBytes < 0 || c.DenunciaEvidenciaMaxBytes > 10*1024*1024 {
		return errors.New("DENUNCIA_EVIDENCIA_MAX_BYTES deve estar entre 1 e 10485760")
	}
	if c.EvidenciasDir != "" && c.UploadsDir != "" && filepathLimpo(c.EvidenciasDir) == filepathLimpo(c.UploadsDir) {
		return errors.New("EVIDENCIAS_DIR não pode ser a pasta pública UPLOADS_DIR")
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
		JWTSecret:         getEnvOrDefault("JWT_SECRET", JWTSecretPadrao),
		AdminJWTSecret:    os.Getenv("ADMIN_JWT_SECRET"), // vazio fora de produção: SegredoAdmin() usa o padrão de dev
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
	cfg.PlataformaTaxaModo = strings.ToLower(strings.TrimSpace(getEnvOrDefault("PLATAFORMA_TAXA_MODO", TaxaModoInscricao)))
	cfg.PlataformaTaxaInscricaoPct = 10
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("PLATAFORMA_TAXA_INSCRICAO_PCT"))); err == nil {
		cfg.PlataformaTaxaInscricaoPct = v // fora de 0..100 é recusado por Validar
	}
	cfg.PlataformaTarifaSaqueCentavos = 200
	if v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("PLATAFORMA_TARIFA_SAQUE_CENTAVOS")), 10, 64); err == nil {
		cfg.PlataformaTarifaSaqueCentavos = v
	}
	cfg.AdminTokenTTL = duracaoEnv("ADMIN_TOKEN_TTL", AdminTokenTTLPadrao)
	cfg.JobsIntervalo = duracaoEnv("JOBS_INTERVALO", 5*time.Minute)
	cfg.DesafioCarencia = duracaoEnv("DESAFIO_CARENCIA", 48*time.Hour)
	cfg.RateLimitMax = 10
	if v, err := strconv.Atoi(getEnv("RATE_LIMIT_MAX", "10")); err == nil && v >= 0 {
		cfg.RateLimitMax = v
	}
	cfg.DenunciaPrazoAcademiaHoras = inteiroEnv("DENUNCIA_PRAZO_ACADEMIA_HORAS", DenunciaPrazoAcademiaHorasPadrao)
	if cfg.DenunciaPrazoAcademiaHoras < 1 {
		cfg.DenunciaPrazoAcademiaHoras = DenunciaPrazoAcademiaHorasPadrao
	}
	cfg.DenunciaLimiteDia = inteiroEnv("DENUNCIA_LIMITE_DIA", DenunciaLimiteDiaPadrao)
	cfg.DenunciaEvidenciaMaxBytes = int64(inteiroEnv("DENUNCIA_EVIDENCIA_MAX_BYTES", DenunciaEvidenciaMaxBytesPadrao))
	cfg.EvidenciasDir = getEnv("EVIDENCIAS_DIR", EvidenciasDirPadrao)
	cfg.RecursoPrazoDias = inteiroEnv("RECURSO_PRAZO_DIAS", RecursoPrazoDiasPadrao)
	if cfg.RecursoPrazoDias < 1 {
		cfg.RecursoPrazoDias = RecursoPrazoDiasPadrao
	}
	cfg.PremioJanelaContestacaoHoras = inteiroEnv("PREMIO_JANELA_CONTESTACAO_HORAS", PremioJanelaContestacaoPadrao)
	if cfg.PremioJanelaContestacaoHoras < 0 {
		cfg.PremioJanelaContestacaoHoras = PremioJanelaContestacaoPadrao
	}
	cfg.AdminLoginRateLimitMax = AdminLoginRateLimitPadrao
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("ADMIN_LOGIN_RATE_LIMIT_MAX"))); err == nil && v > 0 {
		cfg.AdminLoginRateLimitMax = v
	}

	cfg.ViaCEPURL = strings.TrimSpace(os.Getenv("VIACEP_URL"))
	cfg.CEPRateLimitMax = CEPRateLimitPadrao
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CEP_RATE_LIMIT_MAX"))); err == nil && v > 0 {
		cfg.CEPRateLimitMax = v
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

// duracaoEnv lê uma duração Go ("5m", "48h"); vazia ou inválida usa o padrão (com aviso no log).
func duracaoEnv(chave string, padrao time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(chave))
	if v == "" {
		return padrao
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("[CONFIG] %s inválido (%q), usando %s", chave, v, padrao)
		return padrao
	}
	return d
}

// inteiroEnv lê um inteiro; vazio ou ilegível usa o padrão (com aviso no log quando ilegível).
func inteiroEnv(chave string, padrao int) int {
	v := strings.TrimSpace(os.Getenv(chave))
	if v == "" {
		return padrao
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("[CONFIG] %s inválido (%q), usando %d", chave, v, padrao)
		return padrao
	}
	return n
}

// filepathLimpo normaliza um caminho para comparação (absoluto quando possível).
func filepathLimpo(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
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
