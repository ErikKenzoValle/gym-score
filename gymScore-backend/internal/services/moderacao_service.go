package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Moderação (Sprint 7 reconstruída — analise/moderacao-spec.md). Parte 1: denúncias com evidências,
// sigilo, jurisdição academia × admin, prazo/escalonamento, punições (advertência, suspensão,
// banimento, bloqueio_academia), recursos (inclusive do banido — Q1), linha do tempo e auditoria.
// A anulação de resultado e o financeiro do pódio (janela, liberação, débitos) são a parte 2.
//
// Regras transversais:
//   - toda transição de estado é UPDATE condicional (CAS) no repositório; perdeu a corrida = 409;
//   - moderacao_eventos (e admin_auditoria, nas ações do admin) são gravados na MESMA transação;
//   - o estado da conta (banido_em/suspenso_ate) é sempre RECALCULADO das punições vigentes;
//   - notificações só depois do commit;
//   - o denunciante só aparece para o admin.

// Ações gravadas em moderacao_eventos.acao.
const (
	EventoDenunciaCriada      = "denuncia_criada"
	EventoEvidenciaEnviada    = "evidencia_enviada"
	EventoDenunciaAssumida    = "denuncia_assumida"
	EventoDenunciaEncaminhada = "denuncia_encaminhada"
	EventoDenunciaEscalonada  = "denuncia_escalonada"
	EventoDenunciaDecidida    = "denuncia_decidida"
	EventoPunicaoAplicada     = "punicao_aplicada"
	EventoPunicaoRevogada     = "punicao_revogada"
	EventoPunicaoExpirada     = "punicao_expirada"
	EventoRecursoAberto       = "recurso_aberto"
	EventoRecursoJulgado      = "recurso_julgado"
	EventoPagamentoRetido     = "pagamento_retido"
	EventoPagamentoReaberto   = "pagamento_reaberto"
)

// Ações do admin na moderação (admin_auditoria.acao) e tipos de alvo.
const (
	AcaoDenunciaAssumir = "denuncia_assumir"
	AcaoDenunciaDecidir = "denuncia_decidir"
	AcaoPunicaoAplicar  = "punicao_aplicar"
	AcaoPunicaoRevogar  = "punicao_revogar"
	AcaoRecursoJulgar   = "recurso_julgar"

	AlvoDenuncia = "denuncia"
	AlvoPunicao  = "punicao"
	AlvoRecurso  = "recurso"
)

// Limites de texto (runas, após TrimSpace).
const (
	decisaoTextoMin       = 3
	decisaoTextoMax       = 2000
	recursoTextoMin       = 20
	recursoTextoMax       = 2000
	punicoesPorDecisaoMax = 5
	reincidenciaJanela    = 90 * 24 * time.Hour
)

// ModeracaoConfig são os parâmetros da moderação vindos da configuração (config.Config).
type ModeracaoConfig struct {
	PrazoAcademia     time.Duration // DENUNCIA_PRAZO_ACADEMIA_HORAS
	LimiteDia         int           // DENUNCIA_LIMITE_DIA (0 = sem limite)
	EvidenciaMaxBytes int64         // DENUNCIA_EVIDENCIA_MAX_BYTES
	EvidenciasDir     string        // EVIDENCIAS_DIR (privada)
	PrazoRecurso      time.Duration // RECURSO_PRAZO_DIAS
}

// PapelLeitor diz quem pede o arquivo de uma evidência.
type PapelLeitor string

// Quem pode ler uma evidência: o denunciante, a academia julgadora (enquanto a jurisdição for dela) e o admin.
const (
	LeitorDenunciante PapelLeitor = "denunciante"
	LeitorAcademia    PapelLeitor = "academia"
	LeitorAdmin       PapelLeitor = "admin"
)

// ArquivoEvidencia é o conteúdo de uma evidência pronto para enviar.
type ArquivoEvidencia struct {
	Conteudo []byte
	Mime     string
	Nome     string // nome de download sugerido (nunca o original)
}

