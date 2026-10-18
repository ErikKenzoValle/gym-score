package config_test

import (
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/config"
)

func TestValidar_ProducaoExigeJWTSecretForte(t *testing.T) {
	casos := []struct {
		nome    string
		cfg     config.Config
		querErr bool
	}{
		{"prod com segredo vazio", config.Config{AppEnv: "production", JWTSecret: ""}, true},
		{"prod com segredo padrão", config.Config{AppEnv: "production", JWTSecret: config.JWTSecretPadrao}, true},
		{"prod abreviado com padrão", config.Config{AppEnv: "prod", JWTSecret: config.JWTSecretPadrao}, true},
		{"prod com segredo próprio", config.Config{AppEnv: "production", JWTSecret: "um-segredo-bem-longo-e-aleatorio"}, false},
		{"dev com segredo padrão", config.Config{AppEnv: "development", JWTSecret: config.JWTSecretPadrao}, false},
	}
	for _, c := range casos {
		err := c.cfg.Validar()
		if (err != nil) != c.querErr {
			t.Errorf("%s: erro=%v, esperava erro=%v", c.nome, err, c.querErr)
		}
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
