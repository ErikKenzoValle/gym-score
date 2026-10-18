package controllers

import (
	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"
)

// RecomendacaoController gerencia as requisições de recomendação personalizada de desafios
type RecomendacaoController struct {
	service services.RecomendacaoService
}

// NovoRecomendacaoController cria uma nova instância do controller de recomendações
func NovoRecomendacaoController(service services.RecomendacaoService) *RecomendacaoController {
	return &RecomendacaoController{service}
}

// Recomendados godoc
// @Summary     Desafios recomendados
// @Description Retorna até 5 desafios recomendados com razão textual personalizada
// @Tags        desafios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/recomendados [get]
func (ctrl *RecomendacaoController) Recomendados(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	resp, err := ctrl.service.Recomendar(userID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	if resp == nil {
		resp = []models.DesafioRecomendado{}
	}
	return utils.Success(c, fiber.StatusOK, "Recomendações geradas", resp)
}
