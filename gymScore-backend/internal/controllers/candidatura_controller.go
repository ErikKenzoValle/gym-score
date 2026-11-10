package controllers

import (
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// ListarVagas godoc
// @Summary     Trabalhos oferecidos (vagas na equipe de instrutores; perfil instrutor)
// @Description Desafios abertos, com data futura e vaga livre na equipe de instrutores, do evento mais próximo ao mais distante. Cada item traz `vaga` (condições POR instrutor: modo, fixo, %, previsão do %, instrutores_vagas, vagas_abertas e a candidatura do próprio instrutor). Filtros: categoria, q, id_academia, pagina, limite. Mesma rota em /api/instrutor/trabalhos.
// @Tags        instrutor
// @Produce     json
// @Param       categoria   query string false "Categoria"
// @Param       q           query string false "Texto livre (título, local, academia)"
// @Param       id_academia query int    false "Só desta academia"
// @Param       pagina      query int    false "Página (1..)"
// @Param       limite      query int    false "Itens por página (máx. 100)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/instrutor/vagas [get]
func (ctrl *DesafioController) ListarVagas(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	f, ok := filtroDaQuery(c)
	if !ok {
		return nil
	}
	lista, err := ctrl.svc(c).ListarVagas(userID, f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Vagas listadas", lista)
}

// Candidatar godoc
// @Summary     Candidatar-se à equipe de instrutores (perfil instrutor)
// @Description Aceita as condições definidas pela academia (sem lance); mensagem opcional. 201 ao criar (ou reabrir após retirar/ser recusado/removido); 200 ao atualizar a mensagem de uma candidatura pendente. 409 se a equipe está completa, o desafio não está aberto, o evento começou, já é membro ou está inscrito como atleta.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                      true "ID do desafio"
// @Param       body body models.CandidatarRequest true "Mensagem (opcional)"
// @Success     201 {object} utils.APIResponse
// @Router      /api/desafios/{id}/candidaturas [post]
func (ctrl *DesafioController) Candidatar(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	if err := ctrl.svc(c).AutorizarCandidatura(userID, id); err != nil {
		return responderErro(c, err)
	}
	var req models.CandidatarRequest
	if len(c.Body()) > 0 && !lerCorpo(c, &req) {
		return nil
	}
	r, criada, err := ctrl.svc(c).Candidatar(userID, id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	if criada {
		return utils.Success(c, fiber.StatusCreated, "Candidatura registrada", r)
	}
	return utils.Success(c, fiber.StatusOK, "Candidatura atualizada", r)
}

// RetirarCandidatura godoc
// @Summary     Retirar a própria candidatura pendente (perfil instrutor)
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/candidaturas [delete]
func (ctrl *DesafioController) RetirarCandidatura(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	r, err := ctrl.svc(c).RetirarCandidatura(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Candidatura retirada", r)
}

// ListarCandidaturas godoc
// @Summary     Candidatos à equipe de instrutores (só a academia dona)
// @Description Nome, CREF, especialidade, se é vinculado, mensagem e status; pendentes primeiro, por ordem de chegada; sem as retiradas.
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/candidaturas [get]
func (ctrl *DesafioController) ListarCandidaturas(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	lista, err := ctrl.svc(c).ListarCandidaturas(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Candidaturas listadas", lista)
}

// AceitarCandidatura godoc
// @Summary     Aceitar candidato na equipe (só a academia dona)
// @Description Desafio aberto e vaga livre. O candidato entra na equipe (sem responsável, vira o responsável). Ao preencher a última vaga, as pendentes são recusadas e avisadas. Devolve o desafio atualizado. /escolher é o nome antigo da mesma rota.
// @Tags        desafios
// @Produce     json
// @Param       id  path int true "ID do desafio"
// @Param       cid path int true "ID da candidatura"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/candidaturas/{cid}/aceitar [post]
func (ctrl *DesafioController) AceitarCandidatura(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	cid, ok := idDaRota(c, "cid")
	if !ok {
		return nil
	}
	d, err := ctrl.svc(c).AceitarCandidatura(userID, id, cid)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Candidatura aceita", d)
}

// RecusarCandidatura godoc
// @Summary     Recusar candidato (só a academia dona)
// @Description Desafio aberto; só candidatura pendente. Avisa o instrutor. Devolve a candidatura.
// @Tags        desafios
// @Produce     json
// @Param       id  path int true "ID do desafio"
// @Param       cid path int true "ID da candidatura"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/candidaturas/{cid}/recusar [post]
func (ctrl *DesafioController) RecusarCandidatura(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	cid, ok := idDaRota(c, "cid")
	if !ok {
		return nil
	}
	r, err := ctrl.svc(c).RecusarCandidatura(userID, id, cid)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Candidatura recusada", r)
}

// IndicarInstrutor godoc
// @Summary     Indicar instrutor vinculado para a equipe (só a academia dona)
// @Description Desafio aberto e vaga livre; o instrutor precisa estar vinculado à academia e aprovado. Sem responsável, vira o responsável. Devolve o desafio atualizado.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                            true "ID do desafio"
// @Param       body body models.IndicarInstrutorRequest true "id_instrutor (instrutores.id)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/equipe [post]
func (ctrl *DesafioController) IndicarInstrutor(c *fiber.Ctx) error {
	userID, id, ok := ctrl.autorizarDono(c)
	if !ok {
		return nil
	}
	var req models.IndicarInstrutorRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	d, err := ctrl.svc(c).IndicarInstrutor(userID, id, req.IDInstrutor)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Instrutor indicado", d)
}

// RemoverDaEquipe godoc
// @Summary     Remover instrutor da equipe (só a academia dona, antes de iniciar)
// @Description Avisa o removido. Se era o responsável, o próximo pela regra padrão (primeiro indicado; sem indicados, o primeiro aceito) assume. Devolve o desafio atualizado.
// @Tags        desafios
// @Produce     json
// @Param       id  path int true "ID do desafio"
// @Param       iid path int true "instrutores.id do membro"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/equipe/{iid} [delete]
func (ctrl *DesafioController) RemoverDaEquipe(c *fiber.Ctx) error {
	userID, id, ok := ctrl.autorizarDono(c)
	if !ok {
		return nil
	}
	iid, ok := idDaRota(c, "iid")
	if !ok {
		return nil
	}
	d, err := ctrl.svc(c).RemoverDaEquipe(userID, id, iid)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Instrutor removido da equipe", d)
}

// DefinirResponsavel godoc
// @Summary     Definir o instrutor responsável (só a academia dona)
// @Description Desafio aberto ou em andamento; o instrutor precisa ser membro da equipe. O responsável é o único instrutor que inicia e lança o pódio. Devolve o desafio atualizado.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                       true "ID do desafio"
// @Param       body body models.ResponsavelRequest true "id_instrutor (instrutores.id)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/equipe/responsavel [put]
func (ctrl *DesafioController) DefinirResponsavel(c *fiber.Ctx) error {
	userID, id, ok := ctrl.autorizarDono(c)
	if !ok {
		return nil
	}
	var req models.ResponsavelRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	d, err := ctrl.svc(c).DefinirResponsavel(userID, id, req.IDInstrutor)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Responsável definido", d)
}

// autorizarDono lê usuário e :id e confere 404/403 (academia dona) ANTES de validar o corpo.
func (ctrl *DesafioController) autorizarDono(c *fiber.Ctx) (uint, uint, bool) {
	userID, ok := exigirUsuario(c)
	if !ok {
		return 0, 0, false
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return 0, 0, false
	}
	if err := ctrl.svc(c).AutorizarEdicao(userID, id); err != nil {
		_ = responderErro(c, err)
		return 0, 0, false
	}
	return userID, id, true
}

// PlataformaController expõe as taxas vigentes (a UI mostra o máximo do pote e a tarifa de saque).
type PlataformaController struct{ cfg *config.Config }

func NovoPlataformaController(cfg *config.Config) *PlataformaController {
	return &PlataformaController{cfg: cfg}
}

// Taxas godoc
// @Summary     Taxas vigentes da plataforma
// @Description taxa_inscricao_pct é gravada no desafio na criação; tarifa_saque_centavos é 0 fora do modo inscricao_saque.
// @Tags        plataforma
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/plataforma/taxas [get]
func (ctrl *PlataformaController) Taxas(c *fiber.Ctx) error {
	modo := ctrl.cfg.PlataformaTaxaModo
	if modo == "" {
		modo = config.TaxaModoInscricao
	}
	return utils.Success(c, fiber.StatusOK, "Taxas da plataforma", models.TaxasPlataformaResponse{
		Modo: modo, TaxaInscricaoPct: ctrl.cfg.PlataformaTaxaInscricaoPct, TarifaSaqueCentavos: ctrl.cfg.TarifaSaqueAtiva(),
	})
}
