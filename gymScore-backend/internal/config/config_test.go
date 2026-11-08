package config_test

import (
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/config"
)

func TestValidar_ProducaoExigeSegredosFortes(t *testing.T) {
	forte := strings.Repeat("a1", 16)  // 32 caracteres
	forte2 := strings.Repeat("b2", 20) // 40 caracteres
	prod := func(jwt, admin string) config.Config {
		return config.Config{AppEnv: "production", JWTSecret: jwt, AdminJWTSecret: admin}
	}
	casos := []struct {
		nome    string
		cfg     config.Config
		querErr bool
	}{
		{"prod com segredo vazio", prod("", forte2), true},
		{"prod só com espaços", prod("                                        ", forte2), true},
		{"prod com segredo padrão", prod(config.JWTSecretPadrao, forte2), true},
		{"prod abreviado com padrão", config.Config{AppEnv: "prod", JWTSecret: config.JWTSecretPadrao, AdminJWTSecret: forte2}, true},
		{"prod com o valor do .env.example", prod("gymscore_mude_esta_chave_em_producao_para_algo_seguro", forte2), true},
		{"prod com segredo curto (31)", prod(forte[:31], forte2), true},
		{"prod sem ADMIN_JWT_SECRET", prod(forte, ""), true},
		{"prod com ADMIN_JWT_SECRET curto", prod(forte, "curto"), true},
		{"prod com ADMIN_JWT_SECRET igual ao JWT_SECRET", prod(forte, forte), true},
		{"prod com segredos fortes e distintos", prod(forte, forte2), false},
		{"dev com segredo padrão", config.Config{AppEnv: "development", JWTSecret: config.JWTSecretPadrao}, false},
		{"dev com intervalo negativo", config.Config{AppEnv: "development", JobsIntervalo: -time.Second}, true},
	}
	for _, c := range casos {
		err := c.cfg.Validar()
		if (err != nil) != c.querErr {
			t.Errorf("%s: erro=%v, esperava erro=%v", c.nome, err, c.querErr)
		}
	}
}

