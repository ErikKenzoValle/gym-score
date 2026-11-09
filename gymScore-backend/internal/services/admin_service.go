package services

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SenhaAdminMinima é o tamanho mínimo da senha de administrador (CLI, troca de senha e senha temporária).
const SenhaAdminMinima = 12

// tamanhoSenhaTemporaria é o tamanho das senhas temporárias geradas pelo painel.
const tamanhoSenhaTemporaria = 16

// AtorAdmin identifica quem executa uma ação no painel (vem do AdminAuthMiddleware, nunca do corpo).
type AtorAdmin struct {
	ID    uint
	Papel string // papel vigente no banco
	IP    string
}

func (a AtorAdmin) superadmin() bool { return a.Papel == models.PapelSuperadmin }

// Ações gravadas em admin_auditoria.acao.
const (
	AcaoLogin              = "login"
	AcaoLoginFalhou        = "login_falhou"
	AcaoLogout             = "logout"
	AcaoTrocarSenha        = "trocar_senha"
	AcaoSaqueReconciliar   = "saque_reconciliar"
	AcaoSaqueReembolsar    = "saque_reembolsar"
	AcaoSaqueMarcarPago    = "saque_marcar_pago"
	AcaoUsuarioBloquear    = "usuario_bloquear"
	AcaoUsuarioDesbloquear = "usuario_desbloquear"
	AcaoDesafioCancelar    = "desafio_cancelar"
	AcaoAcademiaAprovar    = "academia_aprovar"
	AcaoAcademiaRejeitar   = "academia_rejeitar"
	AcaoInstrutorAprovar   = "instrutor_aprovar"
	AcaoInstrutorRejeitar  = "instrutor_rejeitar"
	AcaoAdminCriar         = "admin_criar"
	AcaoAdminDesativar     = "admin_desativar"
	AcaoAdminReativar      = "admin_reativar"
	AcaoAdminTrocarPapel   = "admin_trocar_papel"
	AcaoAdminResetarSenha  = "admin_resetar_senha"
	AcaoCLICriarSuperadmin = "cli_criar_superadmin"
	AcaoCLIResetarSenha    = "cli_resetar_senha"
	AcaoCLISeed            = "cli_seed" // contas de teste criadas/atualizadas por `seed-teste` (nunca em produção)
)

// Tipos de alvo gravados em admin_auditoria.alvo_tipo.
const (
	AlvoAdmin     = "administrador"
	AlvoTransacao = "transacao"
	AlvoUsuario   = "usuario"
	AlvoDesafio   = "desafio"
	AlvoAcademia  = "academia"
	AlvoInstrutor = "instrutor"
)

// ErroSuperadmin é devolvido quando um admin comum tenta uma ação restrita.
var ErroSuperadmin = erro(http.StatusForbidden, "Ação restrita a superadministradores")

// erroAsaas: o Asaas não respondeu à conferência obrigatória antes de mexer no saque.
var erroAsaas = erro(http.StatusBadGateway, "Não foi possível consultar o Asaas agora. Tente novamente em instantes")

// AdminConfig são os parâmetros do painel vindos da configuração.
type AdminConfig struct {
	Segredo  string        // ADMIN_JWT_SECRET (ou o padrão de dev)
	TTL      time.Duration // ADMIN_TOKEN_TTL
	Carencia time.Duration // DESAFIO_CARENCIA (para marcar desafios vencidos)
}

