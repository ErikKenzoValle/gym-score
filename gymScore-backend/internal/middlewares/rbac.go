package middlewares

import (
	"slices"

	"github.com/gofiber/fiber/v2"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/pkg/utils"
)

// RequirePerfil retorna middleware que exige que o usuário autenticado tenha um dos perfis informados.
// Deve ser usado APÓS AuthMiddleware.
func RequirePerfil(usuarioRepo repositories.UsuarioRepository, perfis ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := c.Locals("user_id").(uint)
		if !ok || userID == 0 {
			return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
		}
		usuario, err := usuarioRepo.BuscarPorID(userID)
		if err != nil || usuario == nil {
			return utils.Error(c, fiber.StatusUnauthorized, "Usuário não encontrado")
		}
		if !slices.Contains(perfis, usuario.Perfil) {
			return utils.Error(c, fiber.StatusForbidden, "Acesso não autorizado para este perfil")
		}
		c.Locals("usuario_perfil", usuario.Perfil)
		return c.Next()
	}
}
