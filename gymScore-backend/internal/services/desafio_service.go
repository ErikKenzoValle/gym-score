package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FiltroListagem são os filtros de GET /desafios e GET /desafios/meus.
type FiltroListagem struct {
	Status     models.StatusDesafio
	Categoria  string
	Busca      string
	IDAcademia uint
	Pagina     int
	Limite     int
}

// DesafioService define as operações de negócio para desafios. Todas recebem o
// userID do JWT: a identidade nunca vem do corpo da requisição.
//
// Fluxo: a academia cria o desafio e designa um instrutor; o atleta se inscreve pagando a
// entrada; o instrutor (ou a academia) confirma presença lendo o QR do ingresso, inicia o
// desafio e lança o pódio, que divide o pote, credita os prêmios e encerra.
type DesafioService interface {
	Criar(userID uint, req *models.CriarDesafioRequest) (*models.DesafioResponse, error)
	Editar(userID, id uint, req *models.EditarDesafioRequest) (*models.DesafioResponse, error)
	// AutorizarEdicao / AutorizarCandidatura checam existência (404) e permissão (403) SEM olhar o corpo;
	// o controller chama antes de validar o JSON para que 403 vença 400.
	AutorizarEdicao(userID, id uint) error
	AutorizarCandidatura(userID, id uint) error
	Listar(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error)
	ListarMeus(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error)
	Detalhe(userID, id uint) (*models.DesafioResponse, error)
	Entrar(userID, id uint) (*models.DesafioResponse, error)
	Sair(userID, id uint) (*models.DesafioResponse, error)
	Cancelar(userID, id uint) (*models.DesafioResponse, error)
	Iniciar(userID, id uint) (*models.DesafioResponse, error)
	MarcarPresenca(userID, id uint, req *models.PresencaRequest) (*models.PresencaResponse, error)
	ListarPresentes(userID, id uint) ([]models.ParticipanteResponse, error)
	LancarPodio(userID, id uint, req *models.LancarPodioRequest) (*models.DesafioResponse, error)
	Podio(userID, id uint) ([]models.PodioResponse, error)
	Avaliar(userID, id uint, nota int) (*models.AvaliacaoDesafio, error)
	Convidar(userID, id, idAmigo uint) error

	// Vaga de instrutor (leilão com teto): ver candidatura_service.go.
	Candidatar(userID, id uint, req *models.CandidatarRequest) (*models.CandidaturaResponse, bool, error)
	RetirarCandidatura(userID, id uint) (*models.CandidaturaResponse, error)
	ListarCandidaturas(userID, id uint) ([]models.CandidaturaResponse, error)
	EscolherCandidatura(userID, id, idCandidatura uint) (*models.DesafioResponse, error)
	ListarVagas(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error)

	// ExpirarVencidos é o passo do job de expiração (ver desafio_service.go).
	ExpirarVencidos(carencia time.Duration, limite int) (int, error)
	// CancelarPeloAdmin: cancelamento pelo painel admin com auditoria na mesma transação.
	CancelarPeloAdmin(id uint, auditar func(tx *gorm.DB, d *models.Desafio) error) (*models.Desafio, error)
}

type desafioService struct {
	db         *gorm.DB
	desafios   *repositories.DesafioRepository
	carteira   *repositories.CarteiraRepository
	amizades   repositories.AmizadeRepository
	usuarios   repositories.UsuarioRepository
	academias  repositories.AcademiaRepository
	instrutors repositories.InstrutorRepository
	avaliacoes repositories.AvaliacaoRepository
	notif      Notificador
	agora      func() time.Time

	candidaturas *repositories.CandidaturaRepository
	receitas     *repositories.ReceitaRepository
	// taxaInscricaoPct é a taxa da plataforma vigente (PLATAFORMA_TAXA_INSCRICAO_PCT); cada desafio
	// grava o valor da época da criação em taxa_plataforma_pct.
	taxaInscricaoPct int

	// Janela de contestação do pódio (moderação): os créditos do encerramento viram pagamentos_pendentes
	// e só entram no ledger depois de `janela` (job liberar-pagamentos).
	janela     time.Duration
	pendentes  repositories.PagamentoPendenteRepository
	denuncias  repositories.DenunciaRepository
	eventosMod repositories.ModeracaoEventoRepository
	desafioMod repositories.DesafioModeracaoRepository
}

// NovoDesafioService cria uma nova instância do serviço de desafios
func NovoDesafioService(
	db *gorm.DB,
	desafios *repositories.DesafioRepository,
	carteira *repositories.CarteiraRepository,
	amizades repositories.AmizadeRepository,
	usuarios repositories.UsuarioRepository,
	academias repositories.AcademiaRepository,
	instrutores repositories.InstrutorRepository,
	avaliacoes repositories.AvaliacaoRepository,
	notif Notificador,
	candidaturas *repositories.CandidaturaRepository,
	receitas *repositories.ReceitaRepository,
	taxaInscricaoPct int,
	janelaContestacao time.Duration,
) DesafioService {
	return &desafioService{db: db, desafios: desafios, carteira: carteira, amizades: amizades,
		usuarios: usuarios, academias: academias, instrutors: instrutores, avaliacoes: avaliacoes,
		notif: notif, agora: time.Now, candidaturas: candidaturas, receitas: receitas, taxaInscricaoPct: taxaInscricaoPct,
		janela: janelaContestacao, pendentes: repositories.NovoPagamentoPendenteRepository(db),
		denuncias: repositories.NovoDenunciaRepository(db), eventosMod: repositories.NovoModeracaoEventoRepository(db),
		desafioMod: repositories.NovoDesafioModeracaoRepository(db)}
}

// NovoDesafioServiceComRelogio permite injetar o relógio (testes de data do evento).
func NovoDesafioServiceComRelogio(
	db *gorm.DB,
	desafios *repositories.DesafioRepository,
	carteira *repositories.CarteiraRepository,
	amizades repositories.AmizadeRepository,
	usuarios repositories.UsuarioRepository,
	academias repositories.AcademiaRepository,
	instrutores repositories.InstrutorRepository,
	avaliacoes repositories.AvaliacaoRepository,
	notif Notificador,
	candidaturas *repositories.CandidaturaRepository,
	receitas *repositories.ReceitaRepository,
	taxaInscricaoPct int,
	janelaContestacao time.Duration,
	agora func() time.Time,
) DesafioService {
	s := NovoDesafioService(db, desafios, carteira, amizades, usuarios, academias, instrutores, avaliacoes, notif,
		candidaturas, receitas, taxaInscricaoPct, janelaContestacao).(*desafioService)
	s.agora = agora
	return s
}

// eventos acumula efeitos colaterais (notificações) executados somente após o commit.
type eventos struct{ fns []func() }

func (e *eventos) add(f func()) { e.fns = append(e.fns, f) }
func (e *eventos) disparar() {
	for _, f := range e.fns {
		f()
	}
}

func linkDesafio(id uint) string { return fmt.Sprintf("/desafios?id=%d", id) }

// idsExceto devolve os ids dos participantes, sem o usuário informado (0 = ninguém excluído).
func idsExceto(ps []models.DesafioParticipante, excluir uint) []uint {
	ids := make([]uint, 0, len(ps))
	for _, p := range ps {
		if p.IDUsuario != excluir {
			ids = append(ids, p.IDUsuario)
		}
	}
	return ids
}

// traduzir converte erros de infraestrutura conhecidos em erros de negócio.
func traduzir(err error) error {
	if errors.Is(err, repositories.ErrSaldoInsuficiente) {
		return ErroSaldoInsuficiente
	}
	return err
}

// novoCodigoIngresso gera o token do QR: 128 bits do CSPRNG em 32 caracteres hex.
func novoCodigoIngresso() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// parseDataEvento aceita RFC3339 ou "YYYY-MM-DDTHH:MM[:SS]" / "YYYY-MM-DD HH:MM[:SS]" (horário local).
func parseDataEvento(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, erroRequisicao("data_evento inválida (use RFC3339 ou YYYY-MM-DDTHH:MM)")
}

