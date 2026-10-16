package middlewares

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/config"
	"gymScore-backend/pkg/utils"
)

// VersaoTokenReader consulta a versão de token vigente do usuário (revogação de JWT).
type VersaoTokenReader interface {
	VersaoToken(id uint) (versao int, existe bool, err error)
}

// ExtrairToken lê o JWT do header Authorization (Bearer) ou do cookie "jwt".
func ExtrairToken(c *fiber.Ctx) string {
	var token string

	// 1. Tentar obter o token do cabeçalho Authorization
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			token = parts[1]
		}
	}

	// 2. Se não houver token no header, tentar obter do Cookie "jwt"
	if token == "" {
		token = c.Cookies("jwt")
	}
	return token
}

// AuthMiddleware valida o JWT (header ou cookie) e confere o claim `tv` com o banco: tokens
// emitidos antes de um logout / troca / reset de senha deixam de valer imediatamente.
func AuthMiddleware(cfg *config.Config, versoes VersaoTokenReader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := ExtrairToken(c)
		if token == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "Token de autenticação não fornecido")
		}

		// 3. Validar o token encontrado
		claims, err := utils.ValidarToken(token, cfg.JWTSecret)
		if err != nil {
			return utils.Error(c, fiber.StatusUnauthorized, "Token inválido ou expirado")
		}

		versao, existe, err := versoes.VersaoToken(claims.UserID)
		if err != nil {
			log.Printf("[AUTH] erro ao consultar versão do token do usuário %d: %v", claims.UserID, err)
			return utils.Error(c, fiber.StatusInternalServerError, "Erro interno do servidor")
		}
		if !existe || versao != claims.Versao {
			return utils.Error(c, fiber.StatusUnauthorized, "Sessão encerrada. Faça login novamente")
		}

		// Armazena os dados do usuário autenticado no contexto da requisição
		c.Locals("user_id", claims.UserID)
		c.Locals("user_email", claims.Email)
		c.Locals("token_versao", claims.Versao)

		return c.Next()
	}
}
