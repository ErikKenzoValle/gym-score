package controllers

import (
	"strconv"

	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// RankingController expõe o ranking entre amigos.
type RankingController struct {
	service services.RankingService
}

func NovoRankingController(service services.RankingService) *RankingController {
	return &RankingController{service: service}
}

// Amigos godoc
// @Summary     Ranking entre amigos
// @Description Usuário do token + amigos aceitos. Pontos = exercícios concluídos + 3 x check-ins de desafio no período.
// @Tags        ranking
// @Produce     json
// @Param       dias query int false "Janela em dias (1..365, padrão 30)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/ranking/amigos [get]
func (ctrl *RankingController) Amigos(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	dias := 30
	if raw := c.Query("dias"); raw != "" || c.Context().QueryArgs().Has("dias") {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 365 {
			return utils.ValidationError(c, "dias deve ser um inteiro entre 1 e 365")
		}
		dias = n
	}
	r, err := ctrl.service.Amigos(userID, dias)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Ranking", r)
}