// ModeracaoService concentra denúncias, punições e recursos (usuário, academia, admin e jobs).
type ModeracaoService interface {
	// Usuário
	CriarDenuncia(userID uint, req *models.CriarDenunciaRequest) (*models.DenunciaResponse, error)
	EnviarEvidencia(userID, idDenuncia uint, conteudo []byte) (*models.EvidenciaResponse, error)
	MinhasDenuncias(userID uint, pagina, limite int) (*models.PaginaAdmin[models.DenunciaResponse], error)
	MinhaDenuncia(userID, id uint) (*models.DenunciaResponse, error)
	MinhasPunicoes(userID uint, pagina, limite int) (*models.PaginaAdmin[models.PunicaoResponse], error)
	AbrirRecurso(userID, idPunicao uint, texto string) (*models.RecursoResponse, error)
	// RecursoBanimento é a rota pública do banido (Q1): confere e-mail/senha, sem emitir sessão. Sem texto
	// só consulta; com texto abre o recurso (criado=true) ou devolve o existente.
	RecursoBanimento(req *models.RecursoBanimentoRequest) (info *models.BanimentoInfo, criado bool, err error)
	// Banimento devolve o banimento vigente (nil se não houver) — 403 conta_banida do login.
	Banimento(idUsuario uint) (*models.BanimentoInfo, error)
	Evidencia(papel PapelLeitor, idLeitor, idDenuncia uint, ordem uint8) (*ArquivoEvidencia, error)

	// Academia (jurisdição: suborno_instrutor em desafio dela, até o prazo)
	FilaAcademia(userID uint, status string, pagina, limite int) (*models.PaginaAdmin[models.DenunciaResponse], error)
	DenunciaAcademia(userID, id uint) (*models.DenunciaAcademiaDetalhe, error)
	AssumirAcademia(userID, id uint) (*models.DenunciaResponse, error)
	EncaminharAcademia(userID, id uint, justificativa string) (*models.DenunciaResponse, error)
	DecidirAcademia(userID, id uint, req *models.DecidirDenunciaAcademiaRequest) (*models.DenunciaResponse, error)
	PunicoesAcademia(userID uint, tipo string, vigentes bool, pagina, limite int) (*models.PaginaAdmin[models.PunicaoResponse], error)

	// Admin (motivo obrigatório e admin_auditoria na mesma transação)
	Denuncias(f repositories.FiltroDenuncias) (*models.PaginaAdmin[models.DenunciaResponse], error)
	DenunciaAdmin(id uint) (*models.DenunciaAdminDetalhe, error)
	AssumirAdmin(ator AtorAdmin, id uint, motivo string) (*models.DenunciaResponse, error)
	DecidirAdmin(ator AtorAdmin, id uint, req *models.DecidirDenunciaAdminRequest) (*models.DenunciaAdminDetalhe, error)
	Punicoes(f repositories.FiltroPunicoes) (*models.PaginaAdmin[models.PunicaoResponse], error)
	AplicarPunicao(ator AtorAdmin, req *models.AplicarPunicaoRequest) (*models.PunicaoResponse, error)
	RevogarPunicao(ator AtorAdmin, id uint, motivo string) (*models.PunicaoResponse, error)
	Recursos(status string, pagina, limite int) (*models.PaginaAdmin[models.RecursoFilaItem], error)
	JulgarRecurso(ator AtorAdmin, id uint, req *models.JulgarRecursoRequest) (*models.RecursoFilaItem, error)
	Eventos(entidade string, idEntidade, idDesafio uint, pagina, limite int) (*models.PaginaAdmin[models.EventoResponse], error)
	// BloquearUsuario / DesbloquearUsuario: o "bloquear" do painel virou banimento (punição) e o
	// "desbloquear" revoga os banimentos vigentes. false = já estava no estado pedido.
	BloquearUsuario(ator AtorAdmin, id uint, motivo string) (bool, error)
	DesbloquearUsuario(ator AtorAdmin, id uint, motivo string) (bool, error)

	// Dinheiro do pódio (parte 2): anulação do resultado (superadmin), filas do admin e job de liberação.
	AnularResultado(ator AtorAdmin, idDesafio uint, req *models.AnularResultadoRequest) (*models.AnulacaoResponse, error)
	PagamentosPendentes(f repositories.FiltroPendentes) (*models.PaginaAdmin[models.PagamentoPendenteResponse], error)
	Debitos(abertos bool, idUsuario *uint, pagina, limite int) (*models.PaginaAdmin[models.DebitoResponse], error)

	// Jobs
	EscalonarDenuncias(limite int) (int, error)
	EncerrarSuspensoes(limite int) (int, error)
	// LiberarPagamentos credita os pendentes a_liberar com a janela vencida (por desafio, numa transação).
	LiberarPagamentos(limite int) (int, error)
}