func TestLoad_JobsEJWTVazio(t *testing.T) {
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JOBS_INTERVALO", "30s")
	t.Setenv("DESAFIO_CARENCIA", "lixo")
	cfg := config.Load()
	if cfg.JWTSecret != config.JWTSecretPadrao {
		t.Errorf("JWT_SECRET vazio deveria cair no padrão de desenvolvimento (recusado em produção): %q", cfg.JWTSecret)
	}
	if cfg.JobsIntervalo != 30*time.Second || cfg.DesafioCarencia != 48*time.Hour {
		t.Errorf("durações inesperadas: %v %v", cfg.JobsIntervalo, cfg.DesafioCarencia)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	for _, k := range []string{"APP_ENV", "MIGRATE_ON_BOOT", "CORS_ORIGINS", "RATE_LIMIT_MAX", "ASAAS_ENV", "ASAAS_WEBHOOK_TOKEN"} {
		t.Setenv(k, "")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("MIGRATE_ON_BOOT", "true")
	t.Setenv("RATE_LIMIT_MAX", "10")
	t.Setenv("ASAAS_ENV", "sandbox")
	cfg := config.Load()
	if !cfg.MigrateOnBoot || cfg.RateLimitMax != 10 || !cfg.IsAsaasSandbox() || cfg.IsProduction() {
		t.Errorf("defaults inesperados: %+v", cfg)
	}
	if len(cfg.CORSOrigins) != 0 {
		t.Errorf("CORS_ORIGINS vazio deveria resultar em lista vazia: %v", cfg.CORSOrigins)
	}
}

func TestLoad_CORSOriginsEMigrateOff(t *testing.T) {
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	t.Setenv("CORS_ORIGINS", " https://a.com , https://b.com,,")
	t.Setenv("MIGRATE_ON_BOOT", "false")
	t.Setenv("ASAAS_ENV", "prod")
	cfg := config.Load()
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "https://a.com" || cfg.CORSOrigins[1] != "https://b.com" {
		t.Errorf("CORS_ORIGINS mal interpretado: %v", cfg.CORSOrigins)
	}
	if cfg.MigrateOnBoot {
		t.Error("MIGRATE_ON_BOOT=false deveria desativar")
	}
	if cfg.IsAsaasSandbox() {
		t.Error("ASAAS_ENV=prod não é sandbox")
	}
}

func TestFusoMySQL(t *testing.T) {
	agora := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	casos := map[string]string{"America/Sao_Paulo": "-03:00", "Asia/Tokyo": "+09:00", "UTC": "+00:00", "Asia/Kolkata": "+05:30"}
	for nome, esperado := range casos {
		loc, err := time.LoadLocation(nome)
		if err != nil {
			t.Skipf("tzdata indisponível: %v", err)
		}
		if got := config.FusoMySQL(loc, agora); got != esperado {
			t.Errorf("%s: obteve %s, esperava %s", nome, got, esperado)
		}
	}
}

func TestAplicarFusoDSN(t *testing.T) {
	got := config.AplicarFusoDSN("u:p@tcp(h:3306)/db?parseTime=True&loc=Local", time.FixedZone("x", -3*3600))
	if got != "u:p@tcp(h:3306)/db?parseTime=True&loc=Local&time_zone=%27-03%3A00%27" {
		t.Errorf("DSN inesperado: %s", got)
	}
	if !strings.Contains(config.AplicarFusoDSN("u@tcp(h)/db", time.UTC), "?time_zone=") {
		t.Error("sem query string deveria começar com ?")
	}
}

func TestAdmin_SegredoETTL(t *testing.T) {
	dev := config.Config{AppEnv: "development"}
	if dev.SegredoAdmin() != config.AdminJWTSecretPadrao || dev.SegredoAdmin() == config.JWTSecretPadrao {
		t.Errorf("dev sem ADMIN_JWT_SECRET deveria usar o padrão de dev (distinto do de usuário): %q", dev.SegredoAdmin())
	}
	if dev.TTLAdmin() != config.AdminTokenTTLPadrao {
		t.Errorf("TTL padrão: %v", dev.TTLAdmin())
	}
	forte := strings.Repeat("a1", 16)
	prod := config.Config{AppEnv: "production", JWTSecret: forte, AdminJWTSecret: config.AdminJWTSecretPadrao}
	if prod.Validar() == nil {
		t.Error("produção não pode usar o segredo admin de desenvolvimento")
	}
	if (&config.Config{AppEnv: "development", AdminTokenTTL: 48 * time.Hour}).Validar() == nil {
		t.Error("ADMIN_TOKEN_TTL acima de 24h deveria ser recusado")
	}
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	t.Setenv("ADMIN_TOKEN_TTL", "30m")
	if c := config.Load(); c.AdminTokenTTL != 30*time.Minute {
		t.Errorf("ADMIN_TOKEN_TTL=30m: %v", c.AdminTokenTTL)
	}
}

// R4: o limite do login admin não depende de RATE_LIMIT_MAX e não pode ser desligado.
func TestLoad_LimiteLoginAdminIndependente(t *testing.T) {
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	t.Setenv("RATE_LIMIT_MAX", "0")
	casos := map[string]int{"": 5, "0": 5, "-3": 5, "lixo": 5, "3": 3, " 20 ": 20}
	for env, esperado := range casos {
		t.Setenv("ADMIN_LOGIN_RATE_LIMIT_MAX", env)
		cfg := config.Load()
		if cfg.RateLimitMax != 0 || cfg.AdminLoginRateLimitMax != esperado {
			t.Errorf("ADMIN_LOGIN_RATE_LIMIT_MAX=%q: geral=%d admin=%d (esperado %d)", env, cfg.RateLimitMax, cfg.AdminLoginRateLimitMax, esperado)
		}
	}
}

func TestCookieSeguro(t *testing.T) {
	casos := []struct {
		cfg    config.Config
		seguro bool
	}{
		{config.Config{AppEnv: "development", AppBaseURL: "http://localhost:3000"}, false},
		{config.Config{AppEnv: "development", AppBaseURL: " https://homolog.gymscore.com.br"}, true},
		{config.Config{AppEnv: "production", AppBaseURL: "http://x"}, true},
		{config.Config{AppEnv: "prod"}, true},
	}
	for _, c := range casos {
		if c.cfg.CookieSeguro() != c.seguro {
			t.Errorf("%+v: CookieSeguro=%v", c.cfg, !c.seguro)
		}
	}
}

func TestLoad_Moderacao(t *testing.T) {
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	for _, k := range []string{"DENUNCIA_PRAZO_ACADEMIA_HORAS", "DENUNCIA_LIMITE_DIA", "DENUNCIA_EVIDENCIA_MAX_BYTES", "EVIDENCIAS_DIR", "RECURSO_PRAZO_DIAS"} {
		t.Setenv(k, "")
	}
	cfg := config.Load()
	if cfg.PrazoAcademiaDenuncia() != 72*time.Hour || cfg.LimiteDenunciasDia() != 5 || cfg.EvidenciaMaxBytes() != 2*1024*1024 ||
		cfg.DirEvidencias() != "evidencias" || cfg.PrazoRecurso() != 15*24*time.Hour {
		t.Errorf("padrões da moderação: %v %d %d %q %v", cfg.PrazoAcademiaDenuncia(), cfg.LimiteDenunciasDia(),
			cfg.EvidenciaMaxBytes(), cfg.DirEvidencias(), cfg.PrazoRecurso())
	}

	t.Setenv("DENUNCIA_PRAZO_ACADEMIA_HORAS", "0") // mínimo 1: vira o padrão
	t.Setenv("DENUNCIA_LIMITE_DIA", "0")
	t.Setenv("DENUNCIA_EVIDENCIA_MAX_BYTES", "1048576")
	t.Setenv("EVIDENCIAS_DIR", "/srv/privado/evidencias")
	t.Setenv("RECURSO_PRAZO_DIAS", "lixo")
	cfg = config.Load()
	if cfg.PrazoAcademiaDenuncia() != 72*time.Hour || cfg.LimiteDenunciasDia() != 0 || cfg.EvidenciaMaxBytes() != 1048576 ||
		cfg.DirEvidencias() != "/srv/privado/evidencias" || cfg.PrazoRecurso() != 15*24*time.Hour {
		t.Errorf("moderação configurada: %+v", cfg)
	}
	// 0 = sem limite só fora de produção
	cfg.AppEnv = "production"
	if cfg.LimiteDenunciasDia() != config.DenunciaLimiteDiaPadrao {
		t.Errorf("em produção o limite não pode ser desligado: %d", cfg.LimiteDenunciasDia())
	}
}

func TestLoad_JanelaContestacao(t *testing.T) {
	t.Setenv("ENV_FILE", "/caminho/que/nao/existe.env")
	casos := []struct {
		env  string
		want time.Duration
	}{{"", 48 * time.Hour}, {"0", 0}, {"12", 12 * time.Hour}, {"-3", 48 * time.Hour}, {"lixo", 48 * time.Hour}}
	for _, c := range casos {
		t.Setenv("PREMIO_JANELA_CONTESTACAO_HORAS", c.env)
		if got := config.Load().JanelaContestacao(); got != c.want {
			t.Errorf("PREMIO_JANELA_CONTESTACAO_HORAS=%q: %v, quer %v", c.env, got, c.want)
		}
	}
	// 0 (libera no próximo ciclo) só fora de produção
	cfg := &config.Config{AppEnv: "production"}
	if cfg.JanelaContestacao() != 48*time.Hour {
		t.Errorf("em produção a janela não pode ser 0: %v", cfg.JanelaContestacao())
	}
}

func TestValidar_EvidenciasNaoPodemSerPublicas(t *testing.T) {
	cfg := &config.Config{AppEnv: "development", UploadsDir: "uploads", EvidenciasDir: "./uploads/"}
	if err := cfg.Validar(); err == nil || !strings.Contains(err.Error(), "EVIDENCIAS_DIR") {
		t.Errorf("EVIDENCIAS_DIR igual a UPLOADS_DIR deveria ser recusado: %v", err)
	}
	cfg.EvidenciasDir = "evidencias"
	if err := cfg.Validar(); err != nil {
		t.Errorf("pastas distintas: %v", err)
	}
	cfg.DenunciaEvidenciaMaxBytes = 50 * 1024 * 1024
	if err := cfg.Validar(); err == nil {
		t.Error("evidência de 50 MiB deveria ser recusada")
	}
}
