package controllers

import (
	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"
)

// TreinoController gerencia as requisições HTTP de treinos.
type TreinoController struct {
	service services.TreinoService
}

func NovoTreinoController(service services.TreinoService) *TreinoController {
	return &TreinoController{service: service}
}

// CriarTreino godoc
// @Summary Criar um treino
// @Router  /api/treinos [post]
func (ctrl *TreinoController) CriarTreino(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.CriarTreinoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	treino, err := ctrl.service.Criar(idUsuario, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Treino criado com sucesso", treino)
}

// ListarTreinos godoc
// @Summary Listar treinos do usuário autenticado
// @Router  /api/treinos [get]
func (ctrl *TreinoController) ListarTreinos(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	treinos, err := ctrl.service.ListarPorUsuario(idUsuario)
	if err != nil {
		return responderErro(c, err)
	}
	if treinos == nil {
		treinos = []models.Treino{}
	}
	return utils.Success(c, fiber.StatusOK, "Treinos listados", treinos)
}

// Radar godoc
// @Summary Dados do radar de grupos musculares
// @Router  /api/treinos/radar [get]
func (ctrl *TreinoController) Radar(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	radar, err := ctrl.service.Radar(idUsuario)
	if err != nil {
		return responderErro(c, err)
	}
	if radar == nil {
		radar = []models.RadarGrupo{}
	}
	return utils.Success(c, fiber.StatusOK, "Radar gerado", radar)
}

// ImportarTreino godoc
// @Summary Importar um treino pelo código
// @Router  /api/treinos/importar [post]
func (ctrl *TreinoController) ImportarTreino(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.ImportarTreinoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	treino, err := ctrl.service.Importar(idUsuario, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Treino importado com sucesso", treino)
}

// ConcluirExercicio godoc
// @Summary Registrar exercício concluído (alimenta o radar)
// @Router  /api/treinos/concluir [post]
func (ctrl *TreinoController) ConcluirExercicio(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.ConcluirExercicioRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	if err := ctrl.service.ConcluirExercicio(idUsuario, &req); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Exercício concluído", nil)
}

// BuscarTreino godoc
// @Summary Buscar um treino por ID
// @Router  /api/treinos/{id} [get]
func (ctrl *TreinoController) BuscarTreino(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	treino, err := ctrl.service.BuscarPorID(idUsuario, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Treino encontrado", treino)
}

// DeletarTreino godoc
// @Summary Excluir um treino
// @Router  /api/treinos/{id} [delete]
func (ctrl *TreinoController) DeletarTreino(c *fiber.Ctx) error {
	idUsuario, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	if err := ctrl.service.Deletar(id, idUsuario); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Treino excluído", nil)
}