// AdminService concentra o painel admin: autenticação, consultas e ações auditadas. Toda ação de
// escrita grava admin_auditoria na MESMA transação da mudança, com motivo obrigatório. Ações que
// movem dinheiro reaproveitam os métodos atômicos/idempotentes da carteira e do desafio.
type AdminService interface {
	// Autenticação
	Login(email, senha, ip string) (*models.AdminResponse, string, error)
	Logout(ator AtorAdmin) error
	Me(id uint) (*models.AdminResponse, error)
	TrocarSenha(ator AtorAdmin, atual, nova string) (string, error)

	// Consultas
	Dashboard(de, ate *time.Time) (*models.DashboardAdminResponse, error)
	Transacoes(f repositories.FiltroTransacoesAdmin) (*models.PaginaAdmin[models.TransacaoAdminResponse], error)
	Transacao(id uint) (*models.TransacaoDetalheAdminResponse, error)
	Saques(status string, emRevisao *bool, pagina, limite int) (*models.PaginaAdmin[models.SaqueAdminResponse], error)
	Usuarios(f repositories.FiltroUsuariosAdmin) (*models.PaginaAdmin[models.UsuarioAdminResponse], error)
	Usuario(id uint, pagina, limite int) (*models.UsuarioDetalheAdminResponse, error)
	Desafios(f repositories.FiltroDesafiosAdmin) (*models.PaginaAdmin[models.DesafioResumoAdmin], error)
	Desafio(id uint) (*models.DesafioDetalheAdminResponse, error)
	Academias(f repositories.FiltroCadastrosAdmin) (*models.PaginaAdmin[models.CadastroAdminResponse], error)
	Instrutores(f repositories.FiltroCadastrosAdmin) (*models.PaginaAdmin[models.CadastroAdminResponse], error)
	Receitas(f repositories.FiltroReceitasAdmin) (*models.ReceitasAdminResponse, error)
	Reservas(f repositories.FiltroReservasAdmin) (*models.PaginaAdmin[models.ReservaAdminResponse], error)
	Auditoria(f repositories.FiltroAuditoria) (*models.PaginaAdmin[models.AuditoriaAdminResponse], error)
	Administradores(f repositories.FiltroAdmins) (*models.PaginaAdmin[models.AdminResponse], error)

	// Ações (todas com motivo e auditoria na mesma transação)
	ReconciliarSaque(ator AtorAdmin, id uint, motivo string) (*models.SaqueAcaoResponse, error)
	ReembolsarSaque(ator AtorAdmin, id uint, motivo string) (*models.SaqueAcaoResponse, error)
	MarcarSaquePago(ator AtorAdmin, id uint, motivo string) (*models.SaqueAcaoResponse, error)
	CancelarDesafio(ator AtorAdmin, id uint, motivo string) (*models.DesafioDetalheAdminResponse, error)
	DecidirAcademia(ator AtorAdmin, id uint, aprovar bool, motivo string) (*models.CadastroAdminResponse, error)
	DecidirInstrutor(ator AtorAdmin, id uint, aprovar bool, motivo string) (*models.CadastroAdminResponse, error)
	CriarAdmin(ator AtorAdmin, req *models.CriarAdminRequest) (*models.AdminSenhaTemporariaResponse, error)
	AlterarAtivoAdmin(ator AtorAdmin, id uint, ativo bool, motivo string) (bool, error)
	TrocarPapelAdmin(ator AtorAdmin, id uint, papel, motivo string) (bool, error)
	ResetarSenhaAdmin(ator AtorAdmin, id uint, motivo string) (*models.AdminSenhaTemporariaResponse, error)
}

type adminService struct {
	db         *gorm.DB
	admins     repositories.AdminRepository
	auditoria  repositories.AuditoriaRepository
	moderacao  repositories.ModeracaoRepository
	consultas  repositories.AdminConsultaRepository
	carteira   *repositories.CarteiraRepository
	desafios   *repositories.DesafioRepository
	desafioSvc DesafioService
	asaas      AsaasGateway
	notif      Notificador
	cfg        AdminConfig
	agora      func() time.Time
}

// NovoAdminService cria o serviço do painel admin.
func NovoAdminService(
	db *gorm.DB,
	admins repositories.AdminRepository,
	auditoria repositories.AuditoriaRepository,
	moderacao repositories.ModeracaoRepository,
	consultas repositories.AdminConsultaRepository,
	carteira *repositories.CarteiraRepository,
	desafios *repositories.DesafioRepository,
	desafioSvc DesafioService,
	asaas AsaasGateway,
	notif Notificador,
	cfg AdminConfig,
) AdminService {
	if notif == nil {
		notif = NotificadorNulo{}
	}
	return &adminService{db: db, admins: admins, auditoria: auditoria, moderacao: moderacao, consultas: consultas,
		carteira: carteira, desafios: desafios, desafioSvc: desafioSvc, asaas: asaas, notif: notif, cfg: cfg, agora: time.Now}
}

// ─── Utilitários ─────────────────────────────────────────────────────────────

// RegistrarAuditoria monta e grava a linha de admin_auditoria na transação `tx` (exportado para a CLI).
func RegistrarAuditoria(repo repositories.AuditoriaRepository, tx *gorm.DB, idAdmin uint, ip, acao, alvoTipo string,
	alvoID uint, motivo string, detalhes any) error {
	a := &models.AdminAuditoria{IDAdmin: idAdmin, Acao: acao, AlvoTipo: alvoTipo, Motivo: limitar(motivo, 500), IP: limitar(ip, 45)}
	if alvoID != 0 {
		id := alvoID
		a.AlvoID = &id
	}
	if detalhes != nil {
		b, err := json.Marshal(detalhes)
		if err != nil {
			return err
		}
		s := string(b)
		a.Detalhes = &s
	}
	return repo.Registrar(tx, a)
}

