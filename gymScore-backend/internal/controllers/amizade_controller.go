package controllers

import (
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// AmizadeController gerencia as requisições HTTP relacionadas a amizades
type AmizadeController struct {
	service services.AmizadeService
}

// NovoAmizadeController cria uma nova instância do controller de amizades
func NovoAmizadeController(service services.AmizadeService) *AmizadeController {
	return &AmizadeController{service: service}
}

// ListarAmigos godoc
// @Summary     Listar meus amigos e solicitações
// @Description Amigos aceitos e solicitações pendentes do usuário do token (sem e-mail/CPF)
// @Tags        amigos
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/amigos [get]
func (ctrl *AmizadeController) ListarAmigos(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	amigos, err := ctrl.service.ListarAmigos(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Amigos listados com sucesso", amigos)
}

// acao cria o handler das ações de amizade que recebem apenas {id_amigo}.
func (ctrl *AmizadeController) acao(msg string, fn func(userID, alvo uint) error) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := exigirUsuario(c)
		if !ok {
			return nil
		}
		var req models.AmigoAlvoRequest
		if !lerCorpo(c, &req) {
			return nil
		}
		if err := fn(userID, req.IDAmigo); err != nil {
			return responderErro(c, err)
		}
		return utils.Success(c, fiber.StatusOK, msg, nil)
	}
}

// AdicionarAmigo godoc
// @Summary     Enviar solicitação de amizade
// @Tags        amigos
// @Accept      json
// @Produce     json
// @Param       body body models.AmigoAlvoRequest true "Usuário alvo"
// @Router      /api/amigos/adicionar [post]
func (ctrl *AmizadeController) AdicionarAmigo() fiber.Handler {
	return ctrl.acao("Solicitação de amizade enviada com sucesso", ctrl.service.AdicionarAmigo)
}

// AceitarAmizade godoc
// @Summary     Aceitar solicitação (só o destinatário)
// @Description id_amigo é quem ENVIOU a solicitação
// @Tags        amigos
// @Accept      json
// @Param       body body models.AmigoAlvoRequest true "Remetente"
// @Router      /api/amigos/aceitar [post]
func (ctrl *AmizadeController) AceitarAmizade() fiber.Handler {
	return ctrl.acao("Amizade aceita com sucesso", ctrl.service.AceitarAmizade)
}

// RecusarAmizade godoc
// @Summary     Recusar solicitação (só o destinatário)
// @Tags        amigos
// @Accept      json
// @Param       body body models.AmigoAlvoRequest true "Remetente"
// @Router      /api/amigos/recusar [post]
func (ctrl *AmizadeController) RecusarAmizade() fiber.Handler {
	return ctrl.acao("Solicitação recusada", ctrl.service.RecusarAmizade)
}

// RemoverAmigo godoc
// @Summary     Remover amigo (ou cancelar solicitação pendente)
// @Tags        amigos
// @Accept      json
// @Param       body body models.AmigoAlvoRequest true "Amigo"
// @Router      /api/amigos/remover [post]
func (ctrl *AmizadeController) RemoverAmigo() fiber.Handler {
	return ctrl.acao("Amigo removido com sucesso", ctrl.service.RemoverAmigo)
}