type moderacaoService struct {
	db        *gorm.DB
	denuncias repositories.DenunciaRepository
	punicoes  repositories.PunicaoRepository
	pendentes repositories.PagamentoPendenteRepository
	eventos   repositories.ModeracaoEventoRepository
	consultas repositories.ModeracaoConsultaRepository
	usuarios  repositories.UsuarioRepository
	academias repositories.AcademiaRepository
	auditoria repositories.AuditoriaRepository
	notif     Notificador
	cfg       ModeracaoConfig
	agora     func() time.Time

	// Parte 2 (dinheiro do pódio): liberação, anulação e débitos. Montados a partir do db.
	carteira    *repositories.CarteiraRepository
	debitos     repositories.DebitoRepository
	desafios    *repositories.DesafioRepository
	desafioMod  repositories.DesafioModeracaoRepository
	receitas    *repositories.ReceitaRepository
	instrutores repositories.InstrutorRepository
}

// NovoModeracaoService cria o serviço de moderação.
func NovoModeracaoService(db *gorm.DB, denuncias repositories.DenunciaRepository, punicoes repositories.PunicaoRepository,
	pendentes repositories.PagamentoPendenteRepository, eventos repositories.ModeracaoEventoRepository,
	consultas repositories.ModeracaoConsultaRepository, usuarios repositories.UsuarioRepository,
	academias repositories.AcademiaRepository, auditoria repositories.AuditoriaRepository, notif Notificador,
	cfg ModeracaoConfig) ModeracaoService {
	if notif == nil {
		notif = NotificadorNulo{}
	}
	return &moderacaoService{db: db, denuncias: denuncias, punicoes: punicoes, pendentes: pendentes, eventos: eventos,
		consultas: consultas, usuarios: usuarios, academias: academias, auditoria: auditoria, notif: notif, cfg: cfg,
		agora: time.Now, carteira: repositories.NovoCarteiraRepository(db), debitos: repositories.NovoDebitoRepository(db),
		desafios: repositories.NovoDesafioRepository(db), desafioMod: repositories.NovoDesafioModeracaoRepository(db),
		receitas: repositories.NovoReceitaRepository(db), instrutores: repositories.NovoInstrutorRepository(db)}
}

// now é o relógio da moderação truncado em milissegundos (as colunas são DATETIME(3); o MySQL
// ARREDONDA a fração, o que poderia gravar `inicio` 1 ms no futuro e tirar a punição da vigência).
func (s *moderacaoService) now() time.Time { return s.agora().Truncate(time.Millisecond) }

// ─── Utilitários ─────────────────────────────────────────────────────────────

// textoEntre normaliza (TrimSpace) e valida o tamanho em runas.
func textoEntre(v string, min, max int, campo string) (string, error) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	if n < min {
		return "", erroRequisicao(fmt.Sprintf("%s deve ter pelo menos %d caracteres", campo, min))
	}
	if n > max {
		return "", erroRequisicao(fmt.Sprintf("%s deve ter no máximo %d caracteres", campo, max))
	}
	return v, nil
}

func categoriaValida(c string) bool {
	switch c {
	case models.DenunciaSubornoInstrutor, models.DenunciaManipulacaoPodio, models.DenunciaFraudePresenca,
		models.DenunciaCondutaAbusiva, models.DenunciaPerfilFalso, models.DenunciaOutro:
		return true
	}
	return false
}

func statusDecisaoValido(st string) bool {
	return st == models.DenunciaProcedente || st == models.DenunciaImprocedente || st == models.DenunciaArquivada
}

// jurisdicaoDaDenuncia decide quem julga (spec §2.1.7): academia só em suborno_instrutor de desafio
// dela; se o denunciado é a própria conta da academia (conflito de interesse) vai ao admin, mantendo
// id_academia_julgadora só como histórico.
func jurisdicaoDaDenuncia(categoria string, d *repositories.DesafioDenuncia, idDenunciado uint) (jurisdicao string, idAcademia *uint) {
	if d == nil || d.IDAcademia == nil || categoria != models.DenunciaSubornoInstrutor {
		return models.JurisdicaoAdmin, nil
	}
	if d.IDUsuarioAcademia != nil && *d.IDUsuarioAcademia == idDenunciado {
		return models.JurisdicaoAdmin, d.IDAcademia
	}
	return models.JurisdicaoAcademia, d.IDAcademia
}