func (s *adminService) auditar(tx *gorm.DB, ator AtorAdmin, acao, alvoTipo string, alvoID uint, motivo string, detalhes any) error {
	return RegistrarAuditoria(s.auditoria, tx, ator.ID, ator.IP, acao, alvoTipo, alvoID, motivo, detalhes)
}

// limitar corta a string em n bytes sem quebrar um caractere UTF-8 no meio.
func limitar(v string, n int) string {
	if len(v) <= n {
		return v
	}
	for n > 0 && !utf8RuneStart(v[n]) {
		n--
	}
	return v[:n]
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

// motivoValido normaliza o motivo (obrigatório, 3..500 caracteres).
func motivoValido(m string) (string, error) {
	m = strings.TrimSpace(m)
	if len([]rune(m)) < 3 {
		return "", erroRequisicao("Informe o motivo da ação (mínimo 3 caracteres)")
	}
	if len([]rune(m)) > 500 {
		return "", erroRequisicao("O motivo deve ter no máximo 500 caracteres")
	}
	return m, nil
}

// GerarSenhaTemporaria gera uma senha aleatória (crypto/rand) sem caracteres ambíguos.
func GerarSenhaTemporaria() (string, error) {
	const alfabeto = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	var b strings.Builder
	max := big.NewInt(int64(len(alfabeto)))
	for i := 0; i < tamanhoSenhaTemporaria; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(alfabeto[n.Int64()])
	}
	return b.String(), nil
}

// MascararCPF mantém só os 6 dígitos do meio ("***.456.789-**"). Vazio se não houver CPF.
func MascararCPF(cpf *string) string {
	if cpf == nil || *cpf == "" {
		return ""
	}
	var dig []rune
	for _, r := range *cpf {
		if r >= '0' && r <= '9' {
			dig = append(dig, r)
		}
	}
	if len(dig) != 11 {
		return "***"
	}
	return "***." + string(dig[3:6]) + "." + string(dig[6:9]) + "-**"
}

func paginaDe[T any](itens []T, total int64, pagina, limite int) *models.PaginaAdmin[T] {
	pagina, limite = normalizarPaginaAdmin(pagina, limite)
	if itens == nil {
		itens = []T{}
	}
	return &models.PaginaAdmin[T]{Itens: itens, Total: total, Pagina: pagina, Limite: limite}
}

// normalizarPaginaAdmin espelha a regra do repositório (página >= 1, limite 1..100, padrão 20).
func normalizarPaginaAdmin(pagina, limite int) (int, int) {
	if pagina < 1 {
		pagina = 1
	}
	if limite < 1 {
		limite = 20
	}
	if limite > 100 {
		limite = 100
	}
	return pagina, limite
}

// ─── Autenticação ────────────────────────────────────────────────────────────

// erroLoginAdmin é a mensagem única do login (não revela se o e-mail existe nem se está desativado).
var erroLoginAdmin = erro(http.StatusUnauthorized, "E-mail ou senha inválidos")

func (s *adminService) Login(email, senha, ip string) (*models.AdminResponse, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	a, err := s.admins.BuscarPorEmail(email)
	if err != nil {
		return nil, "", err
	}
	if a == nil {
		_ = bcrypt.CompareHashAndPassword(hashInexistente, []byte(senha)) // tempo constante
		return nil, "", erroLoginAdmin
	}
	ator := AtorAdmin{ID: a.ID, Papel: a.Papel, IP: ip}
	if bcrypt.CompareHashAndPassword([]byte(a.SenhaHash), []byte(senha)) != nil || !a.Ativo {
		motivo := "Senha incorreta"
		if !a.Ativo {
			motivo = "Conta desativada"
		}
		if err := s.auditar(nil, ator, AcaoLoginFalhou, AlvoAdmin, a.ID, motivo, nil); err != nil {
			log.Printf("[ADMIN] falha ao auditar login recusado do admin %d: %v", a.ID, err)
		}
		return nil, "", erroLoginAdmin
	}
	agora := s.agora()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.admins.RegistrarLogin(tx, a.ID, agora); err != nil {
			return err
		}
		return s.auditar(tx, ator, AcaoLogin, AlvoAdmin, a.ID, "Login no painel", nil)
	})
	if err != nil {
		return nil, "", err
	}
	token, err := utils.GerarTokenAdmin(a.ID, a.TokenVersao, s.cfg.Segredo, s.cfg.TTL)
	if err != nil {
		return nil, "", err
	}
	a.UltimoLoginEm = &agora
	r := models.NovoAdminResponse(a)
	return &r, token, nil
}

