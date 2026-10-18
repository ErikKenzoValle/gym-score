package middlewares

import (
	"time"

	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimitMiddleware limita requisições por IP por minuto (login, cadastro, reset de senha).
// max <= 0 desativa. Cada chamada cria um contador independente (um por rota).
func RateLimitMiddleware(max int) fiber.Handler {
	if max <= 0 {
		return func(c *fiber.Ctx) error { return c.Next() }
	}
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return utils.Error(c, fiber.StatusTooManyRequests, "Muitas tentativas. Aguarde um minuto e tente novamente")
		},
	})
}