func mesmoDia(a, b time.Time) bool {
	a, b = a.In(time.Local), b.In(time.Local)
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// ─── Contexto do usuário (papel) ─────────────────────────────────────────────

// contextoUsuario é o papel de quem age: o perfil vem do banco (não do token) e, para academia e
// instrutor, o id da respectiva tabela.
// AcademiaAprovada/InstrutorAprovado: o cadastro passou pela aprovação do admin; sem ela a conta
// não opera desafios (não é dona nem instrutora de nenhum).
type contextoUsuario struct {
	ID                uint
	Perfil            string
	AcademiaID        uint
	InstrutorID       uint
	AcademiaAprovada  bool
	InstrutorAprovado bool
	// SuspensoAte: fim da suspensão global vigente (nil = não suspenso). Ver contextoAtivo (Q2).
	SuspensoAte *time.Time
}

// contextoAtivo é o contexto das ações de participação/gestão: recusa a conta suspensa (403
// conta_suspensa com o fim — Q2). Leituras usam contexto() e continuam liberadas ao suspenso.
func (s *desafioService) contextoAtivo(userID uint) (*contextoUsuario, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	if ctx.SuspensoAte != nil {
		return nil, ErroContaSuspensa(ctx.SuspensoAte)
	}
	return ctx, nil
}

func (s *desafioService) contexto(userID uint) (*contextoUsuario, error) {
	u, err := s.usuarios.BuscarPorID(userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, erro(401, "Usuário não encontrado")
	}
	if u.Banido() {
		return nil, ErroContaBanida(nil)
	}
	c := &contextoUsuario{ID: userID, Perfil: u.Perfil}
	if u.Suspenso(s.agora()) {
		c.SuspensoAte = u.SuspensoAte
	}
	switch u.Perfil {
	case "academia":
		a, err := s.academias.BuscarPorUsuario(userID)
		if err != nil {
			return nil, err
		}
		if a != nil {
			c.AcademiaID = a.ID
			c.AcademiaAprovada = a.Status == models.CadastroAprovado
		}
	case "instrutor":
		i, err := s.instrutors.BuscarPorUsuario(userID)
		if err != nil {
			return nil, err
		}
		if i != nil {
			c.InstrutorID = i.ID
			c.InstrutorAprovado = i.Status == models.CadastroAprovado
		}
	}
	return c, nil
}

// dono: é a academia dona do desafio (legado sem id_academia: a conta que o criou).
func (c *contextoUsuario) dono(d *models.Desafio) bool {
	if c.Perfil != "academia" || !c.AcademiaAprovada {
		return false
	}
	if d.IDAcademia != nil {
		return c.AcademiaID != 0 && *d.IDAcademia == c.AcademiaID
	}
	return d.IDCriador == c.ID
}

// instrutorDe: é o instrutor designado do desafio.
func (c *contextoUsuario) instrutorDe(d *models.Desafio) bool {
	return c.InstrutorID != 0 && c.InstrutorAprovado && d.IDInstrutor != nil && *d.IDInstrutor == c.InstrutorID
}

// staff: pode operar o desafio no dia (iniciar, presença, pódio).
func (c *contextoUsuario) staff(d *models.Desafio) bool { return c.dono(d) || c.instrutorDe(d) }

func exigirStaff(c *contextoUsuario, d *models.Desafio) error {
	if !c.staff(d) {
		return erroProibido("Apenas o instrutor designado ou a academia dona podem realizar esta ação")
	}
	return nil
}

func exigirDono(c *contextoUsuario, d *models.Desafio) error {
	if !c.dono(d) {
		return erroProibido("Apenas a academia dona do desafio pode realizar esta ação")
	}
	return nil
}

// perfilTravado lê o perfil do usuário com SELECT ... FOR UPDATE (só dentro de transação). Usado por
// Entrar e pela promoção a instrutor para que as duas não se cruzem (N4).
func perfilTravado(tx *gorm.DB, userID uint) (string, error) {
	var perfis []string
	if err := tx.Raw("SELECT perfil FROM usuarios WHERE id = ? FOR UPDATE", userID).Scan(&perfis).Error; err != nil {
		return "", err
	}
	if len(perfis) == 0 {
		return "", erro(401, "Usuário não encontrado")
	}
	return perfis[0], nil
}

// exigirSemBloqueioAcademia recusa quem tem bloqueio_academia vigente na academia dona do desafio
// (punição de escopo academia: não entra em desafios nem se candidata a vagas dela).
func exigirSemBloqueioAcademia(repo *repositories.DesafioRepository, idUsuario uint, d *models.Desafio, agora time.Time) error {
	if d.IDAcademia == nil {
		return nil
	}
	bloqueado, err := repo.UsuarioBloqueadoNaAcademia(idUsuario, *d.IDAcademia, agora)
	if err != nil {
		return err
	}
	if bloqueado {
		return erroProibido("Você está bloqueado pela academia deste desafio")
	}
	return nil
}

func exigirAtleta(c *contextoUsuario) error {
	if c.Perfil != "atleta" {
		return erroProibido("Apenas atletas podem realizar esta ação")
	}
	return nil
}

// ─── Criação e edição ────────────────────────────────────────────────────────

// instrutorVinculadoTravado é instrutorDaAcademia com SELECT ... FOR UPDATE no instrutor, dentro da
// transação em andamento. O desvínculo trava a mesma linha e confere os desafios ativos: sem este lock,
// um desafio criado/editado na janela ficaria designado a um instrutor já desvinculado.
func (s *desafioService) instrutorVinculadoTravado(tx *gorm.DB, idInstrutor, idAcademia uint) (*models.Instrutor, error) {
	var i models.Instrutor
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Usuario").First(&i, idInstrutor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, erroRequisicao("Instrutor inválido ou não vinculado à sua academia")
	}
	if err != nil {
		return nil, err
	}
	if i.IDAcademia == nil || *i.IDAcademia != idAcademia {
		return nil, erroRequisicao("Instrutor inválido ou não vinculado à sua academia")
	}
	if i.Status != models.CadastroAprovado {
		return nil, erroRequisicao("O cadastro deste instrutor ainda não foi aprovado")
	}
	return &i, nil
}

func validarCategoria(c string) (string, error) {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" {
		return models.CategoriaPadrao, nil
	}
	if !models.CategoriaValida(c) {
		return "", erroRequisicao("Categoria inválida (use: " + strings.Join(models.CategoriasDesafio, ", ") + ")")
	}
	return c, nil
}

// Criar cria o desafio da academia do JWT. O instrutor é designado (id_instrutor vinculado) ou a vaga
// fica aberta (instrutores se candidatam até o teto). Numa única transação grava o desafio e debita
// da academia a reserva (prêmio fixo + parte fixa do instrutor, ou o teto na vaga aberta); sem saldo
// a criação falha com 422. A academia não paga entrada nem vira participante.
func (s *desafioService) Criar(userID uint, req *models.CriarDesafioRequest) (*models.DesafioResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	if ctx.Perfil != "academia" || ctx.AcademiaID == 0 {
		return nil, erroProibido("Apenas academias podem criar desafios")
	}
	if !ctx.AcademiaAprovada {
		return nil, ErroCadastroNaoAprovado
	}
	if req.ValorEntradaCentavos < 0 || req.ValorEntradaCentavos > models.EntradaMaxCentavos {
		return nil, erroRequisicao("Valor de entrada fora do permitido (R$ 0,00 a R$ 10.000,00)")
	}
	if req.Vagas < models.VagasMin || req.Vagas > models.VagasMax {
		return nil, erroRequisicao(fmt.Sprintf("Vagas devem estar entre %d e %d", models.VagasMin, models.VagasMax))
	}
	if req.PosicoesPodio < models.PosicoesPodioMin || req.PosicoesPodio > models.PosicoesPodioMax {
		return nil, erroRequisicao("posicoes_podio deve estar entre 1 e 5")
	}
	if req.PosicoesPodio > req.Vagas {
		return nil, erroRequisicao("posicoes_podio não pode ser maior que o número de vagas")
	}
	categoria, err := validarCategoria(req.Categoria)
	if err != nil {
		return nil, err
	}
	evento, err := parseDataEvento(req.DataEvento)
	if err != nil {
		return nil, err
	}
	if !evento.After(s.agora()) {
		return nil, erroRequisicao("data_evento deve estar no futuro")
	}

	cfg := ConfigFinanceira{
		PremioFixo: req.PremioFixoCentavos, DividirPote: req.DividirPote == nil || *req.DividirPote,
		Taxa: s.taxaInscricaoPct, Modo: req.InstrutorModoPagamento, Fixo: req.InstrutorFixoCentavos,
		Pct: req.InstrutorPct, Selecao: req.InstrutorSelecao, Teto: req.InstrutorTetoCentavos,
	}
	if cfg.Modo == "" {
		cfg.Modo = models.InstrutorFixo
	}
	if cfg.Selecao == "" {
		cfg.Selecao = models.SelecaoDesignado
	}
	if req.PercentualPote != nil {
		cfg.PercentualPote = *req.PercentualPote
	}
	if err := cfg.Validar(req.PercentualPote == nil); err != nil {
		return nil, err
	}

	var instrutor *models.Instrutor
	switch cfg.Selecao {
	case models.SelecaoDesignado:
		if req.IDInstrutor == 0 {
			return nil, erroRequisicao("Informe id_instrutor ou abra a vaga de instrutor (instrutor_selecao=vaga_aberta)")
		}
	case models.SelecaoVagaAberta:
		if req.IDInstrutor != 0 {
			return nil, erroRequisicao("Vaga aberta não aceita id_instrutor: o instrutor sai da escolha entre os candidatos")
		}
	}

	academiaID := ctx.AcademiaID
	reserva := cfg.Reserva()
	desafio := &models.Desafio{
		Titulo:                  strings.TrimSpace(req.Titulo),
		Categoria:               categoria,
		ValorEntradaCentavos:    req.ValorEntradaCentavos,
		DataEvento:              &evento,
		Latitude:                req.Latitude,
		Longitude:               req.Longitude,
		Vagas:                   req.Vagas,
		PosicoesPodio:           req.PosicoesPodio,
		Status:                  models.StatusAberto,
		IDCriador:               userID,
		IDAcademia:              &academiaID,
		PremioFixoCentavos:      cfg.PremioFixo,
		DividirPote:             cfg.DividirPote,
		PercentualPote:          cfg.PercentualPote,
		TaxaPlataformaPct:       cfg.Taxa,
		InstrutorModoPagamento:  cfg.Modo,
		InstrutorFixoCentavos:   cfg.Fixo,
		InstrutorPct:            cfg.Pct,
		InstrutorSelecao:        cfg.Selecao,
		InstrutorTetoCentavos:   cfg.Teto,
		ReservaAcademiaCentavos: reserva,
	}
	if req.Descricao != "" {
		desafio.Descricao = &req.Descricao
	}
	local := strings.TrimSpace(req.Local)
	desafio.Local = &local

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if cfg.Selecao == models.SelecaoDesignado {
			i, err := s.instrutorVinculadoTravado(tx, req.IDInstrutor, ctx.AcademiaID)
			if err != nil {
				return err
			}
			instrutor = i
			desafio.IDInstrutor = &i.ID
		}
		if err := s.desafios.ComTx(tx).Criar(desafio); err != nil {
			return err
		}
		if reserva == 0 {
			return nil
		}
		_, err := s.carteira.Debitar(tx, repositories.Lancamento{
			IDUsuario: userID, Tipo: models.TipoReservaDesafio, ValorCentavos: reserva,
			IDDesafio: &desafio.ID, Descricao: "Reserva do desafio: " + desafio.Titulo,
		})
		return err
	})
	if errors.Is(err, repositories.ErrSaldoInsuficiente) {
		return nil, erro(ErroSaldoInsuficiente.Status, msgSaldoReserva(reserva))
	}
	if err != nil {
		return nil, err
	}
	if instrutor != nil {
		s.notif.Notificar(instrutor.IDUsuario, models.NotifInstrutorDesignado, "Você foi designado instrutor",
			fmt.Sprintf("Você foi designado para monitorar o desafio \"%s\".", desafio.Titulo), linkDesafio(desafio.ID))
	}
	return s.Detalhe(userID, desafio.ID)
}

