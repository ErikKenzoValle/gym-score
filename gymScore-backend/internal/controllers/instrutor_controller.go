package controllers

import (
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// InstrutorController gerencia as requisições HTTP relacionadas a instrutores
type InstrutorController struct {
	service services.InstrutorService
}

// NovoInstrutorController cria uma nova instância do controller de instrutores
func NovoInstrutorController(service services.InstrutorService) *InstrutorController {
	return &InstrutorController{service: service}
}

// CriarInstrutor godoc
// @Summary     Tornar-se instrutor
// @Description O usuário LOGADO (JWT) ativa o próprio perfil de instrutor (autônomo). Não aceita id_usuario nem id_academia: o vínculo com a academia é feito por ela em POST /api/academia/instrutores. Só contas de atleta, sem inscrição em desafio aberto/em andamento. O perfil muda na hora (não precisa novo login).
// @Tags        instrutores
// @Accept      json
// @Produce     json
// @Param       body body models.CriarInstrutorRequest true "CREF e especialidade"
// @Success     201 {object} utils.APIResponse{data=models.InstrutorResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     401 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/instrutores [post]
func (ctrl *InstrutorController) CriarInstrutor(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.CriarInstrutorRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	i, err := ctrl.service.Criar(userID, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Perfil de instrutor ativado com sucesso", i)
}

// Listar godoc
// @Summary     Diretório de instrutores
// @Description Lista os instrutores cadastrados (nome, CREF, especialidade e se são autônomos). Exige login; não expõe e-mail nem CPF. Para os instrutores da sua academia use GET /api/academia/instrutores.
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse{data=[]models.InstrutorResponse}
// @Security    BearerAuth
// @Router      /api/instrutores [get]
func (ctrl *InstrutorController) Listar(c *fiber.Ctx) error {
	lista, err := ctrl.service.Listar()
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", lista)
}

// Agenda godoc
// @Summary     Agenda do instrutor
// @Description Desafios em que o instrutor logado é o instrutor designado (DesafioResponse), paginados. Sem status: abertos e em andamento por data_evento crescente. Exige perfil instrutor.
// @Tags        instrutores
// @Produce     json
// @Param       status query string false "aberto | em_andamento | encerrado | cancelado"
// @Param       pagina query int false "Página (1..)"
// @Param       limite query int false "Itens por página (padrão 20, máx. 100)"
// @Success     200 {object} utils.APIResponse{data=models.ListaDesafiosResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/instrutor/agenda [get]
func (ctrl *InstrutorController) Agenda(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.service.Agenda(userID, models.StatusDesafio(c.Query("status")),
		c.QueryInt("pagina", 1), c.QueryInt("limite", 20))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// DesvincularAcademia godoc
// @Summary     Sair da academia (instrutor)
// @Description O instrutor logado encerra o próprio vínculo com a academia. 409 se ainda for o instrutor designado de desafio aberto ou em andamento, ou se não tiver vínculo.
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/instrutor/vinculo [delete]
func (ctrl *InstrutorController) DesvincularAcademia(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	if err := ctrl.service.Desvincular(userID); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Vínculo com a academia encerrado", nil)
}

// Convite godoc
// @Summary     Convite de academia pendente (instrutor)
// @Description Devolve o convite de vínculo pendente do instrutor logado (convite = null se não houver) e o vínculo atual.
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse{data=models.ConviteInstrutorResponse}
// @Failure     403 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/instrutor/convite [get]
func (ctrl *InstrutorController) Convite(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.service.Convite(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// AceitarConvite godoc
// @Summary     Aceitar convite de academia
// @Description O instrutor logado aceita o convite pendente e passa a ser vinculado à academia. 404 sem convite; 409 se já tiver vínculo.
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/instrutor/convite/aceitar [post]
func (ctrl *InstrutorController) AceitarConvite(c *fiber.Ctx) error {
	return ctrl.responderConvite(c, true, "Convite aceito: você agora está vinculado à academia")
}

// RecusarConvite godoc
// @Summary     Recusar convite de academia
// @Description O instrutor logado recusa o convite pendente (a academia é avisada). 404 sem convite.
// @Tags        instrutores
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/instrutor/convite/recusar [post]
func (ctrl *InstrutorController) RecusarConvite(c *fiber.Ctx) error {
	return ctrl.responderConvite(c, false, "Convite recusado")
}

func (ctrl *InstrutorController) responderConvite(c *fiber.Ctx, aceitar bool, msg string) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	if err := ctrl.service.ResponderConvite(userID, aceitar); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, msg, nil)
}
