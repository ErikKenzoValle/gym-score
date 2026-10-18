package controllers

import (
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// CarteiraController expõe saldo, extrato e saque PIX.
type CarteiraController struct {
	service services.CarteiraService
}

func NovoCarteiraController(service services.CarteiraService) *CarteiraController {
	return &CarteiraController{service: service}
}

// Saldo godoc
// @Summary     Saldo da carteira
// @Tags        carteira
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/carteira [get]
func (ctrl *CarteiraController) Saldo(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	r, err := ctrl.service.Saldo(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Saldo", r)
}

// Extrato godoc
// @Summary     Extrato (ledger) paginado
// @Tags        carteira
// @Produce     json
// @Param       pagina query int false "Página (1..)"
// @Param       limite query int false "Itens por página (máx. 100)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/carteira/extrato [get]
func (ctrl *CarteiraController) Extrato(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	r, err := ctrl.service.Extrato(userID, c.QueryInt("pagina", 1), c.QueryInt("limite", 20))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Extrato", r)
}

// Sacar godoc
// @Summary     Saque via PIX
// @Description Debita o valor antes de chamar o Asaas; reembolsa automaticamente se a transferência falhar.
// @Tags        carteira
// @Accept      json
// @Produce     json
// @Param       body body models.SaqueRequest true "Dados do saque"
// @Success     201 {object} utils.APIResponse
// @Router      /api/carteira/saque [post]
func (ctrl *CarteiraController) Sacar(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.SaqueRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	r, err := ctrl.service.Sacar(userID, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Saque solicitado", r)
}

// ConsultarSaque godoc
// @Summary     Acompanhar saque (polling)
// @Tags        carteira
// @Produce     json
// @Param       id path int true "ID do saque"
// @Success     200 {object} utils.APIResponse
// @Router      /api/carteira/saque/{id} [get]
func (ctrl *CarteiraController) ConsultarSaque(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	r, err := ctrl.service.ConsultarSaque(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Saque consultado", r)
}