// AutorizarEdicao: desafio existe (404) e o usuário é a academia dona (403).
func (s *desafioService) AutorizarEdicao(userID, id uint) error {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return err
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return err
	}
	if d == nil {
		return erroNaoEncontrado("Desafio não encontrado")
	}
	return exigirDono(ctx, d)
}

// AutorizarCandidatura: desafio existe (404) e o usuário é instrutor (403).
func (s *desafioService) AutorizarCandidatura(userID, id uint) error {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return err
	}
	if ctx.InstrutorID == 0 {
		return erroProibido("Apenas instrutores podem se candidatar a uma vaga")
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return err
	}
	if d == nil {
		return erroNaoEncontrado("Desafio não encontrado")
	}
	return nil
}

// Editar (só academia dona, só aberto) atualiza os campos informados. Não altera a entrada se já
// houver inscrito, nem as vagas abaixo dos inscritos. Trocar o instrutor notifica o antigo e o novo.
func (s *desafioService) Editar(userID, id uint, req *models.EditarDesafioRequest) (*models.DesafioResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirDono(ctx, d); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Só é possível editar desafios abertos")
		}
		inscritos, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		campos := map[string]any{}
		vagas, posicoes := d.Vagas, d.PosicoesPodio

		if req.Titulo != nil {
			campos["titulo"] = strings.TrimSpace(*req.Titulo)
		}
		if req.Descricao != nil {
			if *req.Descricao == "" {
				campos["descricao"] = nil
			} else {
				campos["descricao"] = *req.Descricao
			}
		}
		if req.Categoria != nil {
			c, err := validarCategoria(*req.Categoria)
			if err != nil {
				return err
			}
			campos["categoria"] = c
		}
		if req.Local != nil {
			local := strings.TrimSpace(*req.Local)
			if local == "" {
				return erroRequisicao("local não pode ser vazio")
			}
			campos["local"] = local
		}
		if req.Latitude != nil {
			campos["latitude"] = *req.Latitude
		}
		if req.Longitude != nil {
			campos["longitude"] = *req.Longitude
		}
		if req.DataEvento != nil {
			evento, err := parseDataEvento(*req.DataEvento)
			if err != nil {
				return err
			}
			if d.DataEvento == nil || !evento.Equal(*d.DataEvento) {
				if !evento.After(s.agora()) {
					return erroRequisicao("data_evento deve estar no futuro")
				}
				campos["data_evento"] = evento
			}
		}
		if req.Vagas != nil {
			if *req.Vagas < models.VagasMin || *req.Vagas > models.VagasMax {
				return erroRequisicao(fmt.Sprintf("Vagas devem estar entre %d e %d", models.VagasMin, models.VagasMax))
			}
			if int64(*req.Vagas) < inscritos {
				return erroConflito(fmt.Sprintf("Já há %d inscritos: as vagas não podem ser menores que isso", inscritos))
			}
			vagas = *req.Vagas
			campos["vagas"] = vagas
		}
		if req.PosicoesPodio != nil {
			if *req.PosicoesPodio < models.PosicoesPodioMin || *req.PosicoesPodio > models.PosicoesPodioMax {
				return erroRequisicao("posicoes_podio deve estar entre 1 e 5")
			}
			posicoes = *req.PosicoesPodio
			campos["posicoes_podio"] = posicoes
		}
		if posicoes > vagas {
			return erroRequisicao("posicoes_podio não pode ser maior que o número de vagas")
		}
		if req.ValorEntradaCentavos != nil && *req.ValorEntradaCentavos != d.ValorEntradaCentavos {
			if *req.ValorEntradaCentavos < 0 || *req.ValorEntradaCentavos > models.EntradaMaxCentavos {
				return erroRequisicao("Valor de entrada fora do permitido (R$ 0,00 a R$ 10.000,00)")
			}
			if inscritos > 0 {
				return erroConflito("Não é possível alterar a entrada: já há atletas inscritos")
			}
			campos["valor_entrada_centavos"] = *req.ValorEntradaCentavos
		}
		if req.IDInstrutor != nil && (d.IDInstrutor == nil || *req.IDInstrutor != *d.IDInstrutor) {
			if d.InstrutorSelecao == models.SelecaoVagaAberta {
				return erroConflito("Este desafio tem vaga de instrutor aberta: o instrutor é escolhido entre os candidatos")
			}
			if ctx.AcademiaID == 0 {
				return erroProibido("Apenas a academia dona do desafio pode realizar esta ação")
			}
			novo, err := s.instrutorVinculadoTravado(tx, *req.IDInstrutor, ctx.AcademiaID)
			if err != nil {
				return err
			}
			campos["id_instrutor"] = novo.ID
			titulo := d.Titulo
			if t, ok := campos["titulo"].(string); ok {
				titulo = t
			}
			idDesafio := d.ID
			if d.IDInstrutor != nil {
				if antigo, err := s.instrutors.BuscarPorID(*d.IDInstrutor); err == nil && antigo != nil {
					ev.add(func() {
						s.notif.Notificar(antigo.IDUsuario, models.NotifInstrutorRemovido, "Você não monitora mais um desafio",
							fmt.Sprintf("A academia designou outro instrutor para o desafio \"%s\".", titulo), linkDesafio(idDesafio))
					})
				}
			}
			ev.add(func() {
				s.notif.Notificar(novo.IDUsuario, models.NotifInstrutorDesignado, "Você foi designado instrutor",
					fmt.Sprintf("Você foi designado para monitorar o desafio \"%s\".", titulo), linkDesafio(idDesafio))
			})
		}
		if err := s.editarFinanceiro(tx, repo, d, req, inscritos, campos); err != nil {
			return err
		}
		return repo.Atualizar(d.ID, campos)
	})
	if err != nil {
		return nil, err
	}
	return s.Detalhe(userID, id)
}

