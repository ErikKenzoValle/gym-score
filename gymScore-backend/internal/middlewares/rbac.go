package middlewares

import (
	"slices"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/models"
	"gymScore-backend/pkg/utils"
)

// PerfilReader é o subconjunto do UsuarioRepository usado pelo RBAC.
type PerfilReader interface {
	// PapeisDoUsuario: tipo de conta + pedido de instrutor + academia (uma consulta; nil = não existe).
	PapeisDoUsuario(id uint) (*models.PapeisUsuario, error)
}

// Papéis aceitos por RequirePerfil.
const (
	PapelAcademia  = models.PerfilAcademia // conta de academia com cadastro aprovado
	PapelAtleta    = models.ModoAtleta     // conta de atleta no modo atleta
	PapelInstrutor = models.ModoInstrutor  // conta de atleta no modo instrutor, com o pedido APROVADO
)

// Códigos/mensagens iguais aos do pacote services (erros.go); repetidos aqui para não criar ciclo.
const (
	codigoModoIncorreto    = "modo_incorreto"
	codigoModoIndisponivel = "modo_indisponivel"
)

// RequirePerfil retorna middleware que exige que o usuário autenticado atue em um dos papéis informados
// (rodada 4: tipo de conta + modo do JWT, sempre reconferidos no banco):
//   - "academia": conta de academia com o cadastro APROVADO (pendente/rejeitado = 403 com status_cadastro);
//   - "atleta": conta de atleta com o token no modo atleta (claim vazio de tokens antigos = atleta);
//   - "instrutor": conta de atleta com o token no modo instrutor E o pedido de instrutor aprovado AGORA
//     (pedido rejeitado/revogado depois da troca de modo = 403 modo_indisponivel, mesmo com o claim).
//
// Conta de atleta no modo errado recebe 403 `modo_incorreto` (com o modo necessário). Punições da conta
// (banimento/suspensão) são do AuthMiddleware/BloquearSuspenso e valem em qualquer modo.
// Deve ser usado APÓS AuthMiddleware. Grava c.Locals("usuario_perfil") com o papel aceito.
func RequirePerfil(usuarioRepo PerfilReader, papeis ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := c.Locals("user_id").(uint)
		if !ok || userID == 0 {
			return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
		}
		p, err := usuarioRepo.PapeisDoUsuario(userID)
		if err != nil {
			return utils.Error(c, fiber.StatusInternalServerError, "Erro interno do servidor")
		}
		if p == nil {
			return utils.Error(c, fiber.StatusUnauthorized, "Usuário não encontrado")
		}
		papel := p.Conta
		if p.Conta == models.PerfilAtleta {
			papel = p.ModoEfetivo(ModoDaSessao(c))
		}
		if !slices.Contains(papeis, papel) {
			// Conta de atleta no modo errado: o front oferece a troca de modo.
			if p.Conta == models.PerfilAtleta && (slices.Contains(papeis, PapelAtleta) || slices.Contains(papeis, PapelInstrutor)) {
				necessario := PapelAtleta
				if !slices.Contains(papeis, PapelAtleta) {
					necessario = PapelInstrutor
				}
				msg := "Troque para o modo atleta para realizar esta ação"
				if necessario == PapelInstrutor {
					msg = "Troque para o modo instrutor para realizar esta ação"
				}
				return utils.ErrorCodigo(c, fiber.StatusForbidden, msg, codigoModoIncorreto, fiber.Map{"modo_necessario": necessario})
			}
			return utils.Error(c, fiber.StatusForbidden, "Acesso não autorizado para este perfil")
		}
		switch papel {
		case PapelAcademia:
			if p.AcademiaStatus != models.CadastroAprovado {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"success": false, "error": "Seu cadastro ainda não foi aprovado pela equipe GymScore",
					"status_cadastro": p.AcademiaStatus,
				})
			}
		case PapelInstrutor:
			// O claim só registra a escolha: o modo instrutor vale enquanto o pedido estiver aprovado.
			if !p.InstrutorAprovado() {
				return utils.ErrorCodigo(c, fiber.StatusForbidden, "O modo instrutor não está disponível para a sua conta",
					codigoModoIndisponivel, fiber.Map{"status_instrutor": p.InstrutorStatus, "status_cadastro": p.InstrutorStatus})
			}
		}
		c.Locals("usuario_perfil", papel)
		return c.Next()
	}
}