func (s *adminService) Logout(ator AtorAdmin) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.admins.RevogarTokens(tx, ator.ID); err != nil {
			return err
		}
		return s.auditar(tx, ator, AcaoLogout, AlvoAdmin, ator.ID, "Logout do painel", nil)
	})
}

func (s *adminService) Me(id uint) (*models.AdminResponse, error) {
	a, err := s.admins.BuscarPorID(id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, erroNaoEncontrado("Administrador não encontrado")
	}
	r := models.NovoAdminResponse(a)
	return &r, nil
}

// TrocarSenha troca a senha do próprio admin (revoga os tokens) e devolve um token novo.
func (s *adminService) TrocarSenha(ator AtorAdmin, atual, nova string) (string, error) {
	if len(nova) < SenhaAdminMinima {
		return "", erroRequisicao(fmt.Sprintf("A nova senha deve ter no mínimo %d caracteres", SenhaAdminMinima))
	}
	a, err := s.admins.BuscarPorID(ator.ID)
	if err != nil {
		return "", err
	}
	if a == nil || bcrypt.CompareHashAndPassword([]byte(a.SenhaHash), []byte(atual)) != nil {
		return "", erroRequisicao("Senha atual incorreta")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(nova), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	var versao int
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if _, err := s.admins.AlterarSenha(tx, ator.ID, string(hash)); err != nil {
			return err
		}
		atualizado, err := s.admins.TravarPorID(tx, ator.ID)
		if err != nil || atualizado == nil {
			return errors.Join(err, errors.New("admin sumiu durante a troca de senha"))
		}
		versao = atualizado.TokenVersao
		return s.auditar(tx, ator, AcaoTrocarSenha, AlvoAdmin, ator.ID, "Troca da própria senha", nil)
	})
	if err != nil {
		return "", err
	}
	return utils.GerarTokenAdmin(ator.ID, versao, s.cfg.Segredo, s.cfg.TTL)
}

// ─── Consultas ───────────────────────────────────────────────────────────────

func (s *adminService) Dashboard(de, ate *time.Time) (*models.DashboardAdminResponse, error) {
	r, err := s.consultas.ResumoFinanceiro()
	if err != nil {
		return nil, err
	}
	receitas, err := s.consultas.ReceitaPorOrigem(repositories.Periodo{De: de, Ate: ate})
	if err != nil {
		return nil, err
	}
	var totalReceita int64
	for _, v := range receitas {
		totalReceita += v
	}
	sim := true
	_, pendentes, err := s.consultas.ListarSaques(models.TransacaoPending, nil, 1, 1)
	if err != nil {
		return nil, err
	}
	_, emRevisao, err := s.consultas.ListarSaques(models.TransacaoPending, &sim, 1, 1)
	if err != nil {
		return nil, err
	}
	limite := s.agora().Add(-s.cfg.Carencia)
	_, vencidos, err := s.consultas.ListarDesafios(repositories.FiltroDesafiosAdmin{Vencidos: true,
		LimiteAberto: limite, LimiteAndamento: limite, Pagina: 1, Limite: 1})
	if err != nil {
		return nil, err
	}
	_, acadPend, err := s.consultas.ListarAcademias(repositories.FiltroCadastrosAdmin{Status: models.CadastroPendente, Pagina: 1, Limite: 1})
	if err != nil {
		return nil, err
	}
	_, instPend, err := s.consultas.ListarInstrutores(repositories.FiltroCadastrosAdmin{Status: models.CadastroPendente, Pagina: 1, Limite: 1})
	if err != nil {
		return nil, err
	}
	porStatus, err := s.consultas.ContarDesafiosPorStatus()
	if err != nil {
		return nil, err
	}
	mod, err := repositories.ContarPendenciasModeracao(s.db)
	if err != nil {
		return nil, err
	}
	custodia := r.CustodiaEntradas + r.CustodiaReservas
	aplicacoes := r.SomaSaldos + custodia + r.Receitas + r.PendentesUsuarios + r.PendentesPlataforma - r.DebitosAbertos
	origens := r.Depositos - r.Saques + r.Ajustes
	return &models.DashboardAdminResponse{
		Financeiro: models.FinanceiroDashboardAdmin{
			SaldoCarteirasCentavos: r.SomaSaldos, SomaLedgerCentavos: r.SomaLedger, DepositosCentavos: r.Depositos,
			SaquesCentavos: r.Saques, AjustesCentavos: r.Ajustes, CustodiaEntradasCentavos: r.CustodiaEntradas,
			CustodiaReservasCentavos: r.CustodiaReservas, CustodiaTotalCentavos: custodia, ReceitasCentavos: r.Receitas,
			PendentesUsuariosCentavos: r.PendentesUsuarios, PendentesPlataformaCentavos: r.PendentesPlataforma,
			DebitosAbertosCentavos: r.DebitosAbertos,
		},
		Conferencia: models.ConferenciaDashboardAdmin{
			LadoAplicacoesCentavos: aplicacoes, LadoOrigensCentavos: origens, DivergenciaCentavos: aplicacoes - origens,
			Confere: aplicacoes == origens, SaldosVsLedgerCentavos: r.SomaSaldos - r.SomaLedger, SaldosConferem: r.SomaSaldos == r.SomaLedger,
		},
		Receita: models.ReceitaDashboardAdmin{De: de, Ate: ate, PorOrigem: receitas, TotalCentavos: totalReceita},
		Pendencias: models.PendenciasDashboardAdmin{SaquesPendentes: pendentes, SaquesEmRevisao: emRevisao,
			DesafiosVencidos: vencidos, AcademiasPendentes: acadPend, InstrutoresPendentes: instPend,
			DenunciasAdmin: mod.DenunciasAdmin, DenunciasEscalonadas: mod.DenunciasEscalonadas,
			DenunciasAcademia: mod.DenunciasAcademia, RecursosPendentes: mod.RecursosPendentes,
			PagamentosRetidos: mod.PagamentosRetidos, DebitosAbertos: mod.DebitosAbertos},
		DesafiosPorStatus: porStatus,
	}, nil
}