// editarFinanceiro aplica ao mapa de campos as mudanças de premiação e de pagamento do instrutor do
// PATCH e ajusta a reserva da academia pela diferença (débito `reserva_desafio` ou crédito
// `estorno_reserva`). A taxa da plataforma continua a gravada na criação. Com atletas inscritos o
// prêmio só pode aumentar; depois de escolhido o instrutor da vaga aberta os campos dele ficam travados.
func (s *desafioService) editarFinanceiro(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio,
	req *models.EditarDesafioRequest, inscritos int64, campos map[string]any) error {
	tocaPremio := req.PremioFixoCentavos != nil || req.DividirPote != nil || req.PercentualPote != nil
	tocaInstrutor := req.InstrutorModoPagamento != nil || req.InstrutorFixoCentavos != nil ||
		req.InstrutorPct != nil || req.InstrutorTetoCentavos != nil
	if !tocaPremio && !tocaInstrutor {
		return nil
	}
	antes := configDoDesafio(d)
	novo := antes
	if req.PremioFixoCentavos != nil {
		novo.PremioFixo = *req.PremioFixoCentavos
	}
	if req.DividirPote != nil {
		novo.DividirPote = *req.DividirPote
	}
	// percentual_pote omitido: mantém o atual (o default "máximo" é só da criação).
	if req.PercentualPote != nil {
		novo.PercentualPote = *req.PercentualPote
	}
	if req.InstrutorModoPagamento != nil {
		novo.Modo = *req.InstrutorModoPagamento
	}
	if req.InstrutorFixoCentavos != nil {
		novo.Fixo = *req.InstrutorFixoCentavos
	}
	if req.InstrutorPct != nil {
		novo.Pct = *req.InstrutorPct
	}
	if req.InstrutorTetoCentavos != nil {
		t := *req.InstrutorTetoCentavos
		novo.Teto = &t
	}
	if antes.Escolhido && (novo.Modo != antes.Modo || novo.Fixo != antes.Fixo || novo.Pct != antes.Pct ||
		(novo.Teto != nil && antes.Teto != nil && *novo.Teto != *antes.Teto)) {
		return erroConflito("O instrutor já foi escolhido: o pagamento dele não pode mais ser alterado")
	}
	// Ao desligar o % do instrutor (mudança de modo para sem %), o pct antigo deixa de valer.
	if !models.ModoTemPercentual(novo.Modo) && req.InstrutorPct == nil {
		novo.Pct = 0
	}
	if !models.ModoTemFixo(novo.Modo) && req.InstrutorFixoCentavos == nil && !novo.Escolhido {
		novo.Fixo = 0
	}
	// Religar o pote sem informar o percentual usa o máximo permitido (como na criação).
	religou := req.DividirPote != nil && *req.DividirPote && !antes.DividirPote && req.PercentualPote == nil
	if err := novo.Validar(religou); err != nil {
		return err
	}
	if inscritos > 0 && tocaPremio {
		potePremio := func(c ConfigFinanceira) int {
			if c.DividirPote {
				return c.PercentualPote
			}
			return 0
		}
		if novo.PremioFixo < antes.PremioFixo || potePremio(novo) < potePremio(antes) {
			return erroConflito("Já há atletas inscritos: o prêmio (fixo e percentual do pote) só pode aumentar")
		}
	}
	if novo.Selecao == models.SelecaoVagaAberta && !novo.Escolhido && req.InstrutorTetoCentavos != nil {
		maior, err := s.candidaturas.ComTx(tx).MaiorLancePendente(d.ID)
		if err != nil {
			return err
		}
		if maior > *novo.Teto {
			return erroConflito(fmt.Sprintf("Há candidatura com lance de %s, acima do novo teto", reaisBR(maior)))
		}
	}

	campos["premio_fixo_centavos"] = novo.PremioFixo
	campos["dividir_pote"] = novo.DividirPote
	campos["percentual_pote"] = novo.PercentualPote
	campos["instrutor_modo_pagamento"] = novo.Modo
	campos["instrutor_fixo_centavos"] = novo.Fixo
	campos["instrutor_pct"] = novo.Pct
	if novo.Teto != nil {
		campos["instrutor_teto_centavos"] = *novo.Teto
	}

	reserva := novo.Reserva()
	campos["reserva_academia_centavos"] = reserva
	diff := reserva - d.ReservaAcademiaCentavos
	switch {
	case diff > 0:
		if _, err := s.carteira.Debitar(tx, repositories.Lancamento{
			IDUsuario: d.IDCriador, Tipo: models.TipoReservaDesafio, ValorCentavos: diff,
			IDDesafio: &d.ID, Descricao: "Reserva adicional do desafio: " + d.Titulo,
		}); err != nil {
			if errors.Is(err, repositories.ErrSaldoInsuficiente) {
				return erro(ErroSaldoInsuficiente.Status, msgSaldoReserva(diff))
			}
			return err
		}
	case diff < 0:
		if _, err := s.carteira.Creditar(tx, repositories.Lancamento{
			IDUsuario: d.IDCriador, Tipo: models.TipoEstornoReserva, ValorCentavos: -diff,
			IDDesafio: &d.ID, Descricao: "Reserva devolvida (edição do desafio): " + d.Titulo,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ─── Transições ──────────────────────────────────────────────────────────────

// comDesafioTravado abre uma transação, trava a linha do desafio (FOR UPDATE) e executa fn.
func (s *desafioService) comDesafioTravado(id uint, fn func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) error {
	ev := &eventos{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		repo := s.desafios.ComTx(tx)
		d, err := repo.TravarPorID(id)
		if err != nil {
			return err
		}
		if d == nil {
			return erroNaoEncontrado("Desafio não encontrado")
		}
		return fn(tx, repo, d, ev)
	})
	if err == nil {
		ev.disparar() // notificações só depois do commit
	}
	return traduzir(err)
}

func (s *desafioService) acao(userID, id uint, fn func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) (*models.DesafioResponse, error) {
	return s.acaoCom(userID, id, true, fn)
}

// acaoCom é acao com a escolha de recusar (exigirAtivo) ou não a conta suspensa: sair do desafio (com
// estorno) continua permitido ao suspenso.
func (s *desafioService) acaoCom(userID, id uint, exigirAtivo bool, fn func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) (*models.DesafioResponse, error) {
	obter := s.contexto
	if exigirAtivo {
		obter = s.contextoAtivo
	}
	ctx, err := obter(userID)
	if err != nil {
		return nil, err
	}
	if err := s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		return fn(ctx, tx, repo, d, ev)
	}); err != nil {
		return nil, err
	}
	return s.Detalhe(userID, id)
}

// Entrar (só atleta) debita a entrada e ocupa uma vaga (atômico; a trava do desafio evita estourar
// as vagas). Gera o código do ingresso (QR). Entrada 0 = gratuito: não mexe no ledger.
func (s *desafioService) Entrar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirAtleta(ctx); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Desafio não está aberto para inscrições")
		}
		if d.DataEvento != nil && !d.DataEvento.After(s.agora()) {
			return erroConflito("O evento já começou: as inscrições estão encerradas")
		}
		// Reconfere o perfil sob a trava da linha do usuário: uma promoção a instrutor simultânea
		// (que trava a mesma linha) não deixa um instrutor inscrito como atleta (N4).
		if perfil, err := perfilTravado(tx, userID); err != nil {
			return err
		} else if perfil != "atleta" {
			return erroProibido("Apenas atletas podem realizar esta ação")
		}
		if jaParticipa, err := repo.UsuarioParticipa(d.ID, userID); err != nil {
			return err
		} else if jaParticipa {
			return erroConflito("Você já está participando deste desafio")
		}
		if err := exigirSemBloqueioAcademia(repo, userID, d, s.agora()); err != nil {
			return err
		}
		inscritos, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		if int(inscritos) >= d.Vagas {
			return erroConflito("Não há mais vagas neste desafio")
		}
		if d.ValorEntradaCentavos > 0 {
			if _, err := s.carteira.Debitar(tx, repositories.Lancamento{
				IDUsuario: userID, Tipo: models.TipoEntradaDesafio, ValorCentavos: d.ValorEntradaCentavos,
				IDDesafio: &d.ID, Descricao: "Entrada no desafio: " + d.Titulo,
			}); err != nil {
				return err
			}
		}
		codigo, err := novoCodigoIngresso()
		if err != nil {
			return err
		}
		if err := repo.AdicionarParticipante(&models.DesafioParticipante{IDDesafio: d.ID, IDUsuario: userID, CodigoIngresso: codigo}); err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("Você já está participando deste desafio")
			}
			return err
		}
		titulo, idDesafio := d.Titulo, d.ID
		ev.add(func() {
			s.notif.Notificar(userID, models.NotifInscricaoConfirmada, "Inscrição confirmada",
				fmt.Sprintf("Você está inscrito em \"%s\". Apresente seu ingresso ao instrutor no dia do evento.", titulo), linkDesafio(idDesafio))
		})
		return nil
	})
}

// Sair (só atleta, só aberto) devolve a entrada e libera a vaga.
func (s *desafioService) Sair(userID, id uint) (*models.DesafioResponse, error) {
	return s.acaoCom(userID, id, false, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirAtleta(ctx); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Só é possível sair enquanto o desafio está aberto")
		}
		removido, err := repo.RemoverParticipante(d.ID, userID)
		if err != nil {
			return err
		}
		if !removido {
			return erroConflito("Você não está participando deste desafio")
		}
		if d.ValorEntradaCentavos == 0 {
			return nil
		}
		_, err = s.carteira.Creditar(tx, repositories.Lancamento{
			IDUsuario: userID, Tipo: models.TipoEstornoDesafio, ValorCentavos: d.ValorEntradaCentavos,
			IDDesafio: &d.ID, Descricao: "Reembolso (saída do desafio): " + d.Titulo,
		})
		return err
	})
}

// Cancelar (só academia dona, aberto ou em andamento) cancela, reembolsa integralmente todos os inscritos
// (sem taxa) e devolve a reserva inteira à academia.
func (s *desafioService) Cancelar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirDono(ctx, d); err != nil {
			return err
		}
		return s.cancelarTravado(tx, repo, d, ev, cancelAcademia)
	})
}

// origemCancelamento diz quem cancelou (muda só o texto das notificações e quem é avisado).
type origemCancelamento int

const (
	cancelAcademia origemCancelamento = iota // a academia dona
	cancelJob                                // job de expiração (prazo vencido)
	cancelAdmin                              // painel admin (moderação)
)

// CancelarPeloAdmin cancela o desafio pelo painel admin, sem exigir dono, com a MESMA lógica de
// Cancelar (estornos, reserva, candidaturas, notificações). `auditar` roda dentro da transação do
// cancelamento, depois dele (grava admin_auditoria): se falhar, nada é cancelado.
func (s *desafioService) CancelarPeloAdmin(id uint, auditar func(tx *gorm.DB, d *models.Desafio) error) (*models.Desafio, error) {
	var cancelado *models.Desafio
	err := s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		statusAnterior := d.Status
		if err := s.cancelarTravado(tx, repo, d, ev, cancelAdmin); err != nil {
			return err
		}
		copia := *d
		copia.Status = statusAnterior // a auditoria registra de onde saiu
		cancelado = &copia
		return auditar(tx, &copia)
	})
	return cancelado, err
}

// cancelarTravado é a lógica única de cancelamento (academia dona, job de expiração e admin), com o
// desafio já travado na transação: CAS de status, estorno integral das entradas, devolução da
// reserva, candidaturas recusadas e notificações pós-commit. Não checa permissão.
func (s *desafioService) cancelarTravado(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos, origem origemCancelamento) error {
	automatico := origem != cancelAcademia // job e admin avisam também a academia
	if d.Status != models.StatusAberto && d.Status != models.StatusEmAndamento {
		return erroConflito("Apenas desafios abertos ou em andamento podem ser cancelados")
	}
	mudou, err := repo.MudarStatus(d.ID, d.Status, models.StatusCancelado, nil)
	if err != nil {
		return err
	}
	if !mudou {
		return erroConflito("O desafio já mudou de estado")
	}
	participantes, err := repo.ListarParticipantes(d.ID)
	if err != nil {
		return err
	}
	// Estorno integral das entradas (sem taxa) e devolução da reserva inteira à academia, tudo
	// na mesma transação e em ordem estável de usuário. reserva_academia_centavos fica como está:
	// quem diz que a reserva foi devolvida é o status cancelado e a linha estorno_reserva.
	var creditos []creditoPlanejado
	if d.ValorEntradaCentavos > 0 {
		for _, p := range participantes {
			creditos = append(creditos, creditoPlanejado{repositories.Lancamento{
				IDUsuario: p.IDUsuario, Tipo: models.TipoEstornoDesafio, ValorCentavos: d.ValorEntradaCentavos,
				IDDesafio: &d.ID, Descricao: "Reembolso (desafio cancelado): " + d.Titulo,
			}})
		}
	}
	if d.ReservaAcademiaCentavos > 0 {
		creditos = append(creditos, creditoPlanejado{repositories.Lancamento{
			IDUsuario: d.IDCriador, Tipo: models.TipoEstornoReserva, ValorCentavos: d.ReservaAcademiaCentavos,
			IDDesafio: &d.ID, Descricao: "Reserva devolvida (desafio cancelado): " + d.Titulo,
		}})
	}
	if err := s.aplicarCreditos(tx, creditos); err != nil {
		return err
	}
	// Candidatos pendentes da vaga aberta são dispensados e avisados.
	candidatosUsuarios, err := s.candidaturas.ComTx(tx).RecusarPendentes(d.ID, 0)
	if err != nil {
		return err
	}
	ids, titulo, idDesafio, reembolso := idsExceto(participantes, 0), d.Titulo, d.ID, d.ValorEntradaCentavos > 0
	academiaUser, reserva := d.IDCriador, d.ReservaAcademiaCentavos
	var instrutorUser uint
	if d.IDInstrutor != nil {
		if i, err := s.instrutors.BuscarPorID(*d.IDInstrutor); err == nil && i != nil {
			instrutorUser = i.IDUsuario
		}
	}
	ev.add(func() {
		causa := "foi cancelado"
		switch origem {
		case cancelJob:
			causa = "foi cancelado automaticamente (prazo do evento vencido sem encerramento)"
		case cancelAdmin:
			causa = "foi cancelado pela equipe GymScore"
		}
		msg := fmt.Sprintf("O desafio \"%s\" %s.", titulo, causa)
		if reembolso {
			msg = fmt.Sprintf("O desafio \"%s\" %s e sua entrada foi reembolsada.", titulo, causa)
		}
		s.notif.NotificarVarios(ids, models.NotifDesafioCancel, "Desafio cancelado", msg, linkDesafio(idDesafio))
		if instrutorUser != 0 {
			msgInst := fmt.Sprintf("A academia cancelou o desafio \"%s\".", titulo)
			if automatico {
				msgInst = fmt.Sprintf("O desafio \"%s\" %s.", titulo, causa)
			}
			s.notif.Notificar(instrutorUser, models.NotifDesafioCancel, "Desafio cancelado", msgInst, linkDesafio(idDesafio))
		}
		if automatico {
			msgAcad := fmt.Sprintf("O desafio \"%s\" %s.", titulo, causa)
			if reserva > 0 {
				msgAcad = fmt.Sprintf("O desafio \"%s\" %s e a reserva de %s voltou ao seu saldo.", titulo, causa, reaisBR(reserva))
			}
			s.notif.Notificar(academiaUser, models.NotifDesafioCancel, "Desafio cancelado", msgAcad, linkDesafio(idDesafio))
		}
		s.notif.NotificarVarios(candidatosUsuarios, models.NotifCandidaturaRecusada, "Vaga cancelada",
			fmt.Sprintf("O desafio \"%s\" foi cancelado e sua candidatura foi encerrada.", titulo), linkDesafio(idDesafio))
	})
	return nil
}

