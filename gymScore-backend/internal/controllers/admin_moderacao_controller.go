package controllers

import (
	"strconv"
	"strings"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// Moderação no painel admin (/api/admin). Toda escrita exige {motivo} (admin_auditoria na mesma
// transação) e passa pelo AdminCSRFMiddleware do grupo, como as demais ações do painel.

// ─── Usuários: bloquear/desbloquear (bloqueio unificado) ─────────────────────

// BloquearUsuario godoc
// @Summary     Bloquear usuário (banimento)
// @Description Aplica uma punição de banimento (global, sem fim) com o motivo como justificativa: login recusado (403 conta_banida, com caminho de recurso), sessões derrubadas. Idempotente (alterado=false se já há banimento vigente). Evento punicao_aplicada + auditoria usuario_bloquear.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do usuário"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.AcaoAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/usuarios/{id}/bloquear [post]
func (ctrl *AdminController) BloquearUsuario(c *fiber.Ctx) error {
	return ctrl.acaoBool(c, ctrl.mod.BloquearUsuario, "Usuário banido")
}

// DesbloquearUsuario godoc
// @Summary     Desbloquear usuário
// @Description Revoga os banimentos vigentes (a conta é recalculada a partir das punições restantes). Idempotente. Evento punicao_revogada + auditoria usuario_desbloquear.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do usuário"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.AcaoAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/usuarios/{id}/desbloquear [post]
func (ctrl *AdminController) DesbloquearUsuario(c *fiber.Ctx) error {
	return ctrl.acaoBool(c, ctrl.mod.DesbloquearUsuario, "Usuário desbloqueado")
}

// ─── Denúncias ───────────────────────────────────────────────────────────────

// ListarDenuncias godoc
// @Summary     Denúncias (admin)
// @Description Fila (mais antigas primeiro) com o denunciante. Inclui as da jurisdição de academias (só leitura até subirem).
// @Tags        admin-moderacao
// @Produce     json
// @Param       status         query string false "aberta | em_analise | procedente | improcedente | arquivada (lista separada por vírgula)"
// @Param       jurisdicao     query string false "admin | academia"
// @Param       categoria      query string false "Categoria"
// @Param       id_denunciado  query int    false "Denunciado"
// @Param       id_denunciante query int    false "Denunciante"
// @Param       id_desafio     query int    false "Desafio"
// @Param       pagina         query int    false "Página"
// @Param       limite         query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.DenunciaResponse]}
// @Security    AdminAuth
// @Router      /api/admin/denuncias [get]
func (ctrl *AdminController) ListarDenuncias(c *fiber.Ctx) error {
	f := repositories.FiltroDenuncias{Jurisdicao: c.Query("jurisdicao"), Categoria: c.Query("categoria")}
	for _, st := range strings.Split(c.Query("status"), ",") {
		if st = strings.TrimSpace(st); st == "" {
			continue
		}
		switch st {
		case models.DenunciaAberta, models.DenunciaEmAnalise, models.DenunciaProcedente, models.DenunciaImprocedente, models.DenunciaArquivada:
			f.Status = append(f.Status, st)
		default:
			return utils.ValidationError(c, "Status inválido")
		}
	}
	switch f.Jurisdicao {
	case "", models.JurisdicaoAdmin, models.JurisdicaoAcademia:
	default:
		return utils.ValidationError(c, "Jurisdição inválida")
	}
	for nome, dst := range map[string]**uint{"id_denunciado": &f.IDDenunciado, "id_denunciante": &f.IDDenunciante, "id_desafio": &f.IDDesafio} {
		v, ok := queryUint(c, nome)
		if !ok {
			return nil
		}
		if v != 0 {
			*dst = &v
		}
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.mod.Denuncias(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// BuscarDenuncia godoc
// @Summary     Detalhe da denúncia (admin)
// @Description Denúncia com denunciante, evidências (metadados), punições geradas, linha do tempo, outras denúncias contra o mesmo usuário e reincidência de advertências (90 dias).
// @Tags        admin-moderacao
// @Produce     json
// @Param       id path int true "ID da denúncia"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaAdminDetalhe}
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/denuncias/{id} [get]
func (ctrl *AdminController) BuscarDenuncia(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	resp, err := ctrl.mod.DenunciaAdmin(id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// EvidenciaDenuncia godoc
// @Summary     Arquivo de evidência (admin)
// @Description Imagem da evidência (ordem 1..3), servida da pasta privada com nosniff e Content-Disposition inline.
// @Tags        admin-moderacao
// @Produce     image/jpeg,image/png,image/webp
// @Param       id    path int true "ID da denúncia"
// @Param       ordem path int true "Ordem (1..3)"
// @Success     200 {file} binary
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/denuncias/{id}/evidencias/{ordem} [get]
func (ctrl *AdminController) EvidenciaDenuncia(c *fiber.Ctx) error {
	return enviarEvidencia(c, ctrl.mod, services.LeitorAdmin, 0)
}

// AssumirDenuncia godoc
// @Summary     Assumir denúncia (admin)
// @Description aberta|em_analise (jurisdição admin) -> em_analise com este admin como responsável. 409 se decidida ou ainda com a academia.
// @Tags        admin-moderacao
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID da denúncia"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaResponse}
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/denuncias/{id}/assumir [post]
func (ctrl *AdminController) AssumirDenuncia(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.mod.AssumirAdmin(ctrl.ator(c), id, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Denúncia assumida", resp)
}

// DecidirDenuncia godoc
// @Summary     Decidir denúncia (admin)
// @Description procedente | improcedente | arquivada, com decisao_texto (vai ao denunciante) e punições opcionais ao denunciado (só procedente; advertencia, suspensao com fim, banimento, bloqueio_academia com id_academia), tudo numa transação. A retenção do pódio da denúncia é liberada/repassada. Duas decisões simultâneas: uma vence, a outra 409.
// @Tags        admin-moderacao
// @Accept      json
// @Produce     json
// @Param       id   path int                                true "ID da denúncia"
// @Param       body body models.DecidirDenunciaAdminRequest true "Decisão"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaAdminDetalhe}
// @Failure     400 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/denuncias/{id}/decidir [post]
func (ctrl *AdminController) DecidirDenuncia(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.DecidirDenunciaAdminRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.mod.DecidirAdmin(ctrl.ator(c), id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Denúncia decidida", resp)
}

// ─── Punições ────────────────────────────────────────────────────────────────

// ListarPunicoes godoc
// @Summary     Punições (admin)
// @Tags        admin-moderacao
// @Produce     json
// @Param       id_usuario  query int    false "Punido"
// @Param       id_academia query int    false "Academia (escopo)"
// @Param       id_denuncia query int    false "Denúncia de origem"
// @Param       tipo        query string false "advertencia | suspensao | banimento | bloqueio_academia | anulacao_resultado"
// @Param       escopo      query string false "global | academia"
// @Param       vigente     query bool   false "Só vigentes"
// @Param       pagina      query int    false "Página"
// @Param       limite      query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.PunicaoResponse]}
// @Security    AdminAuth
// @Router      /api/admin/punicoes [get]
func (ctrl *AdminController) ListarPunicoes(c *fiber.Ctx) error {
	f := repositories.FiltroPunicoes{Tipo: c.Query("tipo"), Escopo: c.Query("escopo")}
	switch f.Tipo {
	case "", models.PunicaoAdvertencia, models.PunicaoSuspensao, models.PunicaoBanimento, models.PunicaoBloqueioAcademia, models.PunicaoAnulacaoResultado:
	default:
		return utils.ValidationError(c, "Tipo inválido")
	}
	switch f.Escopo {
	case "", models.EscopoGlobal, models.EscopoAcademia:
	default:
		return utils.ValidationError(c, "Escopo inválido")
	}
	for nome, dst := range map[string]**uint{"id_usuario": &f.IDUsuario, "id_academia": &f.IDAcademia, "id_denuncia": &f.IDDenuncia} {
		v, ok := queryUint(c, nome)
		if !ok {
			return nil
		}
		if v != 0 {
			*dst = &v
		}
	}
	vig, ok := queryBool(c, "vigente")
	if !ok {
		return nil
	}
	f.Vigentes = vig != nil && *vig
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.mod.Punicoes(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// AplicarPunicao godoc
// @Summary     Aplicar punição (admin)
// @Description Aplicação direta (com ou sem denúncia procedente). Suspensão exige fim futuro; banimento não tem fim; bloqueio_academia exige id_academia; advertência com id_academia vira escopo academia. Suspensão/banimento recalculam a conta e derrubam as sessões. anulacao_resultado tem rota própria.
// @Tags        admin-moderacao
// @Accept      json
// @Produce     json
// @Param       body body models.AplicarPunicaoRequest true "Punição"
// @Success     201 {object} utils.APIResponse{data=models.PunicaoResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/punicoes [post]
func (ctrl *AdminController) AplicarPunicao(c *fiber.Ctx) error {
	var req models.AplicarPunicaoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.mod.AplicarPunicao(ctrl.ator(c), &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Punição aplicada", resp)
}

// RevogarPunicao godoc
// @Summary     Revogar punição (admin)
// @Description Revoga e recalcula banido_em/suspenso_ate a partir das punições restantes. 409 se já revogada ou se for anulacao_resultado (Q4).
// @Tags        admin-moderacao
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID da punição"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.PunicaoResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/punicoes/{id}/revogar [post]
func (ctrl *AdminController) RevogarPunicao(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.mod.RevogarPunicao(ctrl.ator(c), id, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Punição revogada", resp)
}

// ─── Recursos ────────────────────────────────────────────────────────────────

// ListarRecursos godoc
// @Summary     Recursos (admin)
// @Description Fila de recursos (mais antigos primeiro) com a punição.
// @Tags        admin-moderacao
// @Produce     json
// @Param       status query string false "pendente | deferido | indeferido"
// @Param       pagina query int    false "Página"
// @Param       limite query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.RecursoFilaItem]}
// @Security    AdminAuth
// @Router      /api/admin/recursos [get]
func (ctrl *AdminController) ListarRecursos(c *fiber.Ctx) error {
	pagina, limite := paginacao(c)
	resp, err := ctrl.mod.Recursos(c.Query("status"), pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// JulgarRecurso godoc
// @Summary     Julgar recurso (admin)
// @Description deferido (revoga a punição e recalcula a conta; em anulacao_resultado só o histórico — Q4) ou indeferido. Dois julgamentos simultâneos: um vence, o outro 409.
// @Tags        admin-moderacao
// @Accept      json
// @Produce     json
// @Param       id   path int                         true "ID do recurso"
// @Param       body body models.JulgarRecursoRequest true "Julgamento"
// @Success     200 {object} utils.APIResponse{data=models.RecursoFilaItem}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/recursos/{id}/julgar [post]
func (ctrl *AdminController) JulgarRecurso(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.JulgarRecursoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.mod.JulgarRecurso(ctrl.ator(c), id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Recurso julgado", resp)
}

// ListarEventosModeracao godoc
// @Summary     Linha do tempo da moderação (admin)
// @Description Eventos append-only (moderacao_eventos) por desafio ou por entidade, em ordem cronológica.
// @Tags        admin-moderacao
// @Produce     json
// @Param       id_desafio  query int    false "Desafio"
// @Param       entidade    query string false "denuncia | punicao | recurso | desafio | pagamento | debito | usuario"
// @Param       id_entidade query int    false "ID da entidade"
// @Param       pagina      query int    false "Página"
// @Param       limite      query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.EventoResponse]}
// @Failure     400 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/moderacao/eventos [get]
func (ctrl *AdminController) ListarEventosModeracao(c *fiber.Ctx) error {
	idDesafio, ok := queryUint(c, "id_desafio")
	if !ok {
		return nil
	}
	idEntidade, ok := queryUint(c, "id_entidade")
	if !ok {
		return nil
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.mod.Eventos(c.Query("entidade"), idEntidade, idDesafio, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ─── Envio de evidência (comum a denunciante, academia e admin) ──────────────

// enviarEvidencia responde o arquivo da evidência com headers que impedem execução/sniffing e cache.
func enviarEvidencia(c *fiber.Ctx, mod services.ModeracaoService, papel services.PapelLeitor, idLeitor uint) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	ordem, err := strconv.ParseUint(c.Params("ordem"), 10, 8)
	if err != nil || ordem < 1 || ordem > models.DenunciaEvidenciasMaximo {
		return utils.ValidationError(c, "Ordem inválida (1 a 3)")
	}
	arq, err := mod.Evidencia(papel, idLeitor, id, uint8(ordem))
	if err != nil {
		return responderErro(c, err)
	}
	c.Set(fiber.HeaderContentType, arq.Mime)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="`+arq.Nome+`"`)
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	return c.Status(fiber.StatusOK).Send(arq.Conteudo)
}

// ─── Dinheiro do pódio: anulação, pagamentos pendentes e débitos ────────────

// AnularResultado godoc
// @Summary     Anular resultado do desafio (superadmin)
// @Description Desafio encerrado com pódio lançado na janela de contestação. Cancela os pagamentos pendentes não liberados, estorna dos saldos os já liberados (o que faltar vira débito do usuário, que bloqueia o saque) e estorna a taxa da plataforma já liberada; desfaz os pontos do pódio; aplica a punição anulacao_resultado a id_usuario_punido; decide procedente a id_denuncia (se aberta com o admin). modo=relancar_podio (só sem pagamento liberado): o pódio é removido e o desafio volta a em_andamento para a academia dona relançar. modo=devolver_inscricoes: devolve a entrada INTEGRAL a cada inscrito e a reserva à academia; o desafio fica encerrado com o resultado anulado. Irreversível (Q4). Tudo numa transação com evento e auditoria desafio_anular_resultado.
// @Tags        admin-moderacao
// @Accept      json
// @Produce     json
// @Param       id   path int                           true "ID do desafio"
// @Param       body body models.AnularResultadoRequest true "Anulação"
// @Success     200 {object} utils.APIResponse{data=models.AnulacaoResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/desafios/{id}/anular-resultado [post]
func (ctrl *AdminController) AnularResultado(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.AnularResultadoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.mod.AnularResultado(ctrl.ator(c), id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Resultado anulado", resp)
}

// ListarPagamentosPendentes godoc
// @Summary     Pagamentos pendentes do pódio (admin)
// @Description Créditos do pódio na janela de contestação, por liberar_em (mais próximos primeiro). taxa_inscricao tem id_usuario null (plataforma).
// @Tags        admin-moderacao
// @Produce     json
// @Param       status     query string false "a_liberar | retido | liberado | cancelado (lista separada por vírgula)"
// @Param       id_desafio query int    false "Desafio"
// @Param       id_usuario query int    false "Beneficiário"
// @Param       tipo       query string false "premio_desafio | pagamento_instrutor | receita_academia | taxa_inscricao"
// @Param       pagina     query int    false "Página"
// @Param       limite     query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.PagamentoPendenteResponse]}
// @Security    AdminAuth
// @Router      /api/admin/pagamentos-pendentes [get]
func (ctrl *AdminController) ListarPagamentosPendentes(c *fiber.Ctx) error {
	f := repositories.FiltroPendentes{Tipo: c.Query("tipo")}
	switch f.Tipo {
	case "", models.PendentePremio, models.PendentePagamentoInstrutor, models.PendenteReceitaAcademia, models.PendenteTaxaInscricao:
	default:
		return utils.ValidationError(c, "Tipo inválido")
	}
	if st := strings.TrimSpace(c.Query("status")); st != "" {
		for _, v := range strings.Split(st, ",") {
			v = strings.TrimSpace(v)
			switch v {
			case models.PendenteALiberar, models.PendenteRetido, models.PendenteLiberado, models.PendenteCancelado:
				f.Status = append(f.Status, v)
			default:
				return utils.ValidationError(c, "Status inválido")
			}
		}
	}
	for nome, dst := range map[string]**uint{"id_desafio": &f.IDDesafio, "id_usuario": &f.IDUsuario} {
		v, ok := queryUint(c, nome)
		if !ok {
			return nil
		}
		if v != 0 {
			*dst = &v
		}
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.mod.PagamentosPendentes(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ListarDebitos godoc
// @Summary     Débitos de usuários (admin)
// @Description O que a anulação de resultado não conseguiu estornar do saldo. Com débito aberto o saque do usuário é bloqueado; créditos efetivos o abatem.
// @Tags        admin-moderacao
// @Produce     json
// @Param       aberto     query bool false "Só abertos (quitado_em nulo)"
// @Param       id_usuario query int  false "Devedor"
// @Param       pagina     query int  false "Página"
// @Param       limite     query int  false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.DebitoResponse]}
// @Security    AdminAuth
// @Router      /api/admin/debitos [get]
func (ctrl *AdminController) ListarDebitos(c *fiber.Ctx) error {
	aberto, ok := queryBool(c, "aberto")
	if !ok {
		return nil
	}
	idU, ok := queryUint(c, "id_usuario")
	if !ok {
		return nil
	}
	var idUsuario *uint
	if idU != 0 {
		idUsuario = &idU
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.mod.Debitos(aberto != nil && *aberto, idUsuario, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}
