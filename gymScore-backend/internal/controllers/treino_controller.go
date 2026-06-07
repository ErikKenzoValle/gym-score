package controllers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
	"gynScore-backend/pkg/utils"
)

// TreinoController gerencia as requisições HTTP de treinos.
type TreinoController struct {
	service services.TreinoService
}

func NovoTreinoController(service services.TreinoService) *TreinoController {
	return &TreinoController{service: service}
}

// usuarioAutenticado retorna o ID do usuário do contexto JWT (0 se ausente).
func usuarioAutenticado(c *fiber.Ctx) uint {
	if uid := c.Locals("user_id"); uid != nil {
		if id, ok := uid.(uint); ok {
			return id
		}
	}
	return 0
}

// CriarTreino godoc
// @Summary Criar um treino
// @Router  /api/treinos [post]
func (ctrl *TreinoController) CriarTreino(c *fiber.Ctx) error {
	var req models.CriarTreinoRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.IDUsuario == 0 {
		req.IDUsuario = usuarioAutenticado(c)
	}

	treino, err := ctrl.service.Criar(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusCreated, "Treino criado com sucesso", treino)
}

// ListarTreinos godoc
// @Summary Listar treinos do usuário autenticado
// @Router  /api/treinos [get]
func (ctrl *TreinoController) ListarTreinos(c *fiber.Ctx) error {
	idUsuario := usuarioAutenticado(c)
	if idUsuario == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	treinos, err := ctrl.service.ListarPorUsuario(idUsuario)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao listar treinos: "+err.Error())
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
	idUsuario := usuarioAutenticado(c)
	if idUsuario == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	radar, err := ctrl.service.Radar(idUsuario)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao montar radar: "+err.Error())
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
	var req models.ImportarTreinoRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.IDUsuario == 0 {
		req.IDUsuario = usuarioAutenticado(c)
	}
	treino, err := ctrl.service.Importar(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Treino importado com sucesso", treino)
}

// ConcluirExercicio godoc
// @Summary Registrar exercício concluído (alimenta o radar)
// @Router  /api/treinos/concluir [post]
func (ctrl *TreinoController) ConcluirExercicio(c *fiber.Ctx) error {
	var req models.ConcluirExercicioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.IDUsuario == 0 {
		req.IDUsuario = usuarioAutenticado(c)
	}
	if err := ctrl.service.ConcluirExercicio(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Exercício concluído", nil)
}

// BuscarTreino godoc
// @Summary Buscar um treino por ID
// @Router  /api/treinos/{id} [get]
func (ctrl *TreinoController) BuscarTreino(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	treino, err := ctrl.service.BuscarPorID(uint(id))
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao buscar treino: "+err.Error())
	}
	if treino == nil {
		return utils.Error(c, fiber.StatusNotFound, "Treino não encontrado")
	}
	return utils.Success(c, fiber.StatusOK, "Treino encontrado", treino)
}

// DeletarTreino godoc
// @Summary Excluir um treino
// @Router  /api/treinos/{id} [delete]
func (ctrl *TreinoController) DeletarTreino(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	idUsuario := usuarioAutenticado(c)
	if err := ctrl.service.Deletar(uint(id), idUsuario); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Treino excluído", nil)
}