// vencido diz se o desafio passou do prazo para ser operado: `aberto` ou `em_andamento` com
// data_evento + carência no passado. Desafio sem data_evento nunca vence.
func vencido(d *models.Desafio, agora time.Time, carencia time.Duration) bool {
	if d.Status != models.StatusAberto && d.Status != models.StatusEmAndamento {
		return false
	}
	// Q3: resultado anulado aguardando relançamento — a carência conta da anulação.
	if d.Status == models.StatusEmAndamento && d.ResultadoAnuladoEm != nil {
		return agora.After(d.ResultadoAnuladoEm.Add(carencia))
	}
	if d.DataEvento == nil {
		return false
	}
	return agora.After(d.DataEvento.Add(carencia))
}

// ExpirarVencidos (job) cancela, com a mesma lógica de Cancelar e sem exigir dono, os desafios abertos
// ou em andamento cuja data_evento + carência já passou. Seguro com N instâncias: cada desafio é
// travado (FOR UPDATE), a condição é reconferida sob a trava e a troca de status é condicional.
// Devolve quantos foram cancelados por esta chamada.
func (s *desafioService) ExpirarVencidos(carencia time.Duration, limite int) (int, error) {
	agora := s.agora()
	ids, err := s.desafios.IDsVencidos(agora.Add(-carencia), limite)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		cancelou := false
		err := s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
			if !vencido(d, agora, carencia) {
				return nil // outra instância (ou a academia) já tratou
			}
			cancelou = true
			return s.cancelarTravado(tx, repo, d, ev, cancelJob)
		})
		if err != nil {
			var neg *ErroNegocio
			if errors.As(err, &neg) && neg.Status == 409 {
				continue // mudou de estado entre a listagem e a trava
			}
			return n, fmt.Errorf("expirar desafio %d: %w", id, err)
		}
		if cancelou {
			n++
		}
	}
	return n, nil
}

// creditoPlanejado é um crédito a aplicar numa transação financeira com vários destinatários.
type creditoPlanejado struct{ repositories.Lancamento }

// aplicarCreditos credita todos em ordem estável de usuário (evita deadlock entre transações que
// tocam os mesmos saldos em ordens diferentes).
func (s *desafioService) aplicarCreditos(tx *gorm.DB, cs []creditoPlanejado) error {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].IDUsuario < cs[j].IDUsuario })
	for _, c := range cs {
		if _, err := s.carteira.Creditar(tx, c.Lancamento); err != nil {
			return err
		}
	}
	return nil
}

// Iniciar (instrutor designado ou academia dona, desafio aberto, >= 1 inscrito) move para em_andamento
// e fecha as inscrições.
func (s *desafioService) Iniciar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirStaff(ctx, d); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Apenas desafios abertos podem ser iniciados")
		}
		if d.IDInstrutor == nil {
			return erroConflito("Escolha um instrutor entre os candidatos antes de iniciar o desafio")
		}
		n, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		if n < 1 {
			return erroConflito("Nenhum atleta inscrito: aguarde inscrições ou cancele o desafio")
		}
		mudou, err := repo.MudarStatus(d.ID, models.StatusAberto, models.StatusEmAndamento, nil)
		if err != nil {
			return err
		}
		if !mudou {
			return erroConflito("Apenas desafios abertos podem ser iniciados")
		}
		ps, err := repo.ListarParticipantes(d.ID)
		if err != nil {
			return err
		}
		ids, titulo, idDesafio := idsExceto(ps, 0), d.Titulo, d.ID
		ev.add(func() {
			s.notif.NotificarVarios(ids, models.NotifDesafioIniciado, "Desafio iniciado",
				fmt.Sprintf("O desafio \"%s\" começou. Bora treinar!", titulo), linkDesafio(idDesafio))
		})
		return nil
	})
}

// ─── Presença ────────────────────────────────────────────────────────────────

// elegivelPodio devolve o filtro de quem pode ter presença marcada e entrar no pódio: só inscritos
// com perfil `atleta` hoje e que não sejam o usuário do instrutor designado (N1/N4: academia ou
// instrutor inscritos em desafio legado/corrida não podem se premiar).
func (s *desafioService) elegivelPodio(repo *repositories.DesafioRepository, d *models.Desafio) (func(uint) bool, error) {
	perfis, err := repo.PerfisParticipantes(d.ID)
	if err != nil {
		return nil, err
	}
	var instrutorUser uint
	if d.IDInstrutor != nil {
		i, err := s.instrutors.BuscarPorID(*d.IDInstrutor)
		if err != nil {
			return nil, err
		}
		if i != nil {
			instrutorUser = i.IDUsuario
		}
	}
	return func(uid uint) bool { return perfis[uid] == "atleta" && uid != instrutorUser }, nil
}

