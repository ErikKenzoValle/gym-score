package controllers

import (
	"strconv"
	"strings"
	"time"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// AdminController expõe a API do painel admin (/api/admin). A identidade do admin vem sempre do
// AdminAuthMiddleware (Locals), nunca do corpo.
type AdminController struct {
	svc services.AdminService
	mod services.ModeracaoService // denúncias, punições, recursos e o bloquear/desbloquear do painel
	cfg *config.Config
}

// NovoAdminController cria o controller do painel admin.
func NovoAdminController(svc services.AdminService, cfg *config.Config) *AdminController {
	return &AdminController{svc: svc, cfg: cfg}
}

// ComModeracao liga o serviço de moderação (rotas de denúncias/punições/recursos e bloquear/desbloquear).
func (ctrl *AdminController) ComModeracao(mod services.ModeracaoService) *AdminController {
	ctrl.mod = mod
	return ctrl
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (ctrl *AdminController) ator(c *fiber.Ctx) services.AtorAdmin {
	id, _ := c.Locals(middlewares.LocalAdminID).(uint)
	papel, _ := c.Locals(middlewares.LocalAdminPapel).(string)
	return services.AtorAdmin{ID: id, Papel: papel, IP: c.IP()}
}

func paginacao(c *fiber.Ctx) (int, int) {
	return c.QueryInt("pagina", 1), c.QueryInt("limite", 20)
}

// queryUint lê um id opcional da query (0 = ausente). ok=false já respondeu 400.
func queryUint(c *fiber.Ctx, nome string) (uint, bool) {
	v := strings.TrimSpace(c.Query(nome))
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		_ = utils.ValidationError(c, "Parâmetro "+nome+" inválido")
		return 0, false
	}
	return uint(n), true
}

// queryBool lê true/false opcional (nil = ausente).
func queryBool(c *fiber.Ctx, nome string) (*bool, bool) {
	switch strings.ToLower(strings.TrimSpace(c.Query(nome))) {
	case "":
		return nil, true
	case "true", "1", "sim":
		v := true
		return &v, true
	case "false", "0", "nao", "não":
		v := false
		return &v, true
	}
	_ = utils.ValidationError(c, "Parâmetro "+nome+" deve ser true ou false")
	return nil, false
}

// queryData lê uma data (AAAA-MM-DD, fuso local) ou instante RFC3339. `ate` com só a data vira o dia
// seguinte (intervalo [de, ate) inclui o dia inteiro informado).
func queryData(c *fiber.Ctx, nome string, fimDoDia bool) (*time.Time, bool) {
	v := strings.TrimSpace(c.Query(nome))
	if v == "" {
		return nil, true
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		if fimDoDia {
			t = t.AddDate(0, 0, 1)
		}
		return &t, true
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, true
	}
	_ = utils.ValidationError(c, "Parâmetro "+nome+" deve ser uma data AAAA-MM-DD ou RFC3339")
	return nil, false
}

func periodo(c *fiber.Ctx) (repositories.Periodo, bool) {
	de, ok := queryData(c, "de", false)
	if !ok {
		return repositories.Periodo{}, false
	}
	ate, ok := queryData(c, "ate", true)
	if !ok {
		return repositories.Periodo{}, false
	}
	return repositories.Periodo{De: de, Ate: ate}, true
}

// lerMotivo lê o corpo {"motivo": "..."} (obrigatório, 3..500).
func lerMotivo(c *fiber.Ctx) (string, bool) {
	var req models.MotivoRequest
	if !lerCorpo(c, &req) {
		return "", false
	}
	return req.Motivo, true
}

// ─── Autenticação ────────────────────────────────────────────────────────────

// Login godoc
// @Summary     Login no painel admin
// @Description Autentica um administrador e grava o cookie `admin_jwt` (HttpOnly, SameSite=Strict, Path /api/admin, validade ADMIN_TOKEN_TTL). O token NÃO vem no corpo. Mensagem genérica para qualquer falha; rate limit rígido por IP. Sem header Authorization exige Origin/Referer confiável.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       body body models.AdminLoginRequest true "Credenciais"
// @Success     200 {object} utils.APIResponse{data=models.AdminResponse}
// @Failure     400 {object} utils.APIResponse
// @Failure     401 {object} utils.APIResponse
// @Failure     403 {object} utils.APIResponse "origem não permitida"
// @Failure     429 {object} utils.APIResponse
// @Router      /api/admin/auth/login [post]
func (ctrl *AdminController) Login(c *fiber.Ctx) error {
	var req models.AdminLoginRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	admin, token, err := ctrl.svc.Login(req.Email, req.Senha, c.IP())
	if err != nil {
		return responderErro(c, err)
	}
	c.Cookie(middlewares.CookieSessaoAdmin(ctrl.cfg, token))
	return utils.Success(c, fiber.StatusOK, "Login realizado", admin)
}

// Logout godoc
// @Summary     Logout do painel admin
// @Description Revoga todos os tokens do admin (incrementa token_versao), grava auditoria e limpa o cookie.
// @Tags        admin
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Failure     401 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/auth/logout [post]
func (ctrl *AdminController) Logout(c *fiber.Ctx) error {
	if err := ctrl.svc.Logout(ctrl.ator(c)); err != nil {
		return responderErro(c, err)
	}
	c.Cookie(middlewares.CookieSessaoAdminApagar(ctrl.cfg))
	return utils.Success(c, fiber.StatusOK, "Sessão encerrada", nil)
}

// Me godoc
// @Summary     Admin logado
// @Description Dados do administrador autenticado (papel vigente no banco).
// @Tags        admin
// @Produce     json
// @Success     200 {object} utils.APIResponse{data=models.AdminResponse}
// @Failure     401 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/auth/me [get]
func (ctrl *AdminController) Me(c *fiber.Ctx) error {
	resp, err := ctrl.svc.Me(ctrl.ator(c).ID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// TrocarSenha godoc
// @Summary     Trocar a própria senha (admin)
// @Description Troca a senha do admin logado (mínimo 12 caracteres), revoga os tokens antigos e grava um cookie novo.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       body body models.AdminTrocarSenhaRequest true "Senhas"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/auth/senha [post]
func (ctrl *AdminController) TrocarSenha(c *fiber.Ctx) error {
	var req models.AdminTrocarSenhaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	token, err := ctrl.svc.TrocarSenha(ctrl.ator(c), req.SenhaAtual, req.NovaSenha)
	if err != nil {
		return responderErro(c, err)
	}
	c.Cookie(middlewares.CookieSessaoAdmin(ctrl.cfg, token))
	return utils.Success(c, fiber.StatusOK, "Senha alterada", nil)
}

// ─── Painel ──────────────────────────────────────────────────────────────────

// Dashboard godoc
// @Summary     Dashboard do admin
// @Description Saldo total em carteiras, custódia em desafios ativos (entradas + reservas), receita da plataforma por origem no período, conferência "saldos + custódia + receitas = depósitos − saques (+ ajustes)" com a divergência, e pendências (saques pendentes/em revisão, desafios vencidos, academias/instrutores pendentes). Valores em centavos.
// @Tags        admin
// @Produce     json
// @Param       de  query string false "Início do período da receita (AAAA-MM-DD ou RFC3339)"
// @Param       ate query string false "Fim do período (inclusive, AAAA-MM-DD)"
// @Success     200 {object} utils.APIResponse{data=models.DashboardAdminResponse}
// @Security    AdminAuth
// @Router      /api/admin/dashboard [get]
func (ctrl *AdminController) Dashboard(c *fiber.Ctx) error {
	p, ok := periodo(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.Dashboard(p.De, p.Ate)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// tiposLedger são os tipos aceitos no filtro de transações.
var tiposLedger = map[string]bool{
	string(models.TipoDeposito): true, string(models.TipoSaque): true, string(models.TipoEntradaDesafio): true,
	string(models.TipoEstornoDesafio): true, string(models.TipoPremioDesafio): true, string(models.TipoAjuste): true,
	string(models.TipoReservaDesafio): true, string(models.TipoEstornoReserva): true, string(models.TipoPagamentoInstrutor): true,
	string(models.TipoReceitaAcademia): true, string(models.TipoTarifaSaque): true,
}

var statusLedger = map[string]bool{
	models.TransacaoPending: true, models.TransacaoReceived: true, models.TransacaoRefunded: true,
	models.TransacaoFailed: true, models.TransacaoConcluido: true,
}

// ListarTransacoes godoc
// @Summary     Ledger (admin)
// @Description Lista paginada do ledger com filtros. Tipos: deposito, saque, entrada_desafio, estorno_desafio, premio_desafio, ajuste, reserva_desafio, estorno_reserva, pagamento_instrutor, receita_academia, tarifa_saque. Status: pending, received, refunded, failed, concluido.
// @Tags        admin
// @Produce     json
// @Param       tipo       query string false "Tipo"
// @Param       status     query string false "Status"
// @Param       id_usuario query int    false "Usuário"
// @Param       id_desafio query int    false "Desafio"
// @Param       em_revisao query bool   false "Só saques em revisão (true) / fora (false)"
// @Param       de         query string false "Início (AAAA-MM-DD)"
// @Param       ate        query string false "Fim inclusive (AAAA-MM-DD)"
// @Param       pagina     query int    false "Página (1..)"
// @Param       limite     query int    false "Itens por página (1..100, padrão 20)"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.TransacaoAdminResponse]}
// @Failure     400 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/transacoes [get]
func (ctrl *AdminController) ListarTransacoes(c *fiber.Ctx) error {
	f := repositories.FiltroTransacoesAdmin{Tipo: models.TipoTransacao(c.Query("tipo")), Status: c.Query("status")}
	if f.Tipo != "" && !tiposLedger[string(f.Tipo)] {
		return utils.ValidationError(c, "Tipo de transação inválido")
	}
	if f.Status != "" && !statusLedger[f.Status] {
		return utils.ValidationError(c, "Status de transação inválido")
	}
	var ok bool
	if f.IDUsuario, ok = queryUint(c, "id_usuario"); !ok {
		return nil
	}
	if f.IDDesafio, ok = queryUint(c, "id_desafio"); !ok {
		return nil
	}
	if f.EmRevisao, ok = queryBool(c, "em_revisao"); !ok {
		return nil
	}
	if f.Periodo, ok = periodo(c); !ok {
		return nil
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Transacoes(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// BuscarTransacao godoc
// @Summary     Detalhe de transação (admin)
// @Description Linha do ledger com o usuário, a transação de origem e as derivadas (ex.: tarifa do saque).
// @Tags        admin
// @Produce     json
// @Param       id path int true "ID da transação"
// @Success     200 {object} utils.APIResponse{data=models.TransacaoDetalheAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/transacoes/{id} [get]
func (ctrl *AdminController) BuscarTransacao(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.Transacao(id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ─── Saques ──────────────────────────────────────────────────────────────────

// ListarSaques godoc
// @Summary     Saques (admin)
// @Description Saques com tarifa cobrada, usuário e referência do Asaas (saque-<id>).
// @Tags        admin
// @Produce     json
// @Param       status     query string false "pending | concluido | failed"
// @Param       em_revisao query bool   false "Só em revisão"
// @Param       pagina     query int    false "Página"
// @Param       limite     query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.SaqueAdminResponse]}
// @Security    AdminAuth
// @Router      /api/admin/saques [get]
func (ctrl *AdminController) ListarSaques(c *fiber.Ctx) error {
	status := c.Query("status")
	if status != "" && !statusLedger[status] {
		return utils.ValidationError(c, "Status inválido")
	}
	emRevisao, ok := queryBool(c, "em_revisao")
	if !ok {
		return nil
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.svc.Saques(status, emRevisao, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

func (ctrl *AdminController) acaoSaque(c *fiber.Ctx, fn func(services.AtorAdmin, uint, string) (*models.SaqueAcaoResponse, error), msg string) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	resp, err := fn(ctrl.ator(c), id, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	if !resp.Alterado {
		msg = "Nada a fazer: o saque já estava nesse estado"
	}
	return utils.Success(c, fiber.StatusOK, msg, resp)
}

// ReconciliarSaque godoc
// @Summary     Reconciliar saque com o Asaas
// @Description Consulta a transferência no Asaas e aplica o status (DONE conclui; FAILED/CANCELLED reembolsa valor + tarifa). Só saque pending com transferência registrada (409 caso contrário). 502 se o Asaas não responder. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do saque"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.SaqueAcaoResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Failure     502 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/saques/{id}/reconciliar [post]
func (ctrl *AdminController) ReconciliarSaque(c *fiber.Ctx) error {
	return ctrl.acaoSaque(c, ctrl.svc.ReconciliarSaque, "Saque reconciliado")
}

// ReembolsarSaque godoc
// @Summary     Reembolsar saque (superadmin)
// @Description Devolve valor + tarifa de um saque pending (inclusive em revisão), pelo mesmo método idempotente do sistema. Repetir devolve alterado=false. Recusa (409) saque concluído ou transferência DONE no Asaas. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do saque"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.SaqueAcaoResponse}
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Failure     502 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/saques/{id}/reembolsar [post]
func (ctrl *AdminController) ReembolsarSaque(c *fiber.Ctx) error {
	return ctrl.acaoSaque(c, ctrl.svc.ReembolsarSaque, "Saque reembolsado")
}

// MarcarSaquePago godoc
// @Summary     Marcar saque como pago (superadmin)
// @Description Conclui um saque pending conferido no Asaas (a tarifa continua cobrada). Idempotente. Recusa (409) saque já reembolsado ou transferência FAILED/CANCELLED no Asaas. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do saque"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.SaqueAcaoResponse}
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/saques/{id}/marcar-pago [post]
func (ctrl *AdminController) MarcarSaquePago(c *fiber.Ctx) error {
	return ctrl.acaoSaque(c, ctrl.svc.MarcarSaquePago, "Saque marcado como pago")
}

// ─── Usuários ────────────────────────────────────────────────────────────────

// ListarUsuarios godoc
// @Summary     Usuários (admin)
// @Description Busca paginada (q em nome, sobrenome, e-mail, username; CPF exato). CPF sempre mascarado.
// @Tags        admin
// @Produce     json
// @Param       q         query string false "Busca"
// @Param       perfil    query string false "atleta (tipo de conta, inclui quem também é instrutor) | academia | instrutor (conta com o modo instrutor aprovado)"
// @Param       bloqueado query bool   false "Filtrar banidos (compatibilidade; prefira situacao)"
// @Param       situacao  query string false "ativo | suspenso | banido"
// @Param       pagina    query int    false "Página"
// @Param       limite    query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.UsuarioAdminResponse]}
// @Security    AdminAuth
// @Router      /api/admin/usuarios [get]
func (ctrl *AdminController) ListarUsuarios(c *fiber.Ctx) error {
	f := repositories.FiltroUsuariosAdmin{Q: c.Query("q"), Perfil: c.Query("perfil")}
	switch f.Perfil {
	case "", "atleta", "academia", "instrutor":
	default:
		return utils.ValidationError(c, "Perfil inválido")
	}
	if len(f.Q) > 150 {
		return utils.ValidationError(c, "Busca muito longa")
	}
	var ok bool
	if f.Bloqueado, ok = queryBool(c, "bloqueado"); !ok {
		return nil
	}
	switch f.Situacao = c.Query("situacao"); f.Situacao {
	case "", repositories.SituacaoAtivo, repositories.SituacaoSuspenso, repositories.SituacaoBanido:
	default:
		return utils.ValidationError(c, "Situação inválida (ativo, suspenso ou banido)")
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Usuarios(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// BuscarUsuario godoc
// @Summary     Detalhe de usuário (admin)
// @Description Perfil (CPF mascarado), saldo e conferência com o ledger, ledger paginado, desafios inscritos e cadastro de academia/instrutor.
// @Tags        admin
// @Produce     json
// @Param       id     path  int true  "ID do usuário"
// @Param       pagina query int false "Página do ledger"
// @Param       limite query int false "Itens por página do ledger"
// @Success     200 {object} utils.APIResponse{data=models.UsuarioDetalheAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/usuarios/{id} [get]
func (ctrl *AdminController) BuscarUsuario(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	pagina, limite := paginacao(c)
	resp, err := ctrl.svc.Usuario(id, pagina, limite)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

func (ctrl *AdminController) acaoBool(c *fiber.Ctx, fn func(services.AtorAdmin, uint, string) (bool, error), msg string) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	alterado, err := fn(ctrl.ator(c), id, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	if !alterado {
		msg = "Nada a fazer: já estava nesse estado"
	}
	return utils.Success(c, fiber.StatusOK, msg, models.AcaoAdminResponse{Alterado: alterado})
}

// ─── Desafios ────────────────────────────────────────────────────────────────

// ListarDesafios godoc
// @Summary     Desafios (admin)
// @Description Lista paginada; vencidos=true traz abertos/em andamento com data_evento + carência no passado.
// @Tags        admin
// @Produce     json
// @Param       status      query string false "aberto | em_andamento | encerrado | cancelado"
// @Param       id_academia query int    false "Academia"
// @Param       vencidos    query bool   false "Só vencidos"
// @Param       pagina      query int    false "Página"
// @Param       limite      query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.DesafioResumoAdmin]}
// @Security    AdminAuth
// @Router      /api/admin/desafios [get]
func (ctrl *AdminController) ListarDesafios(c *fiber.Ctx) error {
	f := repositories.FiltroDesafiosAdmin{Status: models.StatusDesafio(c.Query("status"))}
	switch f.Status {
	case "", models.StatusAberto, models.StatusEmAndamento, models.StatusEncerrado, models.StatusCancelado:
	default:
		return utils.ValidationError(c, "Status inválido")
	}
	var ok bool
	if f.IDAcademia, ok = queryUint(c, "id_academia"); !ok {
		return nil
	}
	vencidos, ok := queryBool(c, "vencidos")
	if !ok {
		return nil
	}
	f.Vencidos = vencidos != nil && *vencidos
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Desafios(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// BuscarDesafio godoc
// @Summary     Detalhe de desafio (admin)
// @Description Configuração financeira e totais do ledger, participantes com presença (qr/manual e quem marcou), candidaturas, pódio e ledger do desafio.
// @Tags        admin
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse{data=models.DesafioDetalheAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/desafios/{id} [get]
func (ctrl *AdminController) BuscarDesafio(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.Desafio(id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// CancelarDesafio godoc
// @Summary     Cancelar desafio (admin)
// @Description Cancela um desafio aberto/em andamento com a mesma lógica do cancelamento da academia (estorno integral das entradas, reserva devolvida, candidaturas recusadas, notificações). 409 se já encerrado/cancelado. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do desafio"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.DesafioDetalheAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/desafios/{id}/cancelar [post]
func (ctrl *AdminController) CancelarDesafio(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.CancelarDesafio(ctrl.ator(c), id, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Desafio cancelado", resp)
}

// ─── Academias / instrutores ─────────────────────────────────────────────────

func filtroCadastro(c *fiber.Ctx) (repositories.FiltroCadastrosAdmin, bool) {
	f := repositories.FiltroCadastrosAdmin{Status: c.Query("status", models.CadastroPendente)}
	switch f.Status {
	case "todos":
		f.Status = ""
	case models.CadastroPendente, models.CadastroAprovado, models.CadastroRejeitado:
	default:
		_ = utils.ValidationError(c, "Status inválido (pendente, aprovada, rejeitada ou todos)")
		return f, false
	}
	f.Pagina, f.Limite = paginacao(c)
	return f, true
}

// ListarAcademias godoc
// @Summary     Academias (aprovação)
// @Description Lista por status (padrão pendente; "todos" lista todas), mais antigas primeiro.
// @Tags        admin
// @Produce     json
// @Param       status query string false "pendente (padrão) | aprovada | rejeitada | todos"
// @Param       pagina query int    false "Página"
// @Param       limite query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.CadastroAdminResponse]}
// @Security    AdminAuth
// @Router      /api/admin/academias [get]
func (ctrl *AdminController) ListarAcademias(c *fiber.Ctx) error {
	f, ok := filtroCadastro(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.Academias(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ListarInstrutores godoc
// @Summary     Instrutores (aprovação)
// @Description Lista por status (padrão pendente; "todos" lista todos), mais antigos primeiro.
// @Tags        admin
// @Produce     json
// @Param       status query string false "pendente (padrão) | aprovada | rejeitada | todos"
// @Param       pagina query int    false "Página"
// @Param       limite query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.CadastroAdminResponse]}
// @Security    AdminAuth
// @Router      /api/admin/instrutores [get]
func (ctrl *AdminController) ListarInstrutores(c *fiber.Ctx) error {
	f, ok := filtroCadastro(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.Instrutores(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

func (ctrl *AdminController) decidir(c *fiber.Ctx, fn func(services.AtorAdmin, uint, bool, string) (*models.CadastroAdminResponse, error), aprovar bool) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	resp, err := fn(ctrl.ator(c), id, aprovar, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	msg := "Cadastro rejeitado"
	if aprovar {
		msg = "Cadastro aprovado"
	}
	return utils.Success(c, fiber.StatusOK, msg, resp)
}

// AprovarAcademia godoc
// @Summary     Aprovar academia
// @Description Só de pendente (409 caso contrário). Notifica a conta. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID da academia (academias.id)"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.CadastroAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/academias/{id}/aprovar [post]
func (ctrl *AdminController) AprovarAcademia(c *fiber.Ctx) error {
	return ctrl.decidir(c, ctrl.svc.DecidirAcademia, true)
}

// RejeitarAcademia godoc
// @Summary     Rejeitar academia
// @Description Só de pendente; o motivo vira motivo_rejeicao e é enviado à conta. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID da academia"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.CadastroAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/academias/{id}/rejeitar [post]
func (ctrl *AdminController) RejeitarAcademia(c *fiber.Ctx) error {
	return ctrl.decidir(c, ctrl.svc.DecidirAcademia, false)
}

// AprovarInstrutor godoc
// @Summary     Aprovar instrutor
// @Description Só de pendente. Notifica a conta. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do instrutor (instrutores.id)"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.CadastroAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/instrutores/{id}/aprovar [post]
func (ctrl *AdminController) AprovarInstrutor(c *fiber.Ctx) error {
	return ctrl.decidir(c, ctrl.svc.DecidirInstrutor, true)
}

// RejeitarInstrutor godoc
// @Summary     Rejeitar instrutor
// @Description Só de pendente; o motivo vira motivo_rejeicao. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do instrutor"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.CadastroAdminResponse}
// @Failure     404 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/instrutores/{id}/rejeitar [post]
func (ctrl *AdminController) RejeitarInstrutor(c *fiber.Ctx) error {
	return ctrl.decidir(c, ctrl.svc.DecidirInstrutor, false)
}

// ─── Receita / reservas / auditoria ──────────────────────────────────────────

// ListarReceitas godoc
// @Summary     Receita da plataforma
// @Description Linhas de plataforma_receitas e os totais por origem no período.
// @Tags        admin
// @Produce     json
// @Param       origem     query string false "taxa_inscricao | tarifa_saque"
// @Param       id_desafio query int    false "Desafio"
// @Param       de         query string false "Início (AAAA-MM-DD)"
// @Param       ate        query string false "Fim inclusive (AAAA-MM-DD)"
// @Param       pagina     query int    false "Página"
// @Param       limite     query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.ReceitasAdminResponse}
// @Security    AdminAuth
// @Router      /api/admin/receitas [get]
func (ctrl *AdminController) ListarReceitas(c *fiber.Ctx) error {
	f := repositories.FiltroReceitasAdmin{Origem: c.Query("origem")}
	switch f.Origem {
	case "", models.ReceitaTaxaInscricao, models.ReceitaTarifaSaque:
	default:
		return utils.ValidationError(c, "Origem inválida")
	}
	var ok bool
	if f.IDDesafio, ok = queryUint(c, "id_desafio"); !ok {
		return nil
	}
	if f.Periodo, ok = periodo(c); !ok {
		return nil
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Receitas(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ListarReservas godoc
// @Summary     Reservas das academias
// @Description Desafios com reserva (garantia da academia): valor vigente e quanto ainda está retido no ledger.
// @Tags        admin
// @Produce     json
// @Param       status      query string false "Status do desafio"
// @Param       id_academia query int    false "Academia"
// @Param       pagina      query int    false "Página"
// @Param       limite      query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.ReservaAdminResponse]}
// @Security    AdminAuth
// @Router      /api/admin/reservas [get]
func (ctrl *AdminController) ListarReservas(c *fiber.Ctx) error {
	f := repositories.FiltroReservasAdmin{Status: models.StatusDesafio(c.Query("status"))}
	var ok bool
	if f.IDAcademia, ok = queryUint(c, "id_academia"); !ok {
		return nil
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Reservas(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ListarAuditoria godoc
// @Summary     Auditoria do painel
// @Description Trilha append-only das ações dos admins (mais recentes primeiro).
// @Tags        admin
// @Produce     json
// @Param       id_admin  query int    false "Admin"
// @Param       acao      query string false "Ação (ex.: saque_reembolsar)"
// @Param       alvo_tipo query string false "administrador | transacao | usuario | desafio | academia | instrutor"
// @Param       alvo_id   query int    false "ID do alvo"
// @Param       de        query string false "Início (AAAA-MM-DD)"
// @Param       ate       query string false "Fim inclusive (AAAA-MM-DD)"
// @Param       pagina    query int    false "Página"
// @Param       limite    query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.AuditoriaAdminResponse]}
// @Security    AdminAuth
// @Router      /api/admin/auditoria [get]
func (ctrl *AdminController) ListarAuditoria(c *fiber.Ctx) error {
	f := repositories.FiltroAuditoria{Acao: c.Query("acao"), AlvoTipo: c.Query("alvo_tipo")}
	if len(f.Acao) > 60 || len(f.AlvoTipo) > 30 {
		return utils.ValidationError(c, "Filtro inválido")
	}
	var ok bool
	if f.IDAdmin, ok = queryUint(c, "id_admin"); !ok {
		return nil
	}
	if f.AlvoID, ok = queryUint(c, "alvo_id"); !ok {
		return nil
	}
	p, ok := periodo(c)
	if !ok {
		return nil
	}
	f.De, f.Ate = p.De, p.Ate
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Auditoria(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// ─── Administradores (superadmin) ────────────────────────────────────────────

// ListarAdmins godoc
// @Summary     Administradores (superadmin)
// @Tags        admin
// @Produce     json
// @Param       papel  query string false "superadmin | admin"
// @Param       ativo  query bool   false "Ativos/inativos"
// @Param       pagina query int    false "Página"
// @Param       limite query int    false "Itens por página"
// @Success     200 {object} utils.APIResponse{data=models.PaginaAdmin[models.AdminResponse]}
// @Failure     403 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/administradores [get]
func (ctrl *AdminController) ListarAdmins(c *fiber.Ctx) error {
	f := repositories.FiltroAdmins{Papel: c.Query("papel")}
	switch f.Papel {
	case "", models.PapelSuperadmin, models.PapelAdmin:
	default:
		return utils.ValidationError(c, "Papel inválido")
	}
	var ok bool
	if f.Ativo, ok = queryBool(c, "ativo"); !ok {
		return nil
	}
	f.Pagina, f.Limite = paginacao(c)
	resp, err := ctrl.svc.Administradores(f)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "ok", resp)
}

// CriarAdmin godoc
// @Summary     Criar administrador (superadmin)
// @Description Cria o admin com senha temporária, devolvida UMA vez em senha_temporaria. 409 para e-mail existente. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       body body models.CriarAdminRequest true "Dados"
// @Success     201 {object} utils.APIResponse{data=models.AdminSenhaTemporariaResponse}
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/administradores [post]
func (ctrl *AdminController) CriarAdmin(c *fiber.Ctx) error {
	var req models.CriarAdminRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.svc.CriarAdmin(ctrl.ator(c), &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Administrador criado", resp)
}

// DesativarAdmin godoc
// @Summary     Desativar administrador (superadmin)
// @Description Desativa e derruba as sessões. 409 para a própria conta ou o último superadmin ativo. Idempotente. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do admin"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.AcaoAdminResponse}
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/administradores/{id}/desativar [post]
func (ctrl *AdminController) DesativarAdmin(c *fiber.Ctx) error {
	return ctrl.acaoBool(c, func(a services.AtorAdmin, id uint, m string) (bool, error) {
		return ctrl.svc.AlterarAtivoAdmin(a, id, false, m)
	}, "Administrador desativado")
}

// ReativarAdmin godoc
// @Summary     Reativar administrador (superadmin)
// @Description Idempotente. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do admin"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.AcaoAdminResponse}
// @Failure     403 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/administradores/{id}/reativar [post]
func (ctrl *AdminController) ReativarAdmin(c *fiber.Ctx) error {
	return ctrl.acaoBool(c, func(a services.AtorAdmin, id uint, m string) (bool, error) {
		return ctrl.svc.AlterarAtivoAdmin(a, id, true, m)
	}, "Administrador reativado")
}

// TrocarPapelAdmin godoc
// @Summary     Trocar papel de administrador (superadmin)
// @Description superadmin <-> admin; derruba as sessões do alvo. 409 ao rebaixar o último superadmin ativo. Idempotente. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                            true "ID do admin"
// @Param       body body models.TrocarPapelAdminRequest true "Papel e motivo"
// @Success     200 {object} utils.APIResponse{data=models.AcaoAdminResponse}
// @Failure     403 {object} utils.APIResponse
// @Failure     409 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/administradores/{id}/papel [post]
func (ctrl *AdminController) TrocarPapelAdmin(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.TrocarPapelAdminRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	alterado, err := ctrl.svc.TrocarPapelAdmin(ctrl.ator(c), id, req.Papel, req.Motivo)
	if err != nil {
		return responderErro(c, err)
	}
	msg := "Papel alterado"
	if !alterado {
		msg = "Nada a fazer: já estava nesse papel"
	}
	return utils.Success(c, fiber.StatusOK, msg, models.AcaoAdminResponse{Alterado: alterado})
}

// ResetarSenhaAdmin godoc
// @Summary     Resetar senha de administrador (superadmin)
// @Description Gera nova senha temporária (devolvida UMA vez) e derruba as sessões do alvo. Auditado.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "ID do admin"
// @Param       body body models.MotivoRequest true "Motivo"
// @Success     200 {object} utils.APIResponse{data=models.AdminSenhaTemporariaResponse}
// @Failure     403 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Security    AdminAuth
// @Router      /api/admin/administradores/{id}/resetar-senha [post]
func (ctrl *AdminController) ResetarSenhaAdmin(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	motivo, ok := lerMotivo(c)
	if !ok {
		return nil
	}
	resp, err := ctrl.svc.ResetarSenhaAdmin(ctrl.ator(c), id, motivo)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Senha redefinida", resp)
}
