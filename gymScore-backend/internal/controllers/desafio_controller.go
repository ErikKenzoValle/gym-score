package controllers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
	"gynScore-backend/pkg/utils"
)

// DesafioController gerencia as requisições HTTP relacionadas a desafios
type DesafioController struct {
	service services.DesafioService
}

// NovoDesafioController cria uma nova instância do controller de desafios
func NovoDesafioController(service services.DesafioService) *DesafioController {
	return &DesafioController{service: service}
}

// CriarDesafio godoc
// @Summary     Criar novo desafio
// @Description Cria um novo desafio após validar saldo do criador
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       body body models.CriarDesafioRequest true "Dados do desafio"
// @Success     201 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/desafios [post]
func (ctrl *DesafioController) CriarDesafio(c *fiber.Ctx) error {
	var req models.CriarDesafioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	if req.Titulo == "" || req.IDCriador == 0 || req.Valor <= 0 {
		return utils.ValidationError(c, "Campos obrigatórios: titulo, id_criador, valor (deve ser maior que zero)")
	}

	desafio, err := ctrl.service.CriarDesafio(&req)
	if err != nil {
		if err.Error() == "saldo insuficiente para criar o desafio" {
			return utils.Error(c, fiber.StatusPaymentRequired, err.Error())
		}
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao criar desafio: "+err.Error())
	}

	return utils.Success(c, fiber.StatusCreated, "Desafio criado com sucesso", desafio)
}

// AceitarDesafio godoc
// @Summary     Aceitar um desafio
// @Description Registra um usuário como participante de um desafio aberto
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       body body models.AceitarDesafioRequest true "IDs do desafio e do usuário"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/desafios/aceitar_desafio [post]
func (ctrl *DesafioController) AceitarDesafio(c *fiber.Ctx) error {
	var req models.AceitarDesafioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	if req.IDDesafio == 0 || req.IDUsuario == 0 {
		return utils.ValidationError(c, "Campos obrigatórios: id_desafio, id_usuario")
	}

	desafio, err := ctrl.service.AceitarDesafio(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "Erro ao entrar no desafio: "+err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Você entrou no desafio", fiber.Map{
		"id_desafio":    desafio.ID,
		"participantes": len(desafio.Participantes),
		"vagas":         desafio.Vagas,
		"status":        desafio.Status,
	})
}

// IniciarDesafio godoc
// @Summary     Iniciar um desafio
// @Description Muda o status do desafio para "em andamento"
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       body body models.IniciarDesafioRequest true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/desafios/iniciar [post]
func (ctrl *DesafioController) IniciarDesafio(c *fiber.Ctx) error {
	var req models.IniciarDesafioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	if req.IDDesafio == 0 {
		return utils.ValidationError(c, "Campo obrigatório: id_desafio")
	}

	desafio, err := ctrl.service.IniciarDesafio(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "Erro ao iniciar desafio: "+err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Desafio iniciado com sucesso", fiber.Map{
		"id_desafio": desafio.ID,
		"status":     desafio.Status,
	})
}

// EncerrarDesafio godoc
// @Summary     Encerrar um desafio
// @Description Finaliza o desafio, atualiza saldos do vencedor e perdedor
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       body body models.EncerrarDesafioRequest true "IDs do desafio, vencedor e perdedor"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/desafios/encerrar [post]
func (ctrl *DesafioController) EncerrarDesafio(c *fiber.Ctx) error {
	var req models.EncerrarDesafioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	if req.IDDesafio == 0 || req.IDVencedor == 0 {
		return utils.ValidationError(c, "Campos obrigatórios: id_desafio, id_vencedor")
	}

	desafio, err := ctrl.service.EncerrarDesafio(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "Erro ao encerrar desafio: "+err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Desafio encerrado com sucesso", fiber.Map{
		"id_desafio": desafio.ID,
		"vencedor":   req.IDVencedor,
		"status":     desafio.Status,
	})
}

// CancelarDesafio godoc
// @Summary     Cancelar um desafio aberto
// @Description Encerra um desafio aberto sem adversário sem alterar saldos
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       body body object true "id_desafio e id_criador"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/desafios/cancelar [post]
func (ctrl *DesafioController) CancelarDesafio(c *fiber.Ctx) error {
	var req struct {
		IDDesafio uint `json:"id_desafio"`
		IDCriador uint `json:"id_criador"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.IDDesafio == 0 {
		return utils.ValidationError(c, "Campo obrigatório: id_desafio")
	}
	if req.IDCriador == 0 {
		if uid := c.Locals("user_id"); uid != nil {
			req.IDCriador = uid.(uint)
		}
	}
	if err := ctrl.service.CancelarDesafio(req.IDDesafio, req.IDCriador); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Desafio cancelado", nil)
}

// ListarDesafios godoc
// @Summary     Listar todos os desafios
// @Description Retorna todos os desafios cadastrados no sistema
// @Tags        desafios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/view [get]
func (ctrl *DesafioController) ListarDesafios(c *fiber.Ctx) error {
	var idUsuario uint
	if uid := c.Locals("user_id"); uid != nil {
		idUsuario, _ = uid.(uint)
	}
	desafios, err := ctrl.service.Listar(idUsuario)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao listar desafios: "+err.Error())
	}

	// Lista vazia é estado válido — não retornar 404
	if desafios == nil {
		desafios = []models.Desafio{}
	}
	return utils.Success(c, fiber.StatusOK, "Desafios listados com sucesso", desafios)
}

// ListarDesafiosPorUsuario godoc
// @Summary     Listar desafios de um usuário
// @Description Retorna os desafios ativos (abertos e em andamento) de um usuário específico
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do usuário"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id} [get]
func (ctrl *DesafioController) ListarDesafiosPorUsuario(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}

	desafios, err := ctrl.service.ListarPorUsuario(uint(id))
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao listar desafios: "+err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Desafios do usuário listados com sucesso", desafios)
}
