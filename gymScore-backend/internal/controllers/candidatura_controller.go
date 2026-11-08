package controllers

import (
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// ListarVagas godoc
// @Summary     Vagas de instrutor abertas (perfil instrutor)
// @Description Desafios abertos, com data futura, vaga de instrutor aberta e sem escolhido, do evento mais próximo ao mais distante. Cada item traz `vaga` (teto, modo de pagamento, % e a candidatura do próprio instrutor). Filtros: categoria, q, id_academia, pagina, limite.
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
	lista, err := ctrl.service.ListarVagas(userID, f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Vagas listadas", lista)
}

// Candidatar godoc
// @Summary     Candidatar-se à vaga de instrutor (perfil instrutor)
// @Description Lance em centavos <= teto da vaga e mensagem opcional. 201 ao criar (ou reabrir após retirar); 200 ao atualizar o lance de uma candidatura pendente.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                      true "ID do desafio"
// @Param       body body models.CandidatarRequest true "Lance e mensagem"
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
	if err := ctrl.service.AutorizarCandidatura(userID, id); err != nil {
		return responderErro(c, err)
	}
	var req models.CandidatarRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	r, criada, err := ctrl.service.Candidatar(userID, id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	if criada {
		return utils.Success(c, fiber.StatusCreated, "Candidatura registrada", r)
	}
	return utils.Success(c, fiber.StatusOK, "Candidatura atualizada", r)
}

// RetirarCandidatura godoc
// @Summary     Retirar a própria candidatura (perfil instrutor)
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
	r, err := ctrl.service.RetirarCandidatura(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Candidatura retirada", r)
}

// ListarCandidaturas godoc
// @Summary     Candidatos da vaga de instrutor (só a academia dona)
// @Description Nome, CREF, especialidade, lance e mensagem; do menor para o maior lance; sem as retiradas.
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
	lista, err := ctrl.service.ListarCandidaturas(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Candidaturas listadas", lista)
}

// EscolherCandidatura godoc
// @Summary     Escolher o instrutor da vaga (só a academia dona)
// @Description O lance vira o valor fixo do instrutor; a diferença teto - lance volta para a academia. Notifica o escolhido e os não escolhidos. Devolve o desafio atualizado.
// @Tags        desafios
// @Produce     json
// @Param       id  path int true "ID do desafio"
// @Param       cid path int true "ID da candidatura"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/candidaturas/{cid}/escolher [post]
func (ctrl *DesafioController) EscolherCandidatura(c *fiber.Ctx) error {
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
	d, err := ctrl.service.EscolherCandidatura(userID, id, cid)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Instrutor escolhido", d)
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