// atorEvento é quem age numa linha de moderacao_eventos (o CHECK chk_eventos_ator confere o par).
type atorEvento struct {
	papel     string
	idUsuario *uint
	idAdmin   *uint
}

func atorUsuario(id uint) atorEvento  { return atorEvento{papel: models.AtorUsuario, idUsuario: &id} }
func atorAcademia(id uint) atorEvento { return atorEvento{papel: models.AtorAcademia, idUsuario: &id} }
func atorDoAdmin(a AtorAdmin) atorEvento {
	id := a.ID
	return atorEvento{papel: models.AtorAdmin, idAdmin: &id}
}

var atorSistema = atorEvento{papel: models.AtorSistema}

// registrar grava uma linha da linha do tempo na transação `tx`. dados vira {"antes":..., "depois":...}
// quando antes/depois são informados (nil = omitido).
func (s *moderacaoService) registrar(tx *gorm.DB, ator atorEvento, acao, entidade string, idEntidade uint,
	idDesafio *uint, justificativa string, antes, depois any) error {
	e := &models.ModeracaoEvento{IDAtor: ator.idUsuario, IDAdmin: ator.idAdmin, AtorPapel: ator.papel, Acao: acao,
		Entidade: entidade, IDEntidade: idEntidade, IDDesafio: idDesafio}
	if justificativa != "" {
		j := justificativa
		e.Justificativa = &j
	}
	if antes != nil || depois != nil {
		dados := map[string]any{}
		if antes != nil {
			dados["antes"] = antes
		}
		if depois != nil {
			dados["depois"] = depois
		}
		b, err := json.Marshal(dados)
		if err != nil {
			return err
		}
		e.Dados = b
	}
	return s.eventos.Registrar(tx, e)
}

func (s *moderacaoService) auditar(tx *gorm.DB, ator AtorAdmin, acao, alvo string, id uint, motivo string, detalhes any) error {
	return RegistrarAuditoria(s.auditoria, tx, ator.ID, ator.IP, acao, alvo, id, motivo, detalhes)
}

// posCommit acumula notificações para disparar só depois do commit.
type posCommit struct{ fns []func() }

func (p *posCommit) add(f func()) { p.fns = append(p.fns, f) }
func (p *posCommit) disparar() {
	for _, f := range p.fns {
		f()
	}
}

func nomeCompleto(u models.Usuario) string { return strings.TrimSpace(u.Nome + " " + u.Sobrenome) }

func refUsuario(u models.Usuario, comEmail bool) models.UsuarioRefModeracao {
	r := models.UsuarioRefModeracao{ID: u.ID, Nome: nomeCompleto(u), Username: u.Username}
	if comEmail {
		r.Email = u.Email
	}
	return r
}

func dataBR(t time.Time) string { return t.Local().Format("02/01/2006 15:04") }

// ─── Montagem das respostas ──────────────────────────────────────────────────

type publicoDenuncia int

const (
	paraDenunciante publicoDenuncia = iota
	paraAcademia
	paraAdmin
)

func evidenciasResponse(es []models.DenunciaEvidencia) []models.EvidenciaResponse {
	out := make([]models.EvidenciaResponse, 0, len(es))
	for _, e := range es {
		out = append(out, models.EvidenciaResponse{Ordem: e.Ordem, Mime: e.Mime, TamanhoBytes: e.TamanhoBytes, CriadoEm: e.CriadoEm})
	}
	return out
}

