package controllers

import (
	"github.com/gofiber/fiber/v2"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
	"gynScore-backend/pkg/utils"
)

// SaqueController gerencia as requisições HTTP relacionadas a saques
type SaqueController struct {
	service services.SaqueService
}

// NovoSaqueController cria uma nova instância do controller de saques
func NovoSaqueController(service services.SaqueService) *SaqueController {
	return &SaqueController{service}
}

// SolicitarSaque godoc
// @Summary     Solicitar saque
// @Description Solicita retirada de saldo via PIX (debita imediatamente, processamento manual)
// @Tags        pagamento
// @Accept      json
// @Produce     json
// @Param       body body models.SolicitarSaqueRequest true "Dados do saque"
// @Success     201 {object} utils.APIResponse
// @Router      /api/pagamento/saque [post]
func (ctrl *SaqueController) SolicitarSaque(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	var req models.SolicitarSaqueRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo inválido")
	}
	resp, err := ctrl.service.Solicitar(userID, &req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusCreated, "Saque solicitado com sucesso", resp)
}

// ListarSaques godoc
// @Summary     Listar saques
// @Description Lista os saques solicitados pelo usuário autenticado
// @Tags        pagamento
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/pagamento/saque [get]
func (ctrl *SaqueController) ListarSaques(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	resp, err := ctrl.service.Listar(userID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	if resp == nil {
		resp = []models.SaqueResponse{}
	}
	return utils.Success(c, fiber.StatusOK, "Saques listados", resp)
}