func transacoesAdmin(ts []models.Transacao) []models.TransacaoAdminResponse {
	out := make([]models.TransacaoAdminResponse, 0, len(ts))
	for i := range ts {
		out = append(out, models.NovaTransacaoAdmin(&ts[i]))
	}
	return out
}

func (s *adminService) Transacoes(f repositories.FiltroTransacoesAdmin) (*models.PaginaAdmin[models.TransacaoAdminResponse], error) {
	ts, total, err := s.consultas.ListarTransacoes(f)
	if err != nil {
		return nil, err
	}
	return paginaDe(transacoesAdmin(ts), total, f.Pagina, f.Limite), nil
}

func (s *adminService) Transacao(id uint) (*models.TransacaoDetalheAdminResponse, error) {
	t, err := s.consultas.BuscarTransacao(id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, erroNaoEncontrado("Transação não encontrada")
	}
	resp := &models.TransacaoDetalheAdminResponse{Transacao: models.NovaTransacaoAdmin(t)}
	if us, err := s.consultas.UsuariosResumo([]uint{t.IDUsuario}); err != nil {
		return nil, err
	} else if u, ok := us[t.IDUsuario]; ok {
		resp.Usuario = models.NovoUsuarioResumoAdmin(&u)
	}
	if t.IDTransacaoOrigem != nil {
		o, err := s.consultas.BuscarTransacao(*t.IDTransacaoOrigem)
		if err != nil {
			return nil, err
		}
		if o != nil {
			or := models.NovaTransacaoAdmin(o)
			resp.Origem = &or
		}
	}
	derivadas, err := s.consultas.TransacoesDerivadas(t.ID)
	if err != nil {
		return nil, err
	}
	resp.Derivadas = transacoesAdmin(derivadas)
	return resp, nil
}

