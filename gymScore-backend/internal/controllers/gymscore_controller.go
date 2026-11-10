package controllers

import (
	"strconv"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// GymScoreController expõe o Nível GymScore e os gráficos de teia do atleta (rodada 6, U3/U5) e a divisão
// padrão do prêmio (U1).
type GymScoreController struct {
	service services.GymScoreService
}

// NovoGymScoreController cria o controller.
func NovoGymScoreController(service services.GymScoreService) *GymScoreController {
	return &GymScoreController{service: service}
}

// Estatisticas godoc
// @Summary     Estatísticas do atleta (nível e gráficos de teia)
// @Description Nível GymScore (1–10), pontuação, quanto falta para o próximo nível, 6 atributos e 8 categorias (0–100, últimos 180 dias) e o texto "como calculamos". Mesma visibilidade do perfil público (qualquer usuário logado).
// @Tags        usuarios
// @Produce     json
// @Param       id path int true "ID do usuário"
// @Success     200 {object} utils.APIResponse{data=models.EstatisticasResponse}
// @Failure     404 {object} utils.APIResponse
// @Router      /api/usuarios/{id}/estatisticas [get]
func (ctrl *GymScoreController) Estatisticas(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || id == 0 {
		return utils.ValidationError(c, "ID inválido")
	}
	resp, err := ctrl.service.Estatisticas(uint(id))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Estatísticas do atleta", resp)
}

// DivisaoPadraoResponse é a divisão sugerida do prêmio para N posições.
type DivisaoPadraoResponse struct {
	Posicoes        int   `json:"posicoes"`
	DivisaoPremioBP []int `json:"divisao_premio_bp"`
}

// DivisaoPadrao godoc
// @Summary     Divisão padrão do prêmio
// @Description Divisão sugerida (pontos-base, soma 10000, não crescente) para um pódio de `posicoes` (1–100) posições: até 5 é a tabela clássica (60/30/10…); acima, pesos lineares decrescentes.
// @Tags        desafios
// @Produce     json
// @Param       posicoes query int true "Número de posições premiadas (1–100)"
// @Success     200 {object} utils.APIResponse{data=controllers.DivisaoPadraoResponse}
// @Router      /api/desafios/divisao-padrao [get]
func (ctrl *GymScoreController) DivisaoPadrao(c *fiber.Ctx) error {
	n, err := strconv.Atoi(c.Query("posicoes"))
	if err != nil || n < models.PosicoesPodioMin || n > models.PosicoesPodioTeto {
		return utils.ValidationError(c, "posicoes deve estar entre 1 e 100")
	}
	return utils.Success(c, fiber.StatusOK, "Divisão padrão", DivisaoPadraoResponse{Posicoes: n, DivisaoPremioBP: services.DivisaoPadraoBP(n)})
}
