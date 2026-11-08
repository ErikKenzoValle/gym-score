package controllers

import (
	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"
)

// AcademiaController gerencia as requisições HTTP relacionadas à gestão de academias
type AcademiaController struct {
	academiaSvc services.AcademiaService
	gestaoSvc   services.AcademiaGestaoService
}

// NovoAcademiaController cria uma nova instância do controller de academia
func NovoAcademiaController(academiaSvc services.AcademiaService, gestaoSvc services.AcademiaGestaoService) *AcademiaController {
	return &AcademiaController{academiaSvc: academiaSvc, gestaoSvc: gestaoSvc}
}

// CriarAcademia godoc
// @Summary     Cadastrar academia
// @Description Cria uma nova conta de perfil "academia" (usuário pessoa jurídica, sem CPF) e o registro de academia vinculado.
// @Description A conta nasce com status "pendente": só opera (criar desafios, painel) depois da aprovação do admin.
// @Tags        academia
// @Accept      json
// @Produce     json
// @Param       body body models.CriarAcademiaRequest true "Dados da academia"
// @Success     201 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse "e-mail ou CNPJ já cadastrado"
// @Failure     429 {object} utils.APIResponse
// @Router      /api/academias [post]
func (ctrl *AcademiaController) CriarAcademia(c *fiber.Ctx) error {
	var req models.CriarAcademiaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	academia, err := ctrl.academiaSvc.Criar(&req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Academia cadastrada. Aguarde a aprovação da equipe GymScore para começar a operar", academia)
}

// Dashboard godoc
// @Summary     Dashboard da academia
// @Description Indicadores só da academia logada: desafios por status, inscritos no mês, pote em jogo, taxa de presença, avaliação média e próximos eventos. Exige perfil academia.
// @Tags        academia
// @Produce     json
// @Success     200 {object} utils.APIResponse{data=models.DashboardAcademiaResponse}
// @Failure     403 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/dashboard [get]
func (ctrl *AcademiaController) Dashboard(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.gestaoSvc.Dashboard(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Dashboard carregado", resp)
}

// ListarDesafios godoc
// @Summary     Desafios da academia
// @Description Lista paginada dos desafios da academia logada, com filtro opcional de status. Exige perfil academia.
// @Tags        academia
// @Produce     json
// @Param       status query string false "aberto | em_andamento | encerrado | cancelado"
// @Param       pagina query int false "Página (1..)"
// @Param       limite query int false "Itens por página (padrão 20, máx. 100)"
// @Success     200 {object} utils.APIResponse{data=models.ListaDesafiosResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/desafios [get]
func (ctrl *AcademiaController) ListarDesafios(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.gestaoSvc.ListarDesafios(userID, models.StatusDesafio(c.Query("status")),
		c.QueryInt("pagina", 1), c.QueryInt("limite", 20))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Desafios da academia", resp)
}

// ListarInstrutores godoc
// @Summary     Instrutores vinculados
// @Description Lista paginada dos instrutores vinculados à academia logada, com a contagem de desafios ativos de cada um. Exige perfil academia.
// @Tags        academia
// @Produce     json
// @Param       pagina query int false "Página (1..)"
// @Param       limite query int false "Itens por página (padrão 20, máx. 100)"
// @Success     200 {object} utils.APIResponse{data=models.ListaInstrutoresVinculadosResponse}
// @Failure     403 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/instrutores [get]
func (ctrl *AcademiaController) ListarInstrutores(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.gestaoSvc.ListarInstrutores(userID, c.QueryInt("pagina", 1), c.QueryInt("limite", 20))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Instrutores da academia", resp)
}

// VincularInstrutor godoc
// @Summary     Convidar instrutor (vínculo)
// @Description Envia à conta de instrutor com o e-mail informado um CONVITE de vínculo com a academia logada; o vínculo só existe depois que o instrutor aceitar (POST /api/instrutor/convite/aceitar). 409 se ele já tem vínculo ou convite pendente de outra academia; repetir renova o convite. Exige perfil academia.
// @Tags        academia
// @Accept      json
// @Produce     json
// @Param       body body models.VincularInstrutorRequest true "E-mail da conta de instrutor"
// @Success     201 {object} utils.APIResponse{data=models.InstrutorVinculadoResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/instrutores [post]
func (ctrl *AcademiaController) VincularInstrutor(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.VincularInstrutorRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.gestaoSvc.VincularInstrutor(userID, req.Email)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Convite enviado: o vínculo vale depois que o instrutor aceitar", resp)
}

// DesvincularInstrutor godoc
// @Summary     Desvincular instrutor
// @Description Remove o vínculo do instrutor com a academia logada e o notifica. Bloqueado (409) se ele for o instrutor designado de desafio aberto ou em andamento. Exige perfil academia.
// @Tags        academia
// @Produce     json
// @Param       id path int true "ID do instrutor (instrutores.id)"
// @Success     200 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/instrutores/{id} [delete]
func (ctrl *AcademiaController) DesvincularInstrutor(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	if err := ctrl.gestaoSvc.DesvincularInstrutor(userID, id); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Instrutor desvinculado", nil)
}

// ListarConvites godoc
// @Summary     Convites de vínculo pendentes
// @Description Convites enviados pela academia logada que o instrutor ainda não aceitou nem recusou. Exige perfil academia.
// @Tags        academia
// @Produce     json
// @Success     200 {object} utils.APIResponse{data=[]models.InstrutorVinculadoResponse}
// @Failure     403 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/instrutores/convites [get]
func (ctrl *AcademiaController) ListarConvites(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.gestaoSvc.ListarConvites(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// CancelarConvite godoc
// @Summary     Cancelar convite de vínculo
// @Description Retira o convite pendente da academia logada para o instrutor. Exige perfil academia.
// @Tags        academia
// @Produce     json
// @Param       id path int true "ID do instrutor (instrutores.id)"
// @Success     200 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/instrutores/convites/{id} [delete]
func (ctrl *AcademiaController) CancelarConvite(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	if err := ctrl.gestaoSvc.CancelarConvite(userID, id); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Convite cancelado", nil)
}
