package controllers

import (
	"io"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// ModeracaoController expõe a moderação para o usuário (denúncias próprias, punições, recurso),
// para a academia julgadora e a rota pública de recurso do banido (Q1).
type ModeracaoController struct {
	svc services.ModeracaoService
	// maxEvidencia é o limite por imagem (DENUNCIA_EVIDENCIA_MAX_BYTES).
	maxEvidencia int64
}

// NovoModeracaoController cria o controller de moderação.
func NovoModeracaoController(svc services.ModeracaoService, maxEvidencia int64) *ModeracaoController {
	return &ModeracaoController{svc: svc, maxEvidencia: maxEvidencia}
}

// ─── Usuário ─────────────────────────────────────────────────────────────────

// CriarDenuncia godoc
// @Summary     Denunciar um usuário
// @Description Abre denúncia sigilosa (o denunciado não sabe quem denunciou). id_desafio obrigatório em suborno_instrutor, manipulacao_podio e fraude_presenca (ambos precisam ter vínculo com o desafio). Suborno de instrutor em desafio de academia vai para a academia julgar no prazo; o resto vai ao admin. Denúncias de desafio retêm os prêmios ainda a liberar. Evidências: POST /api/denuncias/{id}/evidencias.
// @Tags        moderacao
// @Accept      json
// @Produce     json
// @Param       body body models.CriarDenunciaRequest true "Denúncia"
// @Success     201 {object} utils.APIResponse{data=models.DenunciaResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse "conta_suspensa"
// @Failure     409 {object} utils.APIResponse "já existe denúncia aberta para o mesmo caso"
// @Failure     429 {object} utils.APIResponse "limite diário"
// @Security    BearerAuth
// @Router      /api/denuncias [post]
func (ctrl *ModeracaoController) CriarDenuncia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.CriarDenunciaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.svc.CriarDenuncia(uid, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Denúncia registrada", resp)
}

// EnviarEvidencia godoc
// @Summary     Enviar evidência
// @Description multipart com o campo `arquivo` (uma imagem por chamada, até 3 por denúncia; só o denunciante e só com a denúncia aberta). Tipo decidido pelos magic bytes (JPEG, PNG, WebP); arquivo salvo em pasta privada com nome aleatório.
// @Tags        moderacao
// @Accept      multipart/form-data
// @Produce     json
// @Param       id      path     int  true "ID da denúncia"
// @Param       arquivo formData file true "Imagem"
// @Success     201 {object} utils.APIResponse{data=models.EvidenciaResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Failure     413 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/denuncias/{id}/evidencias [post]
func (ctrl *ModeracaoController) EnviarEvidencia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	fh, err := c.FormFile("arquivo")
	if err != nil {
		return utils.ValidationError(c, "Envie a imagem no campo arquivo (multipart/form-data)")
	}
	if fh.Size > ctrl.maxEvidencia {
		return utils.Error(c, fiber.StatusRequestEntityTooLarge, "Imagem acima do tamanho máximo permitido")
	}
	f, err := fh.Open()
	if err != nil {
		return responderErro(c, err)
	}
	defer f.Close()
	conteudo, err := io.ReadAll(io.LimitReader(f, ctrl.maxEvidencia+1))
	if err != nil {
		return responderErro(c, err)
	}
	resp, err := ctrl.svc.EnviarEvidencia(uid, id, conteudo)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Evidência enviada", resp)
}