// saquesAdmin enriquece os saques com tarifa e usuário (duas consultas em lote).
func (s *adminService) saquesAdmin(ts []models.Transacao) ([]models.SaqueAdminResponse, error) {
	ids := make([]uint, 0, len(ts))
	usuarios := make([]uint, 0, len(ts))
	for _, t := range ts {
		ids = append(ids, t.ID)
		usuarios = append(usuarios, t.IDUsuario)
	}
	tarifas, err := s.consultas.TarifasDosSaques(ids)
	if err != nil {
		return nil, err
	}
	resumos, err := s.consultas.UsuariosResumo(usuarios)
	if err != nil {
		return nil, err
	}
	out := make([]models.SaqueAdminResponse, 0, len(ts))
	for i := range ts {
		t := &ts[i]
		r := models.SaqueAdminResponse{TransacaoAdminResponse: models.NovaTransacaoAdmin(t),
			ValorSaqueCentavos: -t.ValorCentavos, TarifaCentavos: tarifas[t.ID], ReferenciaAsaas: ReferenciaSaque(t.ID)}
		if u, ok := resumos[t.IDUsuario]; ok {
			r.Usuario = models.NovoUsuarioResumoAdmin(&u)
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *adminService) Saques(status string, emRevisao *bool, pagina, limite int) (*models.PaginaAdmin[models.SaqueAdminResponse], error) {
	ts, total, err := s.consultas.ListarSaques(status, emRevisao, pagina, limite)
	if err != nil {
		return nil, err
	}
	itens, err := s.saquesAdmin(ts)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, pagina, limite), nil
}

func usuarioAdmin(u *models.Usuario) models.UsuarioAdminResponse {
	return models.UsuarioAdminResponse{ID: u.ID, Nome: u.Nome, Sobrenome: u.Sobrenome, Email: u.Email,
		CPFMascarado: MascararCPF(u.CPF), Perfil: u.Perfil, Username: u.Username, SaldoCentavos: u.SaldoCentavos,
		Bloqueado: u.Banido(), BloqueadoMotivo: u.MotivoBloqueio, BloqueadoEm: u.BanidoEm, UltimaVez: u.UltimaVez,
		BanidoEm: u.BanidoEm, SuspensoAte: u.SuspensoAte, Suspenso: u.Suspenso(time.Now()), MotivoBloqueio: u.MotivoBloqueio,
		CriadoEm: u.CriadoEm}
}

func (s *adminService) Usuarios(f repositories.FiltroUsuariosAdmin) (*models.PaginaAdmin[models.UsuarioAdminResponse], error) {
	f.Q = strings.TrimSpace(f.Q)
	us, total, err := s.consultas.ListarUsuarios(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.UsuarioAdminResponse, 0, len(us))
	for i := range us {
		itens = append(itens, usuarioAdmin(&us[i]))
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *adminService) Usuario(id uint, pagina, limite int) (*models.UsuarioDetalheAdminResponse, error) {
	u, err := s.consultas.BuscarUsuario(id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, erroNaoEncontrado("Usuário não encontrado")
	}
	soma, err := s.carteira.SomaLedger(id)
	if err != nil {
		return nil, err
	}
	ledger, err := s.Transacoes(repositories.FiltroTransacoesAdmin{IDUsuario: id, Pagina: pagina, Limite: limite})
	if err != nil {
		return nil, err
	}
	desafios, err := s.consultas.ParticipacoesUsuario(id)
	if err != nil {
		return nil, err
	}
	resp := &models.UsuarioDetalheAdminResponse{Usuario: usuarioAdmin(u), SomaLedgerCentavos: soma,
		LedgerConfere: soma == u.SaldoCentavos, Ledger: *ledger, Desafios: []models.ParticipacaoAdminResponse{}}
	for _, d := range desafios {
		p := models.ParticipacaoAdminResponse{IDDesafio: d.ID, Titulo: d.Titulo, Status: d.Status, DataEvento: d.DataEvento}
		if len(d.Participantes) > 0 {
			p.Presente, p.PremioCentavos, p.Vencedor = d.Participantes[0].Presente, d.Participantes[0].PremioCentavos, d.Participantes[0].Vencedor
		}
		resp.Desafios = append(resp.Desafios, p)
	}
	switch u.Perfil {
	case "academia":
		var ids []uint
		if err := s.db.Model(&models.Academia{}).Where("id_usuario = ?", id).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			if a, err := s.consultas.BuscarAcademia(ids[0]); err == nil && a != nil {
				c := models.NovoCadastroAcademia(a)
				resp.Academia = &c
			}
		}
	case models.PerfilAtleta: // migration 00020: o pedido de instrutor é da conta de atleta
		var ids []uint
		if err := s.db.Model(&models.Instrutor{}).Where("id_usuario = ?", id).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			if i, err := s.consultas.BuscarInstrutor(ids[0]); err == nil && i != nil {
				c := models.NovoCadastroInstrutor(i)
				resp.Instrutor = &c
			}
		}
	}
	return resp, nil
}

