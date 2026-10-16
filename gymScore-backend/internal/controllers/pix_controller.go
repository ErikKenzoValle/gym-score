package controllers

import (
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

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

// SimularPagamento godoc
// @Summary     Simular pagamento PIX (somente sandbox)
// @Tags        pagamento
// @Produce     json
// @Param       asaas_id path string true "ID da cobrança no Asaas"
// @Success     200 {object} utils.APIResponse
// @Router      /api/pagamento/pix/{asaas_id}/simular [post]
//
// SimularPagamento confirma um pagamento sem pagar de verdade. Disponível APENAS
// no ambiente de sandbox/homologação — em produção retorna 403. Só o dono do depósito.
func (ctrl *pixController) SimularPagamento(c *fiber.Ctx) error {
	if !ctrl.cfg.IsAsaasSandbox() {
		return utils.Error(c, fiber.StatusForbidden, "Simulação disponível apenas em ambiente de testes")
	}
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	asaasID := c.Params("asaas_id")
	if asaasID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "ID do pagamento obrigatório")
	}
	if err := ctrl.pixService.SimularPagamento(userID, asaasID); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Pagamento simulado com sucesso", nil)
}

// ConsultarPagamento godoc
// @Summary     Consultar/confirmar depósito PIX (polling)
// @Tags        pagamento
// @Produce     json
// @Param       asaas_id path string true "ID da cobrança no Asaas"
// @Success     200 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Router      /api/pagamento/pix/{asaas_id} [get]
//
// ConsultarPagamento confirma por polling; só o dono do depósito enxerga (404 para os demais).
func (ctrl *pixController) ConsultarPagamento(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	asaasID := c.Params("asaas_id")
	if asaasID == "" {
		return utils.Error(c, fiber.StatusBadRequest, "ID do pagamento obrigatório")
	}
	transacao, err := ctrl.pixService.ConsultarPagamento(userID, asaasID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Pagamento consultado", transacao)
}

// GerarPagamento godoc
// @Summary     Gerar cobrança PIX de depósito
// @Tags        pagamento
// @Accept      json
// @Produce     json
// @Param       body body models.PIXRequest true "Valor em centavos e CPF"
// @Success     200 {object} utils.APIResponse
// @Router      /api/pagamento/pix [post]
//
// GerarPagamento cria a cobrança PIX para o usuário do token (valor em centavos).
func (ctrl *pixController) GerarPagamento(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.PIXRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	response, err := ctrl.pixService.GerarPagamento(userID, req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "PIX gerado com sucesso", response)
}
