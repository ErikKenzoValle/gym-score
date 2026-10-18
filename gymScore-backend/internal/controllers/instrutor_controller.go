package controllers

import (
	"github.com/gofiber/fiber/v2"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/internal/services"
	"gynScore-backend/pkg/utils"
)

// InstrutorController gerencia as requisições HTTP relacionadas a instrutores
type InstrutorController struct {
	service     services.InstrutorService
	desafioRepo repositories.DesafioRepository
}

// NovoInstrutorController cria uma nova instância do controller de instrutores
func NovoInstrutorController(service services.InstrutorService, desafioRepo repositories.DesafioRepository) *InstrutorController {
	return &InstrutorController{service: service, desafioRepo: desafioRepo}
}

// CriarInstrutor godoc
// @Summary     Cadastrar instrutor
// @Description Cria perfil de instrutor vinculado a um usuário existente
// @Tags        instrutores
// @Accept      json
// @Produce     json
// @Param       body body models.CriarInstrutorRequest true "Dados do instrutor"
// @Success     201 {object} utils.APIResponse
// @Router      /api/instrutores [post]
func (ctrl *InstrutorController) CriarInstrutor(c *fiber.Ctx) error {
	var req models.CriarInstrutorRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.IDUsuario == 0 || req.CREF == "" {
		return utils.ValidationError(c, "id_usuario e cref são obrigatórios")
	}
	i, err := ctrl.service.Criar(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusCreated, "Instrutor cadastrado com sucesso", i)
}

// Listar godoc
// @Summary     Listar instrutores
// @Description Retorna todos os instrutores cadastrados
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/instrutores [get]
func (ctrl *InstrutorController) Listar(c *fiber.Ctx) error {
	lista, err := ctrl.service.Listar()
	if err != nil {
		return utils.Error(c, 500, "Erro ao listar instrutores")
	}
	return utils.Success(c, 200, "ok", lista)
}

// Agenda godoc
// @Summary     Agenda do instrutor
// @Description Retorna os desafios associados ao instrutor autenticado (MVP: lista desafios abertos)
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Failure     500 {object} utils.APIResponse
// @Router      /api/instrutor/agenda [get]
func (ctrl *InstrutorController) Agenda(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	// Busca o instrutor pelo id_usuario — retorna lista vazia se não for instrutor
	inst, err := ctrl.service.BuscarPorUsuario(userID)
	if err != nil || inst == nil {
		return utils.Success(c, 200, "ok", []any{})
	}
	// MVP: retorna desafios abertos — filtro real por id_instrutor quando disponível no schema
	desafios, err := ctrl.desafioRepo.Listar()
	if err != nil {
		return utils.Error(c, 500, "Erro ao buscar agenda")
	}
	return utils.Success(c, 200, "ok", desafios)
}