// MinhasDenuncias godoc
// @Summary     Minhas denúncias
// @Description Denúncias feitas pelo usuário: status, jurisdição, decisão e se geraram punição (sem detalhes da punição de terceiros).
// @Tags        moderacao
// @Produce     json
// @Param       pagina query int false "Página"
// @Param       limite query int false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.DenunciaResponse]}
// @Security    BearerAuth
// @Router      /api/denuncias/minhas [get]
func (ctrl *ModeracaoController) MinhasDenuncias(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.svc.MinhasDenuncias(uid, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// MinhaDenuncia godoc
// @Summary     Detalhe da minha denúncia
// @Tags        moderacao
// @Produce     json
// @Param       id path int true "ID da denúncia"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaResponse}
// @Failure     404 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/denuncias/{id} [get]
func (ctrl *ModeracaoController) MinhaDenuncia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.MinhaDenuncia(uid, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// EvidenciaDoDenunciante godoc
// @Summary     Arquivo de evidência (denunciante)
// @Tags        moderacao
// @Produce     image/jpeg,image/png,image/webp
// @Param       id    path int true "ID da denúncia"
// @Param       ordem path int true "Ordem (1..3)"
// @Success     200 {file} binary
// @Failure     404 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/denuncias/{id}/evidencias/{ordem} [get]
func (ctrl *ModeracaoController) EvidenciaDoDenunciante(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	return enviarEvidencia(c, ctrl.svc, services.LeitorDenunciante, uid)
}

// MinhasPunicoes godoc
// @Summary     Minhas punições
// @Description Punições recebidas (vigentes e histórico), com o recurso e se ainda dá para recorrer.
// @Tags        moderacao
// @Produce     json
// @Param       pagina query int false "Página"
// @Param       limite query int false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.PunicaoResponse]}
// @Security    BearerAuth
// @Router      /api/punicoes/minhas [get]
func (ctrl *ModeracaoController) MinhasPunicoes(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.svc.MinhasPunicoes(uid, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// AbrirRecurso godoc
// @Summary     Recorrer de uma punição
// @Description Um recurso por punição (409 no segundo), só o punido, punição não revogada e dentro de RECURSO_PRAZO_DIAS do início. Julgado só pelo admin; não suspende a punição.
// @Tags        moderacao
// @Accept      json
// @Produce     json
// @Param       id   path int                        true "ID da punição"
// @Param       body body models.AbrirRecursoRequest true "Texto (20..2000)"
// @Success     201 {object} utils.APIResponse{data=models.RecursoResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/punicoes/{id}/recurso [post]
func (ctrl *ModeracaoController) AbrirRecurso(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.AbrirRecursoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.svc.AbrirRecurso(uid, id, req.Texto)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Recurso registrado", resp)
}

// RecursoBanimento godoc
// @Summary     Recurso do banido (público)
// @Description Rota pública com rate limit (Q1): reconfere e-mail e senha e NÃO emite sessão. Sem `texto` devolve o banimento e o andamento do recurso; com `texto` (20..2000) abre o recurso (201) ou devolve o já existente (200). 401 genérico para senha errada, conta inexistente ou conta não banida (mesmo corpo); 409 só para banido sem banimento vigente. Rate limit compartilhado com /api/login.
// @Tags        moderacao
// @Accept      json
// @Produce     json
// @Param       body body models.RecursoBanimentoRequest true "Credenciais e texto opcional"
// @Success     200 {object} utils.APIResponse{data=models.BanimentoInfo}
// @Success     201 {object} utils.APIResponse{data=models.BanimentoInfo}
// @Failure     400 {object} utils.APIResponse
// @Failure     401 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Failure     429 {object} utils.APIResponse
// @Router      /api/recursos/banimento [post]
func (ctrl *ModeracaoController) RecursoBanimento(c *fiber.Ctx) error {
	var req models.RecursoBanimentoRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	info, criado, err := ctrl.svc.RecursoBanimento(&req)
	if err != nil {
		return responderErro(c, err)
	}
	if criado {
		return utils.Success(c, fiber.StatusCreated, "Recurso registrado", info)
	}
	return utils.Success(c, fiber.StatusOK, "ok", info)
}

// ─── Academia julgadora ──────────────────────────────────────────────────────

// FilaAcademia godoc
// @Summary     Denúncias da academia
// @Description Fila da jurisdição da academia logada (suborno de instrutor nos desafios dela), com o prazo. Sem o denunciante.
// @Tags        moderacao-academia
// @Produce     json
// @Param       status query string false "aberta | em_analise | procedente | improcedente | arquivada"
// @Param       pagina query int    false "Página"
// @Param       limite query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.DenunciaResponse]}
// @Security    BearerAuth
// @Router      /api/academia/denuncias [get]
func (ctrl *ModeracaoController) FilaAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	st := c.Query("status")
	switch st {
	case "", models.DenunciaAberta, models.DenunciaEmAnalise, models.DenunciaProcedente, models.DenunciaImprocedente, models.DenunciaArquivada:
	default:
		return utils.ValidationError(c, "Status inválido")
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.svc.FilaAcademia(uid, st, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// DenunciaAcademia godoc
// @Summary     Detalhe da denúncia (academia)
// @Description Sem o denunciante; inclui o histórico de punições do denunciado nesta academia. 404 quando a denúncia não está (mais) na jurisdição da academia.
// @Tags        moderacao-academia
// @Produce     json
// @Param       id path int true "ID da denúncia"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaAcademiaDetalhe}
// @Failure     404 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/denuncias/{id} [get]
func (ctrl *ModeracaoController) DenunciaAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.DenunciaAcademia(uid, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// EvidenciaAcademia godoc
// @Summary     Arquivo de evidência (academia)
// @Description Só enquanto a jurisdição for da academia logada.
// @Tags        moderacao-academia
// @Produce     image/jpeg,image/png,image/webp
// @Param       id    path int true "ID da denúncia"
// @Param       ordem path int true "Ordem (1..3)"
// @Success     200 {file} binary
// @Failure     404 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/denuncias/{id}/evidencias/{ordem} [get]
func (ctrl *ModeracaoController) EvidenciaAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	return enviarEvidencia(c, ctrl.svc, services.LeitorAcademia, uid)
}

// AssumirAcademia godoc
// @Summary     Assumir denúncia (academia)
// @Description aberta -> em_analise com a conta da academia como responsável.
// @Tags        moderacao-academia
// @Produce     json
// @Param       id path int true "ID da denúncia"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/denuncias/{id}/assumir [post]
func (ctrl *ModeracaoController) AssumirAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.AssumirAcademia(uid, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Denúncia assumida", resp)
}

// EncaminharAcademia godoc
// @Summary     Encaminhar denúncia ao admin (academia)
// @Description Caminho só de ida: jurisdição vira admin, status volta a aberta. Justificativa obrigatória.
// @Tags        moderacao-academia
// @Accept      json
// @Produce     json
// @Param       id   path int                              true "ID da denúncia"
// @Param       body body models.EncaminharDenunciaRequest true "Justificativa"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/denuncias/{id}/encaminhar [post]
func (ctrl *ModeracaoController) EncaminharAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.EncaminharDenunciaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.svc.EncaminharAcademia(uid, id, req.Justificativa)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Denúncia encaminhada ao admin", resp)
}

// DecidirAcademia godoc
// @Summary     Decidir denúncia (academia)
// @Description procedente | improcedente | arquivada com decisao_texto; punição opcional só em procedente e só advertencia ou bloqueio_academia (na própria academia). Suspensão/banimento/anulação: encaminhar ao admin.
// @Tags        moderacao-academia
// @Accept      json
// @Produce     json
// @Param       id   path int                                   true "ID da denúncia"
// @Param       body body models.DecidirDenunciaAcademiaRequest true "Decisão"
// @Success     200 {object} utils.APIResponse{data=models.DenunciaResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    BearerAuth
// @Router      /api/academia/denuncias/{id}/decidir [post]
func (ctrl *ModeracaoController) DecidirAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.DecidirDenunciaAcademiaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.svc.DecidirAcademia(uid, id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Denúncia decidida", resp)
}

// PunicoesAcademia godoc
// @Summary     Punições da academia
// @Description Punições de escopo da academia logada (advertências e bloqueios).
// @Tags        moderacao-academia
// @Produce     json
// @Param       tipo    query string false "advertencia | bloqueio_academia"
// @Param       vigente query bool   false "Só vigentes"
// @Param       pagina  query int    false "Página"
// @Param       limite  query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.PunicaoResponse]}
// @Security    BearerAuth
// @Router      /api/academia/punicoes [get]
func (ctrl *ModeracaoController) PunicoesAcademia(c *fiber.Ctx) error {
	uid, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	vig, ok := queryBool(c, "vigente")
	if !ok {
		return nil
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.svc.PunicoesAcademia(uid, c.Query("tipo"), vig != nil && *vig, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}
