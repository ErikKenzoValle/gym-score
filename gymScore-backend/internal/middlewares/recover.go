package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// RecoverMiddleware captura panics e retorna erro 500 de forma controlada.
// O stack trace só é emitido fora de produção.
func RecoverMiddleware(producao bool) fiber.Handler {
	return recover.New(recover.Config{
		EnableStackTrace: !producao,
	})
}
