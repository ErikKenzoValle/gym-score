package controllers

import (
	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"
)

// CEPController expõe a consulta de CEP (proxy do ViaCEP) usada no cadastro/edição da academia.
type CEPController struct {
	service services.CEPService
}

// NovoCEPController cria o controller de CEP.
func NovoCEPController(service services.CEPService) *CEPController {
	return &CEPController{service: service}
}

// Buscar godoc
// @Summary     Consultar endereço pelo CEP (preenchimento automático)
// @Description Proxy do ViaCEP com timeout curto (3s), cache em memória (24h; CEP inexistente 1h) e rate limit por IP (CEP_RATE_LIMIT_MAX/min). Rota pública (o cadastro de academia não tem sessão). É só ajuda: em 404/503 o usuário preenche o endereço à mão — o cadastro nunca depende desta consulta.
// @Tags        academia
// @Produce     json
// @Param       cep path string true "CEP com ou sem máscara (01310100 ou 01310-100)"
// @Success     200 {object} utils.APIResponse{data=models.CEPResponse}
// @Failure     400 {object} utils.APIResponse "CEP fora do formato"
// @Failure     404 {object} utils.APIResponse "CEP não encontrado"
// @Failure     429 {object} utils.APIResponse
// @Failure     503 {object} utils.APIResponse "codigo cep_indisponivel: ViaCEP fora do ar/lento"
// @Router      /api/cep/{cep} [get]
func (ctrl *CEPController) Buscar(c *fiber.Ctx) error {
	r, err := ctrl.service.Buscar(c.UserContext(), c.Params("cep"))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "CEP encontrado", r)
}
