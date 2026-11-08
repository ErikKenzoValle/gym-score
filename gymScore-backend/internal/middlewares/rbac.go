package middlewares

import (
	"slices"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/models"
	"gymScore-backend/pkg/utils"
)

// PerfilReader é o subconjunto do UsuarioRepository usado pelo RBAC.
type PerfilReader interface {
	BuscarPorID(id uint) (*models.Usuario, error)
	StatusCadastro(id uint, perfil string) (string, error)
}

// RequirePerfil retorna middleware que exige que o usuário autenticado tenha um dos perfis informados.
// Academia e instrutor só passam com o cadastro APROVADO pelo admin (pendente/rejeitado = 403).
// Deve ser usado APÓS AuthMiddleware.
func RequirePerfil(usuarioRepo PerfilReader, perfis ...string) fiber.Handler {
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
		if usuario.Perfil == "academia" || usuario.Perfil == "instrutor" {
			status, err := usuarioRepo.StatusCadastro(userID, usuario.Perfil)
			if err != nil {
				return utils.Error(c, fiber.StatusInternalServerError, "Erro interno do servidor")
			}
			if status != models.CadastroAprovado {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"success": false, "error": "Seu cadastro ainda não foi aprovado pela equipe GymScore",
					"status_cadastro": status,
				})
			}
		}
		c.Locals("usuario_perfil", usuario.Perfil)
		return c.Next()
	}
}