func (s *adminService) desafioResumo(d *models.Desafio) models.DesafioResumoAdmin {
	r := models.DesafioResumoAdmin{ID: d.ID, Titulo: d.Titulo, Status: d.Status, Categoria: d.Categoria,
		DataEvento: d.DataEvento, ValorEntradaCentavos: d.ValorEntradaCentavos, Vagas: d.Vagas, IDCriador: d.IDCriador,
		IDAcademia: d.IDAcademia, IDInstrutor: d.IDInstrutor, ReservaAcademiaCentavos: d.ReservaAcademiaCentavos,
		Vencido: vencido(d, s.agora(), s.cfg.Carencia), CriadoEm: d.CriadoEm,
		PodioLancadoEm: d.PodioLancadoEm, ResultadoAnuladoEm: d.ResultadoAnuladoEm, ResultadoAnulacaoModo: d.ResultadoAnulacaoModo,
		PodeAnularResultado: d.Status == models.StatusEncerrado && d.PodioLancadoEm != nil && d.ResultadoAnuladoEm == nil}
	if d.Academia != nil {
		r.NomeAcademia = d.Academia.NomeFantasia
	}
	return r
}

func (s *adminService) Desafios(f repositories.FiltroDesafiosAdmin) (*models.PaginaAdmin[models.DesafioResumoAdmin], error) {
	if f.Vencidos {
		limite := s.agora().Add(-s.cfg.Carencia)
		f.LimiteAberto, f.LimiteAndamento = limite, limite
	}
	ds, total, err := s.consultas.ListarDesafios(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.DesafioResumoAdmin, 0, len(ds))
	for i := range ds {
		itens = append(itens, s.desafioResumo(&ds[i]))
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *adminService) Desafio(id uint) (*models.DesafioDetalheAdminResponse, error) {
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	totais, err := s.consultas.TotaisLedgerDesafio(id)
	if err != nil {
		return nil, err
	}
	receitas, _, err := s.consultas.ListarReceitas(repositories.FiltroReceitasAdmin{IDDesafio: id, Limite: 100})
	if err != nil {
		return nil, err
	}
	var receita int64
	for _, r := range receitas {
		receita += r.ValorCentavos
	}
	resp := &models.DesafioDetalheAdminResponse{
		Desafio: s.desafioResumo(d),
		Financeiro: models.FinanceiroDesafioAdmin{PremioFixoCentavos: d.PremioFixoCentavos, DividirPote: d.DividirPote,
			PercentualPote: d.PercentualPote, TaxaPlataformaPct: d.TaxaPlataformaPct, InstrutorModoPagamento: d.InstrutorModoPagamento,
			InstrutorFixoCentavos: d.InstrutorFixoCentavos, InstrutorPct: d.InstrutorPct, InstrutorSelecao: d.InstrutorSelecao,
			InstrutorTetoCentavos: d.InstrutorTetoCentavos, TotaisPorTipo: totais, ReceitaPlataformaCentavos: receita},
		Participantes: []models.ParticipanteAdminResponse{}, Candidaturas: []models.CandidaturaAdminResponse{},
		Podio: []models.PodioResponse{},
	}
	for _, p := range d.Participantes {
		pa := models.ParticipanteAdminResponse{IDUsuario: p.IDUsuario, Presente: p.Presente, PresenteEm: p.PresenteEm,
			PresencaModo: p.PresencaModo, PresencaMarcadaPor: p.PresencaMarcadaPor, PremioCentavos: p.PremioCentavos,
			Vencedor: p.Vencedor, InscritoEm: p.CriadoEm}
		if p.Usuario != nil {
			pa.Nome, pa.Sobrenome, pa.Email, pa.Perfil = p.Usuario.Nome, p.Usuario.Sobrenome, p.Usuario.Email, p.Usuario.Perfil
		}
		resp.Participantes = append(resp.Participantes, pa)
	}
	cands, _, err := s.consultas.ListarCandidaturas(repositories.FiltroCandidaturasAdmin{IDDesafio: id, Limite: 100})
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		ca := models.CandidaturaAdminResponse{ID: c.ID, IDInstrutor: c.IDInstrutor, LanceCentavos: c.LanceCentavos,
			Status: c.Status, Mensagem: c.Mensagem, CriadoEm: c.CriadoEm}
		if c.Instrutor != nil && c.Instrutor.Usuario != nil {
			ca.NomeInstrutor = strings.TrimSpace(c.Instrutor.Usuario.Nome + " " + c.Instrutor.Usuario.Sobrenome)
		}
		resp.Candidaturas = append(resp.Candidaturas, ca)
	}
	podio, err := s.desafios.ListarPodio(id)
	if err != nil {
		return nil, err
	}
	for _, p := range podio {
		pr := models.PodioResponse{Posicao: p.Posicao, IDParticipante: p.IDParticipante, Marca: p.Marca,
			PremioCentavos: p.PremioCentavos, Premio: utils.CentavosParaReais(p.PremioCentavos)}
		if p.Participante != nil {
			pr.Nome, pr.Sobrenome = p.Participante.Nome, p.Participante.Sobrenome
		}
		resp.Podio = append(resp.Podio, pr)
	}
	ledger, total, err := s.consultas.ListarTransacoes(repositories.FiltroTransacoesAdmin{IDDesafio: id, Limite: 100})
	if err != nil {
		return nil, err
	}
	resp.Ledger, resp.LedgerTotal = transacoesAdmin(ledger), total
	return resp, nil
}

