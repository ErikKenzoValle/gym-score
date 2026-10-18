package middlewares

import (
	"strings"

	"gymScore-backend/internal/config"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// CORSMiddleware configura o CORS conforme CORS_ORIGINS (lista separada por vírgulas).
//
//   - Com lista: libera apenas essas origens;
//   - Sem lista em desenvolvimento: libera "*" (sem credenciais);
//   - Sem lista em produção: retorna nil (CORS desligado; o front é servido na mesma origem).
func CORSMiddleware(cfg *config.Config) fiber.Handler {
	origens := "*"
	if len(cfg.CORSOrigins) > 0 {
		origens = strings.Join(cfg.CORSOrigins, ",")
	} else if cfg.IsProduction() {
		return nil
	}
	return cors.New(cors.Config{
		AllowOrigins:     origens,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Requested-With",
		ExposeHeaders:    "Content-Length",
		AllowCredentials: origens != "*",
		MaxAge:           86400, // 24 horas de cache para preflight
	})
}