// MarcarPresenca (instrutor designado ou academia dona) confirma a presença de um inscrito, pelo
// código do ingresso (QR) ou pelo id do usuário. Idempotente. Permitida em `aberto` no dia do
// evento e em `em_andamento`.
func (s *desafioService) MarcarPresenca(userID, id uint, req *models.PresencaRequest) (*models.PresencaResponse, error) {
	codigo := strings.ToLower(strings.TrimSpace(req.CodigoIngresso))
	if (codigo == "") == (req.IDUsuario == 0) {
		return nil, erroRequisicao("Informe codigo_ingresso OU id_usuario (apenas um dos dois)")
	}
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	var resp *models.PresencaResponse
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirStaff(ctx, d); err != nil {
			return err
		}
		switch d.Status {
		case models.StatusEmAndamento:
		case models.StatusAberto:
			if d.DataEvento != nil && !mesmoDia(*d.DataEvento, s.agora()) {
				return erroConflito("A presença só pode ser marcada no dia do evento")
			}
		default:
			return erroConflito("A presença só pode ser marcada em desafios abertos (no dia do evento) ou em andamento")
		}

		var p *models.DesafioParticipante
		var err error
		if codigo != "" {
			if len(codigo) != 32 || !ehHex(codigo) {
				return erroRequisicao("Código de ingresso inválido")
			}
			p, err = repo.BuscarParticipantePorCodigo(d.ID, codigo)
		} else {
			p, err = repo.BuscarParticipante(d.ID, req.IDUsuario)
		}
		if err != nil {
			return err
		}
		if p == nil {
			return erroNaoEncontrado("Ingresso não encontrado neste desafio")
		}
		elegivel, err := s.elegivelPodio(repo, d)
		if err != nil {
			return err
		}
		if !elegivel(p.IDUsuario) {
			return erroConflito("Só atletas inscritos podem ter presença confirmada (academia e instrutor do desafio não competem)")
		}
		agora := s.agora()
		// Registra quem (staff logado) e como (leitura do QR do ingresso ou escolha manual do inscrito)
		// confirmou a presença (N6); presença já marcada não é sobrescrita.
		modo := models.PresencaManual
		if codigo != "" {
			modo = models.PresencaQR
		}
		marcou, err := repo.MarcarPresenteComOrigem(d.ID, p.IDUsuario, userID, modo, agora)
		if err != nil {
			return err
		}
		em := p.PresenteEm
		if marcou {
			em = &agora
		}
		total, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		presentes, err := repo.ContarPresentes(d.ID)
		if err != nil {
			return err
		}
		resp = &models.PresencaResponse{
			IDUsuario: p.IDUsuario, Presente: true, PresenteEm: em, JaPresente: !marcou,
			PresentesCount: int(presentes), ParticipantesCount: int(total),
		}
		if p.Usuario != nil {
			resp.Nome, resp.Sobrenome = p.Usuario.Nome, p.Usuario.Sobrenome
		}
		if marcou {
			atleta, titulo, idDesafio := p.IDUsuario, d.Titulo, d.ID
			ev.add(func() {
				s.notif.Notificar(atleta, models.NotifPresencaConfirmada, "Presença confirmada",
					fmt.Sprintf("Sua presença no desafio \"%s\" foi confirmada.", titulo), linkDesafio(idDesafio))
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func ehHex(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// ListarPresentes (instrutor designado ou academia dona) devolve os atletas com presença confirmada.
func (s *desafioService) ListarPresentes(userID, id uint) ([]models.ParticipanteResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	if err := exigirStaff(ctx, d); err != nil {
		return nil, err
	}
	ps, err := s.desafios.ListarPresentes(id)
	if err != nil {
		return nil, err
	}
	out := make([]models.ParticipanteResponse, 0, len(ps))
	for i := range ps {
		out = append(out, paraParticipanteResponse(&ps[i]))
	}
	return out, nil
}

// ─── Pódio ───────────────────────────────────────────────────────────────────

// LancarPodio (instrutor designado ou academia dona, desafio em andamento) grava o pódio, divide o
// pote, credita os prêmios, aplica pontos, encerra e notifica — tudo numa transação.
//
// Regras: exatamente min(posicoes_podio, presentes) entradas, só presentes, sem repetir e com marca
// obrigatória. Pote = entrada x inscritos (todos, presentes ou não).
func (s *desafioService) LancarPodio(userID, id uint, req *models.LancarPodioRequest) (*models.DesafioResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirStaff(ctx, d); err != nil {
			return err
		}
		// Q3: depois de uma anulação em relancar_podio, só a academia dona relança (o instrutor
		// designado costuma ser o alvo da denúncia).
		relancamento := d.ResultadoAnuladoEm != nil
		if relancamento && !ctx.dono(d) {
			return erroProibido("O resultado foi anulado: só a academia dona pode relançar o pódio")
		}
		switch d.Status {
		case models.StatusEmAndamento:
		case models.StatusEncerrado:
			return erroConflito("O pódio deste desafio já foi lançado")
		default:
			return erroConflito("O pódio só pode ser lançado com o desafio em andamento")
		}
		participantes, err := repo.ListarParticipantes(d.ID)
		if err != nil {
			return err
		}
		elegivel, err := s.elegivelPodio(repo, d)
		if err != nil {
			return err
		}
		// Só atletas presentes contam (presença antiga de academia/instrutor, se houver, é ignorada).
		presentes := map[uint]bool{}
		for _, p := range participantes {
			if p.Presente && elegivel(p.IDUsuario) {
				presentes[p.IDUsuario] = true
			}
		}
		if len(presentes) == 0 {
			return erroConflito("Nenhum atleta presente: marque a presença ou cancele o desafio")
		}
		esperado := d.PosicoesPodio
		if len(presentes) < esperado {
			esperado = len(presentes)
		}
		if len(req.Posicoes) != esperado {
			return erroRequisicao(fmt.Sprintf("O pódio deve ter exatamente %d posições (%d configuradas, %d presentes)",
				esperado, d.PosicoesPodio, len(presentes)))
		}
		vistos := map[uint]bool{}
		for i, e := range req.Posicoes {
			if !presentes[e.IDParticipante] {
				return erroRequisicao(fmt.Sprintf("Posição %d: o usuário %d não é um atleta presente neste desafio", i+1, e.IDParticipante))
			}
			if vistos[e.IDParticipante] {
				return erroRequisicao(fmt.Sprintf("Posição %d: o atleta %d já aparece em outra posição", i+1, e.IDParticipante))
			}
			vistos[e.IDParticipante] = true
			if m := strings.TrimSpace(e.Marca); m == "" || utf8.RuneCountInString(m) > models.MarcaMaxCaracteres {
				return erroRequisicao(fmt.Sprintf("Posição %d: marca obrigatória (até %d caracteres)", i+1, models.MarcaMaxCaracteres))
			}
		}

		agora := s.agora().Truncate(time.Millisecond)
		mudou, err := repo.MudarStatus(d.ID, models.StatusEmAndamento, models.StatusEncerrado,
			map[string]any{"data_encerramento": agora})
		if err != nil {
			return err
		}
		if !mudou {
			return erroConflito("O pódio deste desafio já foi lançado")
		}
		// podio_lancado_* são somente leitura no model (o GORM os ignora em Updates): SQL direto.
		if err := tx.Exec("UPDATE desafios SET podio_lancado_por = ?, podio_lancado_em = ? WHERE id = ?", userID, agora, d.ID).Error; err != nil {
			return err
		}
		d.PodioLancadoEm = &agora
		if relancamento {
			if _, err := s.desafioMod.LimparAnulacaoNoRelancamento(tx, d.ID); err != nil {
				return err
			}
		}

		dist, err := s.calcularDistribuicao(d, len(participantes), esperado)
		if err != nil {
			return err
		}
		premios := dist.Premios

		type resultado struct {
			idUsuario uint
			posicao   int // 0 = presente fora do pódio
			premio    int64
		}
		resultados := make([]resultado, 0, len(presentes))
		for i, e := range req.Posicoes {
			if err := repo.InserirPodio(&models.PodioDesafio{
				IDDesafio: d.ID, Posicao: i + 1, IDParticipante: e.IDParticipante,
				Marca: strings.TrimSpace(e.Marca), PremioCentavos: premios[i],
			}); err != nil {
				return err
			}
			resultados = append(resultados, resultado{e.IDParticipante, i + 1, premios[i]})
		}
		for _, p := range participantes {
			if presentes[p.IDUsuario] && !vistos[p.IDUsuario] {
				resultados = append(resultados, resultado{idUsuario: p.IDUsuario})
			}
		}
		sort.Slice(resultados, func(i, j int) bool { return resultados[i].idUsuario < resultados[j].idUsuario })

		// Créditos do encerramento (atletas, instrutor, academia, taxa da plataforma): NÃO entram no
		// ledger agora. Viram pagamentos_pendentes com liberar_em = podio_lancado_em + janela de
		// contestação, na mesma transação do CAS de status; o job liberar-pagamentos os credita depois.
		// Com denúncia de desafio já aberta, nascem retidos.
		liberarEm := agora.Add(s.janela)
		retentora, err := s.denuncias.RetentoraAberta(tx, d.ID, 0)
		if err != nil {
			return err
		}
		pendente := func(idUsuario *uint, tipo string, valor int64) models.PagamentoPendente {
			p := models.PagamentoPendente{IDDesafio: d.ID, IDUsuario: idUsuario, Tipo: tipo, ValorCentavos: valor,
				Status: models.PendenteALiberar, LiberarEm: liberarEm}
			if retentora != nil {
				p.Status, p.IDDenunciaRetencao = models.PendenteRetido, retentora
			}
			return p
		}
		var lote []models.PagamentoPendente
		for _, r := range resultados {
			if r.posicao > 0 {
				if err := repo.RegistrarPremio(d.ID, r.idUsuario, r.premio, r.posicao == 1); err != nil {
					return err
				}
				if r.premio > 0 {
					uid := r.idUsuario
					lote = append(lote, pendente(&uid, models.PendentePremio, r.premio))
				}
			}
		}
		var instrutorUser uint
		if fin := dist.Financeiro; fin.InstrutorTotal > 0 {
			if d.IDInstrutor == nil {
				return fmt.Errorf("desafio %d com pagamento de instrutor e sem instrutor", d.ID)
			}
			i, err := s.instrutors.BuscarPorID(*d.IDInstrutor)
			if err != nil {
				return err
			}
			if i == nil {
				return fmt.Errorf("instrutor %d do desafio %d não encontrado", *d.IDInstrutor, d.ID)
			}
			instrutorUser = i.IDUsuario
			lote = append(lote, pendente(&instrutorUser, models.PendentePagamentoInstrutor, fin.InstrutorTotal))
		}
		if dist.Financeiro.Academia > 0 {
			academia := d.IDCriador
			lote = append(lote, pendente(&academia, models.PendenteReceitaAcademia, dist.Financeiro.Academia))
		}
		if dist.Financeiro.Taxa > 0 {
			lote = append(lote, pendente(nil, models.PendenteTaxaInscricao, dist.Financeiro.Taxa))
		}
		if err := s.pendentes.CriarLote(tx, lote); err != nil {
			return err
		}
		// Pontos e Elo (o Elo exibido é sempre recalculado a partir de Pontos na borda da API). O delta de
		// cada um fica no evento podio_lancado: a anulação do resultado desfaz exatamente esses pontos.
		pontosDe := map[string]int{}
		for _, r := range resultados {
			delta := pontosPresente
			if r.posicao > 0 {
				delta = PontosPorColocacao(r.posicao)
			}
			if err := s.usuarios.IncrementarPontos(tx, r.idUsuario, delta); err != nil {
				return err
			}
			pontosDe[fmt.Sprint(r.idUsuario)] = delta
		}
		if err := s.registrarPodioLancado(tx, ctx, d, req, premios, lote, pontosDe, relancamento); err != nil {
			return err
		}
		academiaUser, pagoInstrutor, pagoAcademia := d.IDCriador, dist.Financeiro.InstrutorTotal, dist.Financeiro.Academia

		titulo, idDesafio := d.Titulo, d.ID
		posicaoDe := map[uint]resultado{}
		for _, r := range resultados {
			posicaoDe[r.idUsuario] = r
		}
		ids := idsExceto(participantes, 0)
		quando := "após a janela de contestação, em " + liberarEm.Local().Format("02/01/2006 15:04")
		if retentora != nil {
			quando = "depois da análise de uma contestação aberta sobre o desafio"
		}
		ev.add(func() {
			for _, uid := range ids {
				r, presente := posicaoDe[uid]
				switch {
				case presente && r.posicao > 0 && r.premio > 0:
					s.notif.Notificar(uid, models.NotifResultado, "Resultado do desafio",
						fmt.Sprintf("Você ficou em %dº lugar em \"%s\" e receberá %s %s.", r.posicao, titulo, reaisBR(r.premio), quando), linkDesafio(idDesafio))
				case presente && r.posicao > 0:
					s.notif.Notificar(uid, models.NotifResultado, "Resultado do desafio",
						fmt.Sprintf("Você ficou em %dº lugar em \"%s\"!", r.posicao, titulo), linkDesafio(idDesafio))
				default:
					s.notif.Notificar(uid, models.NotifResultado, "Resultado do desafio",
						fmt.Sprintf("O desafio \"%s\" foi encerrado. Veja o resultado.", titulo), linkDesafio(idDesafio))
				}
			}
			if instrutorUser != 0 {
				s.notif.Notificar(instrutorUser, models.NotifPagamentoInstrutor, "Pagamento a liberar",
					fmt.Sprintf("Você receberá %s pelo desafio \"%s\" %s.", reaisBR(pagoInstrutor), titulo, quando), linkDesafio(idDesafio))
			}
			if pagoAcademia > 0 {
				s.notif.Notificar(academiaUser, models.NotifRepasseAcademia, "Desafio encerrado",
					fmt.Sprintf("O desafio \"%s\" foi encerrado e %s voltarão para o seu saldo %s.", titulo, reaisBR(pagoAcademia), quando), linkDesafio(idDesafio))
			}
		})
		return nil
	})
	if repositories.EhDuplicata(err) {
		// UNIQUE do pódio ou do ledger (uq_transacoes_credito_desafio): outro lançamento venceu
		return nil, erroConflito("O pódio deste desafio já foi lançado")
	}
	if err != nil {
		return nil, err
	}
	return s.Detalhe(userID, id)
}

// distribuicao é o resultado financeiro do encerramento de um desafio.
type distribuicao struct {
	// PremioTotal é o valor a repartir entre as posições do pódio (centavos): prêmio fixo + parcela das
	// inscrições. Nunca é reduzido pela taxa da plataforma.
	PremioTotal int64
	// Premios é o prêmio de cada posição (índice 0 = 1º), já dividido por DividirPremio.
	Premios []int64
	// Financeiro é a conta completa: arrecadação, taxa, instrutor e parte da academia.
	Financeiro ResultadoFinanceiro
}

// calcularDistribuicao é o ÚNICO ponto que decide quanto vai para cada parte ao lançar o pódio.
// inscritos = todos os inscritos que restaram (presentes ou não); presentesNoPodio = posições
// efetivamente premiadas. Garante a invariante entradas + reserva = prêmios + instrutor + academia + taxa.
func (s *desafioService) calcularDistribuicao(d *models.Desafio, inscritos, presentesNoPodio int) (distribuicao, error) {
	fin, err := CalcularFinanceiro(paramsDoDesafio(d, inscritos))
	if err != nil {
		return distribuicao{}, fmt.Errorf("desafio %d: %w", d.ID, err)
	}
	premios := DividirPremio(fin.PremioTotal, d.PosicoesPodio, presentesNoPodio)
	var soma int64
	for _, p := range premios {
		soma += p
	}
	if soma != fin.PremioTotal || fin.Arrecadacao+d.ReservaAcademiaCentavos != soma+fin.InstrutorTotal+fin.Academia+fin.Taxa {
		return distribuicao{}, fmt.Errorf("desafio %d: conta não fecha (prêmios %d, instrutor %d, academia %d, taxa %d)",
			d.ID, soma, fin.InstrutorTotal, fin.Academia, fin.Taxa)
	}
	return distribuicao{PremioTotal: fin.PremioTotal, Premios: premios, Financeiro: fin}, nil
}

// Podio devolve o pódio do desafio (lista vazia enquanto não encerrado). Mesma visibilidade do detalhe.
func (s *desafioService) Podio(userID, id uint) ([]models.PodioResponse, error) {
	if _, err := s.Detalhe(userID, id); err != nil {
		return nil, err
	}
	return s.podioResponse(id)
}

func (s *desafioService) podioResponse(id uint) ([]models.PodioResponse, error) {
	lista, err := s.desafios.ListarPodio(id)
	if err != nil {
		return nil, err
	}
	out := make([]models.PodioResponse, 0, len(lista))
	for _, p := range lista {
		r := models.PodioResponse{
			Posicao: p.Posicao, IDParticipante: p.IDParticipante, Marca: p.Marca,
			PremioCentavos: p.PremioCentavos, Premio: utils.CentavosParaReais(p.PremioCentavos),
		}
		if p.Participante != nil {
			r.Nome, r.Sobrenome = p.Participante.Nome, p.Participante.Sobrenome
		}
		out = append(out, r)
	}
	return out, nil
}

// Avaliar registra a nota (1 a 5) de quem esteve presente num desafio encerrado. Uma por atleta.
func (s *desafioService) Avaliar(userID, id uint, nota int) (*models.AvaliacaoDesafio, error) {
	if nota < 1 || nota > 5 {
		return nil, erroRequisicao("Nota deve estar entre 1 e 5")
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	p, err := s.desafios.BuscarParticipante(id, userID)
	if err != nil {
		return nil, err
	}
	if p == nil || !p.Presente {
		return nil, erroProibido("Apenas atletas que estiveram presentes podem avaliar o desafio")
	}
	if d.Status != models.StatusEncerrado {
		return nil, erroConflito("O desafio só pode ser avaliado depois de encerrado")
	}
	a := &models.AvaliacaoDesafio{IDDesafio: id, IDUsuario: userID, Nota: nota}
	if err := s.avaliacoes.Criar(a); err != nil {
		if repositories.EhDuplicata(err) {
			return nil, erroConflito("Você já avaliou este desafio")
		}
		return nil, err
	}
	return a, nil
}

// Convidar (atleta, desafio aberto) valida que o alvo é amigo aceito e ainda não participa, e
// envia a notificação in-app para ele se inscrever.
func (s *desafioService) Convidar(userID, id, idAmigo uint) error {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return err
	}
	if err := exigirAtleta(ctx); err != nil {
		return err
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return err
	}
	if d == nil {
		return erroNaoEncontrado("Desafio não encontrado")
	}
	if d.Status != models.StatusAberto {
		return erroConflito("Só é possível convidar enquanto o desafio está aberto")
	}
	if idAmigo == userID {
		return erroRequisicao("Você não pode convidar a si mesmo")
	}
	ehAmigo, err := s.amizades.SaoAmigos(userID, idAmigo)
	if err != nil {
		return err
	}
	if !ehAmigo {
		return erroRequisicao("Você só pode convidar amigos")
	}
	if ja, err := s.desafios.UsuarioParticipa(id, idAmigo); err != nil {
		return err
	} else if ja {
		return erroConflito("Este amigo já participa do desafio")
	}
	nome := "Um amigo"
	if u, err := s.usuarios.BuscarPorID(userID); err == nil && u != nil {
		nome = strings.TrimSpace(u.Nome + " " + u.Sobrenome)
	}
	s.notif.Notificar(idAmigo, models.NotifConviteDesafio, "Convite para desafio",
		fmt.Sprintf("%s convidou você para o desafio \"%s\" (entrada %s).", nome, d.Titulo, reaisBR(d.ValorEntradaCentavos)),
		linkDesafio(d.ID))
	return nil
}

// ─── Leitura ─────────────────────────────────────────────────────────────────

func normalizarPagina(pagina, limite int) (int, int) {
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

func (s *desafioService) listar(userID uint, f FiltroListagem, meus bool) (*models.ListaDesafiosResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	pagina, limite := normalizarPagina(f.Pagina, f.Limite)
	filtro := repositories.FiltroDesafios{
		IDUsuario: userID, Status: f.Status, SomenteMeus: meus, Categoria: f.Categoria, Busca: f.Busca,
		IDAcademia: f.IDAcademia, Agora: s.agora(), Pagina: pagina, Limite: limite,
	}
	if meus {
		filtro.Ordem = repositories.OrdemEventoRec
	} else if f.Status == "" || f.Status == models.StatusAberto {
		// Feed: por padrão só abertos com data do evento futura, do mais próximo para o mais distante.
		filtro.Status = models.StatusAberto
		filtro.Futuros = true
		filtro.Ordem = repositories.OrdemEventoProx
	} else {
		filtro.Ordem = repositories.OrdemEventoRec
	}
	desafios, total, err := s.desafios.Listar(filtro)
	if err != nil {
		return nil, err
	}
	avaliados, err := s.avaliadosDe(userID, desafios)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*models.Desafio, len(desafios))
	for i := range desafios {
		ptrs[i] = &desafios[i]
	}
	if err := s.enriquecerVagas(ctx, ptrs); err != nil {
		return nil, err
	}
	itens := make([]models.DesafioResponse, 0, len(desafios))
	for i := range desafios {
		itens = append(itens, *s.montarResposta(&desafios[i], ctx, avaliados[desafios[i].ID]))
	}
	return &models.ListaDesafiosResponse{Itens: itens, Total: total, Pagina: pagina, Limite: limite}, nil
}

// avaliadosDe consulta (em lote) quais desafios encerrados em que o usuário esteve presente ele já avaliou.
func (s *desafioService) avaliadosDe(userID uint, desafios []models.Desafio) (map[uint]bool, error) {
	var ids []uint
	for i := range desafios {
		if desafios[i].Status == models.StatusEncerrado && desafios[i].MinhaParticipacao != nil {
			ids = append(ids, desafios[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return s.desafios.Avaliados(userID, ids)
}

// Listar devolve o feed: desafios abertos e futuros (padrão) ou, com status, os desse status em que o
// usuário está envolvido. Paginado e filtrado no SQL.
func (s *desafioService) Listar(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error) {
	return s.listar(userID, f, false)
}

// ListarMeus devolve os desafios em que o usuário participa (atleta), é a academia dona ou o instrutor
// designado (qualquer status), com o ingresso e o status.
func (s *desafioService) ListarMeus(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error) {
	return s.listar(userID, f, true)
}

// Detalhe devolve o desafio completo. Desafios não abertos só são visíveis a quem participa, à
// academia dona e ao instrutor designado.
func (s *desafioService) Detalhe(userID, id uint) (*models.DesafioResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	d.QtdParticipantes = len(d.Participantes)
	for i := range d.Participantes {
		p := &d.Participantes[i]
		if p.Presente {
			d.QtdPresentes++
		}
		if p.IDUsuario == userID {
			d.MinhaParticipacao = p
		}
	}
	if d.Status != models.StatusAberto && d.MinhaParticipacao == nil && !ctx.staff(d) {
		return nil, erroProibido("Você não participa deste desafio")
	}
	if err := s.enriquecerVagas(ctx, []*models.Desafio{d}); err != nil {
		return nil, err
	}
	var avaliou bool
	if d.Status == models.StatusEncerrado && d.MinhaParticipacao != nil {
		av, err := s.desafios.Avaliados(userID, []uint{d.ID})
		if err != nil {
			return nil, err
		}
		avaliou = av[d.ID]
	}
	resp := s.montarResposta(d, ctx, avaliou)
	for i := range d.Participantes {
		resp.Participantes = append(resp.Participantes, paraParticipanteResponse(&d.Participantes[i]))
	}
	if d.Status == models.StatusEncerrado {
		if resp.Podio, err = s.podioResponse(d.ID); err != nil {
			return nil, err
		}
	}
	if err := s.completarLiberacao(resp, d); err != nil {
		return nil, err
	}
	return resp, nil
}

func paraParticipanteResponse(p *models.DesafioParticipante) models.ParticipanteResponse {
	pr := models.ParticipanteResponse{
		Presente:       p.Presente,
		PresenteEm:     p.PresenteEm,
		PresencaModo:   p.PresencaModo,
		PremioCentavos: p.PremioCentavos,
		Premio:         utils.CentavosParaReais(p.PremioCentavos),
		Vencedor:       p.Vencedor,
	}
	if p.Usuario != nil {
		pr.UsuarioPublico = p.Usuario.Publico()
	} else {
		pr.ID = p.IDUsuario
	}
	return pr
}

// montarResposta converte o model no DTO e calcula as flags de ação para o usuário do contexto.
// Participantes e Podio ficam vazios (o detalhe os preenche).
func (s *desafioService) montarResposta(d *models.Desafio, ctx *contextoUsuario, avaliou bool) *models.DesafioResponse {
	return montarResposta(d, ctx, avaliou, s.agora())
}

func montarResposta(d *models.Desafio, ctx *contextoUsuario, avaliou bool, agora time.Time) *models.DesafioResponse {
	poteCentavos := d.ValorEntradaCentavos * int64(d.QtdParticipantes)
	vagaAberta := d.InstrutorSelecao == models.SelecaoVagaAberta && d.IDInstrutor == nil
	// Previsão sobre as inscrições atuais. Com a vaga ainda aberta, a parte fixa do instrutor é o teto
	// (pior caso, igual ao que está reservado).
	pf := paramsDoDesafio(d, d.QtdParticipantes)
	if vagaAberta && d.InstrutorTetoCentavos != nil {
		pf.InstrutorFixo = *d.InstrutorTetoCentavos
	}
	fin, errFin := CalcularFinanceiro(pf)
	if errFin != nil { // configuração inconsistente (não deveria existir): mostra só o que dá para afirmar
		fin = ResultadoFinanceiro{Arrecadacao: poteCentavos, PremioTotal: d.PremioFixoCentavos}
	}
	previsao := DividirPremio(fin.PremioTotal, d.PosicoesPodio, d.PosicoesPodio)
	if previsao == nil {
		previsao = []int64{}
	}
	resp := &models.DesafioResponse{
		ID:                        d.ID,
		Titulo:                    d.Titulo,
		Categoria:                 d.Categoria,
		ValorEntradaCentavos:      d.ValorEntradaCentavos,
		ValorEntrada:              utils.CentavosParaReais(d.ValorEntradaCentavos),
		PoteCentavos:              poteCentavos,
		Pote:                      utils.CentavosParaReais(poteCentavos),
		Latitude:                  d.Latitude,
		Longitude:                 d.Longitude,
		DataEvento:                d.DataEvento,
		Status:                    d.Status,
		Vagas:                     d.Vagas,
		PosicoesPodio:             d.PosicoesPodio,
		PremiacaoPrevistaCentavos: previsao,
		Premiacao: models.PremiacaoResponse{
			PremioTotalCentavos: fin.PremioTotal, PremioTotal: utils.CentavosParaReais(fin.PremioTotal), PorPosicaoCentavos: previsao,
		},
		InstrutorSelecao:    d.InstrutorSelecao,
		VagaInstrutorAberta: vagaAberta,
		ParticipantesCount:  d.QtdParticipantes,
		PresentesCount:      d.QtdPresentes,
		CriadoEm:            d.CriadoEm,
		Participantes:       []models.ParticipanteResponse{},
		Podio:               []models.PodioResponse{},
	}
	if d.Descricao != nil {
		resp.Descricao = *d.Descricao
	}
	if d.Local != nil {
		resp.Local = *d.Local
	}
	if d.Academia != nil {
		resp.Academia = &models.AcademiaResumo{ID: d.Academia.ID, NomeFantasia: d.Academia.NomeFantasia}
	}
	if d.Instrutor != nil {
		nome := ""
		if d.Instrutor.Usuario != nil {
			nome = strings.TrimSpace(d.Instrutor.Usuario.Nome + " " + d.Instrutor.Usuario.Sobrenome)
		}
		resp.Instrutor = &models.InstrutorResumo{ID: d.Instrutor.ID, IDUsuario: d.Instrutor.IDUsuario, Nome: nome}
	}

	aberto := d.Status == models.StatusAberto
	emAndamento := d.Status == models.StatusEmAndamento
	participa := d.MinhaParticipacao != nil
	if participa {
		resp.VoceParticipa = true
		resp.VocePresente = d.MinhaParticipacao.Presente
		resp.SeuCodigoIngresso = d.MinhaParticipacao.CodigoIngresso
	}
	if ctx == nil {
		estadoResultado(resp, d, nil)
		return resp
	}
	defer estadoResultado(resp, d, ctx)
	atleta := ctx.Perfil == "atleta"
	dono, staff := ctx.dono(d), ctx.staff(d)
	eventoPassou := d.DataEvento != nil && !d.DataEvento.After(agora)

	resp.PodeEntrar = atleta && aberto && !participa && d.QtdParticipantes < d.Vagas && !eventoPassou
	resp.PodeSair = atleta && aberto && participa
	resp.PodeIniciar = staff && aberto && d.QtdParticipantes > 0 && d.IDInstrutor != nil
	resp.PodeMarcarPresenca = staff && d.QtdParticipantes > 0 &&
		(emAndamento || (aberto && (d.DataEvento == nil || mesmoDia(*d.DataEvento, agora))))
	resp.PodeLancarPodio = staff && emAndamento && d.QtdPresentes > 0
	resp.PodeCancelar = dono && (aberto || emAndamento)
	resp.PodeEditar = dono && aberto
	resp.PodeAvaliar = participa && resp.VocePresente && d.Status == models.StatusEncerrado && !avaliou

	// Visões financeiras por perfil: só a academia dona vê taxa/instrutor/líquido/reserva; o instrutor
	// designado vê o que vai ganhar; instrutores veem a oferta da vaga aberta. Atletas e demais
	// só têm `premiacao`.
	if dono {
		resp.Financeiro = &models.FinanceiroResponse{
			ArrecadacaoCentavos: fin.Arrecadacao, PremioFixoCentavos: d.PremioFixoCentavos,
			DividirPote: d.DividirPote, PercentualPote: d.PercentualPote, PotePremioCentavos: fin.PotePremio,
			PremioTotalCentavos: fin.PremioTotal, TaxaPlataformaPct: d.TaxaPlataformaPct, TaxaPlataformaCentavos: fin.Taxa,
			InstrutorSelecao: d.InstrutorSelecao, InstrutorModoPagamento: d.InstrutorModoPagamento,
			InstrutorFixoCentavos: fin.InstrutorFixo, InstrutorPct: d.InstrutorPct, InstrutorPctCentavos: fin.InstrutorPct,
			InstrutorTotalCentavos: fin.InstrutorTotal, InstrutorTetoCentavos: d.InstrutorTetoCentavos,
			LiquidoPrevistoCentavos: fin.Academia, ResultadoPrevistoCentavos: fin.Academia - d.ReservaAcademiaCentavos,
			ReservaCentavos: d.ReservaAcademiaCentavos, CandidaturasPendentes: d.QtdCandidaturas,
		}
		resp.PodeEscolherInstrutor = aberto && vagaAberta
	}
	if ctx.instrutorDe(d) {
		resp.InstrutorGanho = &models.GanhoInstrutorResponse{
			ModoPagamento: d.InstrutorModoPagamento, FixoCentavos: fin.InstrutorFixo, Pct: d.InstrutorPct,
			PctCentavos: fin.InstrutorPct, TotalPrevistoCentavos: fin.InstrutorTotal,
		}
	}
	if ctx.InstrutorID != 0 && d.InstrutorSelecao == models.SelecaoVagaAberta && d.InstrutorTetoCentavos != nil &&
		(vagaAberta || d.MinhaCandidatura != nil) {
		resp.Vaga = &models.VagaInstrutorResponse{
			TetoCentavos: *d.InstrutorTetoCentavos, ModoPagamento: d.InstrutorModoPagamento, Pct: d.InstrutorPct,
		}
		if d.MinhaCandidatura != nil {
			c := paraCandidaturaResponse(d.MinhaCandidatura, nil)
			resp.Vaga.MinhaCandidatura = &c
		}
		st := ""
		if d.MinhaCandidatura != nil {
			st = d.MinhaCandidatura.Status
		}
		resp.PodeCandidatar = aberto && vagaAberta && !eventoPassou &&
			(st == "" || st == models.CandidaturaPendente || st == models.CandidaturaRetirada)
	}
	return resp
}
