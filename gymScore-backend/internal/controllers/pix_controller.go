package controllers

import (
	"gynScore-backend/internal/config"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
	"gynScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

type PIXController interface {
	GerarPagamento(c *fiber.Ctx) error
	ConsultarPagamento(c *fiber.Ctx) error
	SimularPagamento(c *fiber.Ctx) error
}

type pixController struct {
	pixService services.PIXService
	cfg        *config.Config
}

func NovoPIXController(pixService services.PIXService, cfg *config.Config) PIXController {
	return &pixController{pixService, cfg}
}

// SimularPagamento confirma um pagamento sem pagar de verdade. Disponível APENAS
// no ambiente de sandbox/homologação — em produção retorna 403.
func (ctrl *pixController) SimularPagamento(c *fiber.Ctx) error {
	if !ctrl.cfg.IsAsaasSandbox() {
		return utils.Error(c, fiber.StatusForbidden, "Simulação disponível apenas em ambiente de testes")
	}
	asaasID := c.Params("asaas_id")
	if asaasID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "ID do pagamento obrigatório")
	}
	if err := ctrl.pixService.SimularPagamento(asaasID); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Pagamento simulado com sucesso", nil)
}

func (ctrl *pixController) ConsultarPagamento(c *fiber.Ctx) error {
	asaasID := c.Params("asaas_id")
	if asaasID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "ID do pagamento obrigatório")
	}

	transacao, err := ctrl.pixService.ConsultarPagamento(asaasID)
	if err != nil {
		return utils.Error(c, fiber.StatusNotFound, "Pagamento não encontrado")
	}

	return utils.Success(c, fiber.StatusOK, "Pagamento consultado", transacao)
}

func (ctrl *pixController) GerarPagamento(c *fiber.Ctx) error {
	var req models.PIXRequest

	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "Corpo inválido")
	}

	if req.IDUsuario == 0 {
		if userID := c.Locals("user_id"); userID != nil {
			req.IDUsuario = userID.(uint)
		}
	}

	if req.IDUsuario == 0 {
		return utils.Error(c, fiber.StatusBadRequest, "ID do usuário obrigatório")
	}

	response, err := ctrl.pixService.GerarPagamento(req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "PIX gerado com sucesso", response)
}