func (s *adminService) Academias(f repositories.FiltroCadastrosAdmin) (*models.PaginaAdmin[models.CadastroAdminResponse], error) {
	as, total, err := s.consultas.ListarAcademias(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.CadastroAdminResponse, 0, len(as))
	for i := range as {
		itens = append(itens, models.NovoCadastroAcademia(&as[i]))
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *adminService) Instrutores(f repositories.FiltroCadastrosAdmin) (*models.PaginaAdmin[models.CadastroAdminResponse], error) {
	is, total, err := s.consultas.ListarInstrutores(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.CadastroAdminResponse, 0, len(is))
	for i := range is {
		itens = append(itens, models.NovoCadastroInstrutor(&is[i]))
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *adminService) Receitas(f repositories.FiltroReceitasAdmin) (*models.ReceitasAdminResponse, error) {
	rs, total, err := s.consultas.ListarReceitas(f)
	if err != nil {
		return nil, err
	}
	totais, err := s.consultas.ReceitaPorOrigem(f.Periodo)
	if err != nil {
		return nil, err
	}
	itens := make([]models.ReceitaAdminResponse, 0, len(rs))
	for _, r := range rs {
		itens = append(itens, models.ReceitaAdminResponse{ID: r.ID, Origem: r.Origem, ValorCentavos: r.ValorCentavos,
			IDDesafio: r.IDDesafio, IDTransacao: r.IDTransacao, CriadoEm: r.CriadoEm})
	}
	var soma int64
	for _, v := range totais {
		soma += v
	}
	return &models.ReceitasAdminResponse{PaginaAdmin: *paginaDe(itens, total, f.Pagina, f.Limite),
		TotaisPorOrigem: totais, TotalCentavos: soma}, nil
}

func (s *adminService) Reservas(f repositories.FiltroReservasAdmin) (*models.PaginaAdmin[models.ReservaAdminResponse], error) {
	rs, total, err := s.consultas.ListarReservas(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.ReservaAdminResponse, 0, len(rs))
	for _, r := range rs {
		itens = append(itens, models.ReservaAdminResponse{IDDesafio: r.IDDesafio, Titulo: r.Titulo, Status: r.Status,
			IDAcademia: r.IDAcademia, IDCriador: r.IDCriador, ReservaCentavos: r.ReservaCentavos, RetidoCentavos: r.RetidoCentavos})
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *adminService) Auditoria(f repositories.FiltroAuditoria) (*models.PaginaAdmin[models.AuditoriaAdminResponse], error) {
	ls, total, err := s.auditoria.Listar(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.AuditoriaAdminResponse, 0, len(ls))
	for _, l := range ls {
		r := models.AuditoriaAdminResponse{ID: l.ID, IDAdmin: l.IDAdmin, Acao: l.Acao, AlvoTipo: l.AlvoTipo, AlvoID: l.AlvoID,
			Motivo: l.Motivo, IP: l.IP, CriadoEm: l.CriadoEm}
		if l.Detalhes != nil {
			r.Detalhes = json.RawMessage(*l.Detalhes)
		}
		if l.Admin != nil {
			r.AdminNome, r.AdminEmail = l.Admin.Nome, l.Admin.Email
		}
		itens = append(itens, r)
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *adminService) Administradores(f repositories.FiltroAdmins) (*models.PaginaAdmin[models.AdminResponse], error) {
	as, total, err := s.admins.Listar(f)
	if err != nil {
		return nil, err
	}
	itens := make([]models.AdminResponse, 0, len(as))
	for i := range as {
		itens = append(itens, models.NovoAdminResponse(&as[i]))
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}
