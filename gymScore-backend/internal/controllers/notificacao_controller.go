package controllers

import (
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// NotificacaoController expõe as notificações in-app do usuário do token.
type NotificacaoController struct {
	service services.NotificacaoService
}

func NovoNotificacaoController(service services.NotificacaoService) *NotificacaoController {
	return &NotificacaoController{service: service}
}

// Listar godoc
// @Summary     Listar notificações (não lidas primeiro)
// @Tags        notificacoes
// @Produce     json
// @Param       pagina query int false "Página (1..)"
// @Param       limite query int false "Itens por página (máx. 100)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/notificacoes [get]
func (ctrl *NotificacaoController) Listar(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	r, err := ctrl.service.Listar(userID, c.QueryInt("pagina", 1), c.QueryInt("limite", 20))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Notificações", r)
}

// NaoLidas godoc
// @Summary     Contador de notificações não lidas
// @Tags        notificacoes
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/notificacoes/nao-lidas [get]
func (ctrl *NotificacaoController) NaoLidas(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	n, err := ctrl.service.NaoLidas(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Não lidas", fiber.Map{"nao_lidas": n})
}

// MarcarLida godoc
// @Summary     Marcar uma notificação como lida
// @Tags        notificacoes
// @Param       id path int true "ID da notificação"
// @Router      /api/notificacoes/{id}/lida [post]
func (ctrl *NotificacaoController) MarcarLida(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	if err := ctrl.service.MarcarLida(userID, id); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Notificação marcada como lida", nil)
}

// MarcarTodasLidas godoc
// @Summary     Marcar todas como lidas
// @Tags        notificacoes
// @Router      /api/notificacoes/lidas [post]
func (ctrl *NotificacaoController) MarcarTodasLidas(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	n, err := ctrl.service.MarcarTodasLidas(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Notificações marcadas como lidas", fiber.Map{"marcadas": n})
}