// denunciasResponse monta as respostas respeitando o sigilo: o denunciante (e quem assumiu) só para o admin.
func (s *moderacaoService) denunciasResponse(ds []models.Denuncia, publico publicoDenuncia, idLeitor uint) ([]models.DenunciaResponse, error) {
	ids := make([]uint, 0, len(ds)*2)
	for _, d := range ds {
		ids = append(ids, d.IDDenunciado)
		if publico == paraAdmin {
			ids = append(ids, d.IDDenunciante)
		}
	}
	us, err := s.consultas.UsuariosPorID(ids)
	if err != nil {
		return nil, err
	}
	out := make([]models.DenunciaResponse, 0, len(ds))
	for _, d := range ds {
		r := models.DenunciaResponse{ID: d.ID, Denunciado: refUsuario(us[d.IDDenunciado], publico == paraAdmin),
			IDDesafio: d.IDDesafio, Categoria: d.Categoria, Descricao: d.Descricao, Status: d.Status,
			Jurisdicao: d.Jurisdicao, DecididaEm: d.DecididaEm, DecisaoTexto: d.DecisaoTexto, CriadoEm: d.CriadoEm,
			Evidencias: evidenciasResponse(d.Evidencias)}
		if r.Denunciado.ID == 0 {
			r.Denunciado.ID = d.IDDenunciado
		}
		switch publico {
		case paraDenunciante:
			n, err := s.consultas.PunicoesDaDenuncia(d.ID)
			if err != nil {
				return nil, err
			}
			gerou := n > 0
			r.GerouPunicao = &gerou
		case paraAcademia:
			r.IDAcademiaJulgadora, r.PrazoAcademiaAte = d.IDAcademiaJulgadora, d.PrazoAcademiaAte
			comigo := d.Status == models.DenunciaEmAnalise && d.IDResponsavel != nil && *d.IDResponsavel == idLeitor
			r.EmAnaliseComigo = &comigo
		case paraAdmin:
			r.IDAcademiaJulgadora, r.PrazoAcademiaAte, r.EncaminhadaEm = d.IDAcademiaJulgadora, d.PrazoAcademiaAte, d.EncaminhadaEm
			idDen := d.IDDenunciante
			r.IDDenunciante = &idDen
			ref := refUsuario(us[d.IDDenunciante], true)
			ref.ID = d.IDDenunciante
			r.Denunciante = &ref
			r.IDResponsavel, r.IDResponsavelAdmin = d.IDResponsavel, d.IDResponsavelAdmin
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *moderacaoService) denunciaResponse(d *models.Denuncia, publico publicoDenuncia, idLeitor uint) (*models.DenunciaResponse, error) {
	rs, err := s.denunciasResponse([]models.Denuncia{*d}, publico, idLeitor)
	if err != nil {
		return nil, err
	}
	return &rs[0], nil
}

func recursoResponse(rc *models.Recurso, admin bool) *models.RecursoResponse {
	if rc == nil {
		return nil
	}
	r := &models.RecursoResponse{ID: rc.ID, IDPunicao: rc.IDPunicao, Texto: rc.Texto, Status: rc.Status,
		JulgadoEm: rc.JulgadoEm, DecisaoTexto: rc.DecisaoTexto, CriadoEm: rc.CriadoEm}
	if admin {
		r.JulgadoPor = rc.JulgadoPor
	}
	return r
}

// prazoRecurso devolve até quando o punido pode recorrer.
func (s *moderacaoService) prazoRecurso(p *models.Punicao) time.Time {
	return p.Inicio.Add(s.cfg.PrazoRecurso)
}

// podeRecorrer: punição não revogada, sem recurso e dentro do prazo.
func (s *moderacaoService) podeRecorrer(p *models.Punicao, rc *models.Recurso, agora time.Time) bool {
	return p.RevogadaEm == nil && rc == nil && !agora.After(s.prazoRecurso(p))
}

func (s *moderacaoService) punicoesResponse(ps []models.Punicao, admin bool) ([]models.PunicaoResponse, error) {
	ids := make([]uint, 0, len(ps))
	uids := make([]uint, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
		uids = append(uids, p.IDUsuario)
	}
	recursos, err := s.consultas.RecursosDasPunicoes(ids)
	if err != nil {
		return nil, err
	}
	var us map[uint]models.Usuario
	if us, err = s.consultas.UsuariosPorID(uids); err != nil {
		return nil, err
	}
	agora := s.now()
	out := make([]models.PunicaoResponse, 0, len(ps))
	for i := range ps {
		p := &ps[i]
		var rc *models.Recurso
		if v, ok := recursos[p.ID]; ok {
			rc = &v
		}
		r := models.PunicaoResponse{ID: p.ID, IDUsuario: p.IDUsuario, Tipo: p.Tipo, Escopo: p.Escopo, IDAcademia: p.IDAcademia,
			IDDesafio: p.IDDesafio, IDDenuncia: p.IDDenuncia, Inicio: p.Inicio, Fim: p.Fim, Justificativa: p.Justificativa,
			Vigente: p.Vigente(agora), AplicadaPor: models.AtorAdmin, RevogadaEm: p.RevogadaEm, RevogadaMotivo: p.RevogadaMotivo,
			Recurso: recursoResponse(rc, admin), PodeRecorrer: s.podeRecorrer(p, rc, agora), CriadoEm: p.CriadoEm}
		if p.AplicadaPor != nil {
			r.AplicadaPor = models.AtorAcademia
		}
		if p.RevogadaEm == nil {
			ate := s.prazoRecurso(p)
			r.RecorrerAte = &ate
		}
		if u, ok := us[p.IDUsuario]; ok {
			ref := refUsuario(u, admin)
			r.Usuario = &ref
		}
		if admin {
			r.IDAplicadorUser, r.IDAplicadorAdm, r.RevogadaPor = p.AplicadaPor, p.AplicadaPorAdmin, p.RevogadaPor
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *moderacaoService) punicaoResponse(p *models.Punicao, admin bool) (*models.PunicaoResponse, error) {
	rs, err := s.punicoesResponse([]models.Punicao{*p}, admin)
	if err != nil {
		return nil, err
	}
	return &rs[0], nil
}

// ─── Usuário: denúncias ──────────────────────────────────────────────────────

func (s *moderacaoService) CriarDenuncia(userID uint, req *models.CriarDenunciaRequest) (*models.DenunciaResponse, error) {
	if !categoriaValida(req.Categoria) {
		return nil, erroRequisicao("Categoria de denúncia inválida")
	}
	descricao, err := textoEntre(req.Descricao, models.DenunciaDescricaoMin, models.DenunciaDescricaoMax, "A descrição")
	if err != nil {
		return nil, err
	}
	if req.IDDenunciado == userID {
		return nil, erroRequisicao("Você não pode denunciar a si mesmo")
	}
	if models.CategoriaDeDesafio(req.Categoria) && req.IDDesafio == nil {
		return nil, erroRequisicao("Informe o desafio: esta categoria se refere a um desafio")
	}
	eu, err := s.usuarios.BuscarPorID(userID)
	if err != nil {
		return nil, err
	}
	if eu == nil {
		return nil, erro(http.StatusUnauthorized, "Usuário não encontrado")
	}
	if eu.Suspenso(s.agora()) {
		return nil, ErroContaSuspensa(eu.SuspensoAte) // Q2: abrir denúncia é participação
	}
	alvo, err := s.usuarios.BuscarPorID(req.IDDenunciado)
	if err != nil {
		return nil, err
	}
	if alvo == nil {
		return nil, erroNaoEncontrado("Usuário denunciado não encontrado")
	}
	var desafio *repositories.DesafioDenuncia
	if req.IDDesafio != nil {
		if desafio, err = s.consultas.DesafioParaDenuncia(*req.IDDesafio); err != nil {
			return nil, err
		}
		if desafio == nil {
			return nil, erroNaoEncontrado("Desafio não encontrado")
		}
		// Vínculo mínimo: evita denúncia "a esmo" que reteria prêmios de terceiros.
		for _, id := range []uint{userID, req.IDDenunciado} {
			ok, err := s.consultas.VinculadoAoDesafio(desafio.ID, id)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, erroRequisicao("Denunciante e denunciado precisam ter participado do desafio informado")
			}
		}
	}
	jurisdicao, idAcademia := jurisdicaoDaDenuncia(req.Categoria, desafio, req.IDDenunciado)
	agora := s.now()
	d := &models.Denuncia{IDDenunciante: userID, IDDenunciado: req.IDDenunciado, IDDesafio: req.IDDesafio,
		Categoria: req.Categoria, Descricao: descricao, Status: models.DenunciaAberta, Jurisdicao: jurisdicao,
		IDAcademiaJulgadora: idAcademia}
	if jurisdicao == models.JurisdicaoAcademia {
		prazo := agora.Add(s.cfg.PrazoAcademia)
		d.PrazoAcademiaAte = &prazo
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Denúncia que retém pagamentos trava o desafio ANTES do usuário (mesma ordem do pódio, da liberação
		// e da anulação: desafio -> usuários): a retenção e o job liberar-pagamentos ficam serializados e os
		// pendentes do desafio nunca terminam metade retidos, metade liberados.
		if d.IDDesafio != nil && models.CategoriaDeDesafio(d.Categoria) {
			if _, err := s.desafios.ComTx(tx).TravarPorID(*d.IDDesafio); err != nil {
				return err
			}
		}
		// Trava a linha do denunciante: o limite por dia não estoura com requisições simultâneas.
		if _, _, err := s.consultas.TravarUsuario(tx, userID); err != nil {
			return err
		}
		if s.cfg.LimiteDia > 0 {
			// Fora da tx (autocommit), depois da trava: enxerga as denúncias já confirmadas pelos concorrentes.
			n, err := s.denuncias.ContarDoDenuncianteDesde(userID, agora.Add(-24*time.Hour))
			if err != nil {
				return err
			}
			if n >= int64(s.cfg.LimiteDia) {
				return erro(http.StatusTooManyRequests, fmt.Sprintf("Limite de %d denúncias em 24 horas atingido", s.cfg.LimiteDia))
			}
		}
		if err := s.denuncias.Criar(tx, d); err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("Você já tem uma denúncia em aberto contra este usuário para este caso")
			}
			return err
		}
		if err := s.registrar(tx, atorUsuario(userID), EventoDenunciaCriada, models.EntidadeDenuncia, d.ID, d.IDDesafio, "", nil,
			map[string]any{"status": d.Status, "categoria": d.Categoria, "jurisdicao": d.Jurisdicao}); err != nil {
			return err
		}
		// Retenção (spec §2.1.9): denúncia de desafio retém os créditos do pódio ainda a liberar.
		if d.IDDesafio != nil && models.CategoriaDeDesafio(d.Categoria) {
			n, err := s.pendentes.Reter(tx, *d.IDDesafio, d.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				return s.registrar(tx, atorSistema, EventoPagamentoRetido, models.EntidadeDesafio, *d.IDDesafio, d.IDDesafio, "",
					nil, map[string]any{"id_denuncia": d.ID, "pagamentos": n})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notif.Notificar(userID, models.NotifDenunciaRegistrada, "Denúncia registrada",
		"Recebemos sua denúncia. Ela é sigilosa: a pessoa denunciada não sabe quem denunciou.", "/minhas-denuncias")
	if jurisdicao == models.JurisdicaoAcademia && desafio != nil && desafio.IDUsuarioAcademia != nil {
		s.notif.Notificar(*desafio.IDUsuarioAcademia, models.NotifDenunciaRecebida, "Nova denúncia para julgar",
			fmt.Sprintf("Uma denúncia de suborno de instrutor no desafio \"%s\" aguarda sua análise até %s.",
				desafio.Titulo, dataBR(*d.PrazoAcademiaAte)), "/academia/denuncias")
	}
	salva, err := s.denuncias.Buscar(nil, d.ID, false)
	if err != nil || salva == nil {
		return nil, err
	}
	return s.denunciaResponse(salva, paraDenunciante, userID)
}

func (s *moderacaoService) MinhasDenuncias(userID uint, pagina, limite int) (*models.PaginaAdmin[models.DenunciaResponse], error) {
	ds, total, err := s.denuncias.Listar(repositories.FiltroDenuncias{IDDenunciante: &userID, Pagina: pagina, Limite: limite})
	if err != nil {
		return nil, err
	}
	itens, err := s.denunciasResponse(ds, paraDenunciante, userID)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, pagina, limite), nil
}

func (s *moderacaoService) MinhaDenuncia(userID, id uint) (*models.DenunciaResponse, error) {
	d, err := s.denuncias.Buscar(nil, id, false)
	if err != nil {
		return nil, err
	}
	if d == nil || d.IDDenunciante != userID {
		return nil, erroNaoEncontrado("Denúncia não encontrada")
	}
	return s.denunciaResponse(d, paraDenunciante, userID)
}

// ─── Usuário: punições e recursos ────────────────────────────────────────────

func (s *moderacaoService) MinhasPunicoes(userID uint, pagina, limite int) (*models.PaginaAdmin[models.PunicaoResponse], error) {
	ps, total, err := s.punicoes.Listar(repositories.FiltroPunicoes{IDUsuario: &userID, Pagina: pagina, Limite: limite})
	if err != nil {
		return nil, err
	}
	itens, err := s.punicoesResponse(ps, false)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, pagina, limite), nil
}

// abrirRecursoTx valida e cria o recurso do próprio punido na transação.
func (s *moderacaoService) abrirRecursoTx(tx *gorm.DB, userID, idPunicao uint, texto string, agora time.Time) (*models.Recurso, error) {
	p, err := s.punicoes.Buscar(tx, idPunicao, true)
	if err != nil {
		return nil, err
	}
	if p == nil || p.IDUsuario != userID {
		return nil, erroNaoEncontrado("Punição não encontrada")
	}
	if p.RevogadaEm != nil {
		return nil, erroConflito("Esta punição já foi revogada")
	}
	if agora.After(s.prazoRecurso(p)) {
		return nil, erroRequisicao("O prazo para recorrer desta punição terminou em " + dataBR(s.prazoRecurso(p)))
	}
	rc := &models.Recurso{IDPunicao: p.ID, Texto: texto, Status: models.RecursoPendente}
	if err := s.punicoes.CriarRecurso(tx, rc); err != nil {
		if repositories.EhDuplicata(err) {
			return nil, erroConflito("Já existe um recurso para esta punição")
		}
		return nil, err
	}
	if err := s.registrar(tx, atorUsuario(userID), EventoRecursoAberto, models.EntidadeRecurso, rc.ID, p.IDDesafio, "", nil,
		map[string]any{"id_punicao": p.ID, "status": rc.Status}); err != nil {
		return nil, err
	}
	return rc, nil
}

func (s *moderacaoService) AbrirRecurso(userID, idPunicao uint, texto string) (*models.RecursoResponse, error) {
	texto, err := textoEntre(texto, recursoTextoMin, recursoTextoMax, "O texto do recurso")
	if err != nil {
		return nil, err
	}
	var rc *models.Recurso
	err = s.db.Transaction(func(tx *gorm.DB) error {
		rc, err = s.abrirRecursoTx(tx, userID, idPunicao, texto, s.now())
		return err
	})
	if err != nil {
		return nil, err
	}
	salvo, err := s.punicoes.BuscarRecurso(nil, rc.ID, false)
	if err != nil || salvo == nil {
		return nil, err
	}
	return recursoResponse(salvo, false), nil
}

// banimentoVigente devolve o banimento vigente mais antigo (o que define banido_em).
func (s *moderacaoService) banimentoVigente(idUsuario uint) (*models.Punicao, error) {
	ps, _, err := s.punicoes.Listar(repositories.FiltroPunicoes{IDUsuario: &idUsuario, Tipo: models.PunicaoBanimento,
		Vigentes: true, Agora: s.now(), Pagina: 1, Limite: 100})
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	maisAntigo := &ps[len(ps)-1] // Listar ordena por id DESC
	return maisAntigo, nil
}

func (s *moderacaoService) Banimento(idUsuario uint) (*models.BanimentoInfo, error) {
	p, err := s.banimentoVigente(idUsuario)
	if err != nil || p == nil {
		return nil, err
	}
	rc, err := s.punicoes.RecursoDaPunicao(nil, p.ID)
	if err != nil {
		return nil, err
	}
	info := &models.BanimentoInfo{IDPunicao: p.ID, Motivo: p.Justificativa, BanidoEm: p.Inicio,
		PodeRecorrer: s.podeRecorrer(p, rc, s.now()), Recurso: recursoResponse(rc, false)}
	ate := s.prazoRecurso(p)
	info.RecorrerAte = &ate
	return info, nil
}

// erroCredenciaisRecurso é a mensagem única da rota pública (não revela se o e-mail existe).
var erroCredenciaisRecurso = erro(http.StatusUnauthorized, "usuário ou senha inválidos")

func (s *moderacaoService) RecursoBanimento(req *models.RecursoBanimentoRequest) (*models.BanimentoInfo, bool, error) {
	u, err := s.usuarios.BuscarPorEmail(strings.TrimSpace(req.Email))
	if err != nil {
		return nil, false, err
	}
	if u == nil {
		_ = bcrypt.CompareHashAndPassword(hashInexistente, []byte(req.Senha)) // tempo constante
		return nil, false, erroCredenciaisRecurso
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Senha), []byte(req.Senha)) != nil {
		return nil, false, erroCredenciaisRecurso
	}
	if !u.Banido() {
		// R3-1: mesma resposta da senha errada, para a rota não virar oráculo de senha.
		return nil, false, erroCredenciaisRecurso
	}
	info, err := s.Banimento(u.ID)
	if err != nil {
		return nil, false, err
	}
	if info == nil {
		// banido_em sem punição vigente (estado inconsistente): não há o que recorrer.
		return nil, false, erroConflito("Não há banimento vigente para recorrer. Entre em contato com o suporte")
	}
	if strings.TrimSpace(req.Texto) == "" {
		return info, false, nil
	}
	if info.Recurso != nil {
		return info, false, nil // um recurso por punição: devolve o andamento
	}
	if _, err := s.AbrirRecurso(u.ID, info.IDPunicao, req.Texto); err != nil {
		return nil, false, err
	}
	info, err = s.Banimento(u.ID)
	return info, err == nil, err
}
