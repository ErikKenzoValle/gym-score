package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
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
// Fluxo: a academia cria o desafio (endereço, início e fim) e monta a equipe de instrutores (indicados
// e/ou candidaturas aceitas, com 1 responsável); o atleta se inscreve pagando a entrada; qualquer membro
// da equipe (ou a academia) confirma presença lendo o QR do ingresso; o responsável (ou a academia)
// inicia o desafio e lança o pódio, que divide o pote, credita os prêmios e encerra.
type DesafioService interface {
	// ComModo devolve o serviço atuando no modo da sessão (claim `modo` do JWT: "atleta" | "instrutor";
	// "" = atleta). O modo só é a escolha do usuário: o contexto reconfere no banco (instrutor aprovado).
	ComModo(modo string) DesafioService
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

	// Equipe de instrutores (rodada 5): candidatura sem lance e gestão da equipe — ver candidatura_service.go.
	Candidatar(userID, id uint, req *models.CandidatarRequest) (*models.CandidaturaResponse, bool, error)
	RetirarCandidatura(userID, id uint) (*models.CandidaturaResponse, error)
	ListarCandidaturas(userID, id uint) ([]models.CandidaturaResponse, error)
	AceitarCandidatura(userID, id, idCandidatura uint) (*models.DesafioResponse, error)
	RecusarCandidatura(userID, id, idCandidatura uint) (*models.CandidaturaResponse, error)
	ListarVagas(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error)
	IndicarInstrutor(userID, id, idInstrutor uint) (*models.DesafioResponse, error)
	RemoverDaEquipe(userID, id, idInstrutor uint) (*models.DesafioResponse, error)
	DefinirResponsavel(userID, id, idInstrutor uint) (*models.DesafioResponse, error)

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

	// modo é o modo da sessão de quem age (ver ComModo); "" = atleta.
	modo string
}

// ComModo devolve uma cópia rasa do serviço (mesmas dependências) atuando no modo informado.
func (s *desafioService) ComModo(modo string) DesafioService {
	c := *s
	c.modo = modo
	return &c
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

// contextoUsuario é o papel de quem age: tipo de conta e pedido de instrutor vêm do banco; o modo
// (atleta | instrutor) vem do token e só vale se o banco confirmar (instrutor aprovado).
//   - Perfil "academia": conta de academia (AcademiaID/AcademiaAprovada).
//   - Perfil "atleta": conta de atleta no modo atleta (InstrutorID fica 0: não opera como instrutor).
//   - Perfil "instrutor": conta de atleta no modo instrutor com o pedido aprovado (InstrutorID).
type contextoUsuario struct {
	ID                uint
	Perfil            string
	AcademiaID        uint
	InstrutorID       uint
	AcademiaAprovada  bool
	InstrutorAprovado bool
	// SuspensoAte: fim da suspensão global vigente (nil = não suspenso). Ver contextoAtivo (Q2).
	SuspensoAte *time.Time
	// conflito: o registro de instrutor da conta no MODO ATLETA (nil sem pedido), só para calcular
	// `pode_entrar`/`motivo_bloqueio_entrada` com a mesma regra de exigirSemConflitoComoAtleta (B4-8).
	conflito *conflitoAtleta
	// inscricoes: inscrições ativas do atleta com horário (modo atleta), para prever o choque (T6).
	inscricoes []repositories.InscricaoNoHorario
	// elegibilidade: nível GymScore (cache) e data de nascimento do atleta (modo atleta), para prever os
	// filtros de nível/idade (U4). nil = não conferir (ex.: visões sem atleta).
	elegibilidade *PerfilElegibilidade
}

// conflitoAtleta é o que a regra de conflito de interesse precisa saber da conta (modo atleta).
type conflitoAtleta struct {
	InstrutorID uint
	Aprovado    bool
	AcademiaID  uint          // academia vinculada (0 = autônomo)
	Candidatura map[uint]bool // desafios com candidatura pendente/escolhida (só instrutor aprovado)
	Equipe      map[uint]bool // desafios em que está na equipe de instrutores (qualquer status do pedido)
}

// motivoConflitoEntrada devolve a mensagem do 409 que `Entrar` daria por conflito de interesse ("" = sem
// conflito). Mesma regra e mesmas mensagens de exigirSemConflitoComoAtleta (que continua sendo a garantia,
// sob trava; isto é só a previsão para a tela).
func motivoConflitoEntrada(c *conflitoAtleta, d *models.Desafio) string {
	if c == nil {
		return ""
	}
	if c.Equipe[d.ID] || d.Membro(c.InstrutorID) != nil {
		return msgConflitoDesignado
	}
	if !c.Aprovado {
		return ""
	}
	if d.IDAcademia != nil && c.AcademiaID != 0 && c.AcademiaID == *d.IDAcademia {
		return msgConflitoVinculado
	}
	if c.Candidatura[d.ID] {
		return msgConflitoCandidatura
	}
	return ""
}

// Mensagens do conflito de interesse na inscrição (409 de Entrar e motivo_bloqueio_entrada).
const (
	msgConflitoDesignado   = "Você está na equipe de instrutores deste desafio e não pode se inscrever como atleta"
	msgConflitoVinculado   = "Você é instrutor vinculado à academia deste desafio e não pode se inscrever como atleta"
	msgConflitoCandidatura = "Você se candidatou à equipe de instrutores deste desafio: retire a candidatura antes de se inscrever como atleta"
)

// msgChoqueHorario é o 409 / motivo_bloqueio_entrada do choque de horário (T6).
func msgChoqueHorario(titulo string) string {
	return fmt.Sprintf("Você já está inscrito em \"%s\" no mesmo horário", titulo)
}

// choqueDeHorario devolve a inscrição ativa que choca com o desafio d: outro desafio da MESMA academia,
// com intervalos [início, fim) sobrepostos. Desafio sem academia, sem início ou sem fim nunca choca.
func choqueDeHorario(inscricoes []repositories.InscricaoNoHorario, d *models.Desafio) *repositories.InscricaoNoHorario {
	if d.IDAcademia == nil || d.DataEvento == nil || d.DataFimEvento == nil {
		return nil
	}
	for i := range inscricoes {
		o := &inscricoes[i]
		if o.IDDesafio == d.ID || o.IDAcademia != *d.IDAcademia {
			continue
		}
		if o.DataEvento.Before(*d.DataFimEvento) && o.DataFimEvento.After(*d.DataEvento) {
			return o
		}
	}
	return nil
}

// carregarConflito lê o pedido de instrutor da conta (1 consulta) e, se aprovado, os desafios com
// candidatura ativa (mais 1). Usado só no modo atleta.
func (s *desafioService) carregarConflito(userID uint) (*conflitoAtleta, error) {
	inst, err := s.instrutors.BuscarPorUsuario(userID)
	if err != nil || inst == nil {
		return nil, err
	}
	c := &conflitoAtleta{InstrutorID: inst.ID, Aprovado: inst.Status == models.CadastroAprovado}
	if inst.IDAcademia != nil {
		c.AcademiaID = *inst.IDAcademia
	}
	equipe, err := s.desafios.DesafiosDaEquipe(inst.ID)
	if err != nil {
		return nil, err
	}
	c.Equipe = make(map[uint]bool, len(equipe))
	for _, id := range equipe {
		c.Equipe[id] = true
	}
	if !c.Aprovado {
		return c, nil
	}
	var ids []uint
	if err := s.db.Model(&models.CandidaturaInstrutor{}).Where("id_instrutor = ? AND status IN ?", inst.ID,
		[]string{models.CandidaturaPendente, models.CandidaturaEscolhida}).Pluck("id_desafio", &ids).Error; err != nil {
		return nil, err
	}
	c.Candidatura = make(map[uint]bool, len(ids))
	for _, id := range ids {
		c.Candidatura[id] = true
	}
	return c, nil
}

// contextoAtivo é o contexto das ações de participação/gestão: recusa a conta suspensa (403
// conta_suspensa com o fim — Q2). Leituras usam contexto() e continuam liberadas ao suspenso.
// Punições são da CONTA: valem igualmente nos dois modos.
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
	case models.PerfilAcademia:
		a, err := s.academias.BuscarPorUsuario(userID)
		if err != nil {
			return nil, err
		}
		if a != nil {
			c.AcademiaID = a.ID
			c.AcademiaAprovada = a.Status == models.CadastroAprovado
		}
	case models.PerfilAtleta:
		if s.modo != models.ModoInstrutor {
			c.Perfil = models.ModoAtleta
			cf, err := s.carregarConflito(userID)
			if err != nil {
				return nil, err
			}
			c.conflito = cf
			if c.inscricoes, err = s.desafios.InscricoesAtivasComHorario(userID); err != nil {
				return nil, err
			}
			c.elegibilidade = &PerfilElegibilidade{Nivel: u.NivelGymScore, DataNascimento: u.DataNascimento}
			if u.GymScoreCalculadoEm == nil { // nunca calculado (conta antiga): calcula agora e grava
				if err := RecalcularGymScore(s.db, []uint{userID}, s.agora()); err != nil {
					return nil, err
				}
				if p, err := repositories.BuscarPerfilGymScore(s.db, userID); err == nil && p != nil {
					c.elegibilidade.Nivel = p.NivelGymScore
				}
			}
			return c, nil
		}
		// Modo instrutor: o claim só registra a escolha; o pedido tem de estar aprovado AGORA (rejeitado ou
		// revogado depois da troca de modo = 403 modo_indisponivel em toda rota, inclusive leitura).
		inst, err := s.instrutors.BuscarPorUsuario(userID)
		if err != nil {
			return nil, err
		}
		if inst == nil || inst.Status != models.CadastroAprovado {
			p := &models.PapeisUsuario{IDUsuario: userID, Conta: u.Perfil}
			if inst != nil {
				p.InstrutorID, p.InstrutorStatus = inst.ID, inst.Status
			}
			return nil, ErroModoIndisponivel(p)
		}
		c.Perfil = models.ModoInstrutor
		c.InstrutorID = inst.ID
		c.InstrutorAprovado = true
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

// instrutorDe: é membro da equipe de instrutores do desafio (só no modo instrutor). Exige d.Equipe.
func (c *contextoUsuario) instrutorDe(d *models.Desafio) bool {
	return c.InstrutorID != 0 && c.InstrutorAprovado && d.Membro(c.InstrutorID) != nil
}

// responsavelDe: é o responsável da equipe (lança o pódio e inicia). Exige d.Equipe.
func (c *contextoUsuario) responsavelDe(d *models.Desafio) bool {
	m := d.Membro(c.InstrutorID)
	return c.InstrutorID != 0 && c.InstrutorAprovado && m != nil && m.Responsavel
}

// staff: pode operar o desafio no dia (presença): academia dona ou qualquer membro da equipe.
func (c *contextoUsuario) staff(d *models.Desafio) bool { return c.dono(d) || c.instrutorDe(d) }

// comando: pode iniciar e lançar o pódio: academia dona ou o RESPONSÁVEL da equipe.
func (c *contextoUsuario) comando(d *models.Desafio) bool { return c.dono(d) || c.responsavelDe(d) }

func exigirStaff(c *contextoUsuario, d *models.Desafio) error {
	if !c.staff(d) {
		if c.Perfil == models.ModoAtleta && len(d.Equipe) > 0 {
			// Pode ser um instrutor da equipe agindo no modo atleta: a mensagem orienta a troca.
			return erroProibido("Apenas a equipe de instrutores ou a academia dona podem realizar esta ação (instrutor: troque para o modo instrutor)")
		}
		return erroProibido("Apenas a equipe de instrutores ou a academia dona podem realizar esta ação")
	}
	return nil
}

// exigirComando: iniciar e lançar o pódio são da academia dona ou do responsável da equipe.
func exigirComando(c *contextoUsuario, d *models.Desafio) error {
	if c.comando(d) {
		return nil
	}
	if c.instrutorDe(d) {
		return erroProibido("Apenas o instrutor responsável ou a academia dona podem realizar esta ação")
	}
	return exigirStaff(c, d)
}

func exigirDono(c *contextoUsuario, d *models.Desafio) error {
	if !c.dono(d) {
		return erroProibido("Apenas a academia dona do desafio pode realizar esta ação")
	}
	return nil
}

// instrutorDaContaTravado lê (FOR SHARE) o registro de instrutor da conta, dentro da transação de
// inscrição e DEPOIS da trava do desafio. Serializa a inscrição com o aceite de convite de vínculo (que
// trava a mesma linha FOR UPDATE antes de conferir as inscrições na academia): ou o aceite vê a inscrição
// commitada, ou a inscrição vê o vínculo novo. nil = a conta nunca pediu o modo instrutor.
//
// Só trava quando a linha existe (trava de registro pela chave única): uma leitura travada de linha
// inexistente pegaria trava de intervalo no índice e entraria em deadlock com um pedido de instrutor
// simultâneo da mesma conta (INSERT em `instrutores` + checagem da FK em `usuarios` × débito da entrada).
// Sem linha não há conflito possível: um pedido criado agora nasce pendente e sem vínculo.
func instrutorDaContaTravado(tx *gorm.DB, userID uint) (*models.Instrutor, error) {
	var ids []uint
	if err := tx.Raw("SELECT id FROM instrutores WHERE id_usuario = ?", userID).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var is []models.Instrutor
	if err := tx.Raw("SELECT id, id_usuario, id_academia, status FROM instrutores WHERE id_usuario = ? FOR SHARE", userID).
		Scan(&is).Error; err != nil {
		return nil, err
	}
	if len(is) == 0 {
		return nil, nil
	}
	return &is[0], nil
}

// exigirSemConflitoComoAtleta aplica, na inscrição, a regra de conflito de interesse (rodada 4/5): o
// instrutor APROVADO continua competindo como atleta, exceto onde trabalha — não se inscreve em desafio
// em cuja equipe de instrutores está (qualquer membro, não só o responsável), em desafio da academia a
// que está vinculado, nem em desafio em que tem candidatura pendente/aceita. Pedido pendente/rejeitado não
// muda nada para o atleta. Chamado com o desafio travado (FOR UPDATE) e a equipe carregada: indicação,
// candidatura e aceite travam o mesmo desafio.
func (s *desafioService) exigirSemConflitoComoAtleta(tx *gorm.DB, userID uint, d *models.Desafio) error {
	inst, err := instrutorDaContaTravado(tx, userID)
	if err != nil || inst == nil {
		return err
	}
	if d.Membro(inst.ID) != nil {
		return erroConflito(msgConflitoDesignado)
	}
	if inst.Status != models.CadastroAprovado {
		return nil
	}
	if d.IDAcademia != nil && inst.IDAcademia != nil && *inst.IDAcademia == *d.IDAcademia {
		return erroConflito(msgConflitoVinculado)
	}
	c, err := s.candidaturas.ComTx(tx).BuscarDoInstrutor(d.ID, inst.ID)
	if err != nil {
		return err
	}
	if c != nil && (c.Status == models.CandidaturaPendente || c.Status == models.CandidaturaEscolhida) {
		return erroConflito(msgConflitoCandidatura)
	}
	return nil
}

// exigirInstrutorNaoInscrito aplica a direção inversa (designação, candidatura, escolha): o instrutor não
// pode ser designado nem se candidatar em desafio em que está inscrito como atleta. Desafio travado.
func exigirInstrutorNaoInscrito(repo *repositories.DesafioRepository, idUsuarioInstrutor uint, d *models.Desafio, msg string) error {
	inscrito, err := repo.UsuarioParticipa(d.ID, idUsuarioInstrutor)
	if err != nil {
		return err
	}
	if inscrito {
		return erroConflito(msg)
	}
	return nil
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
	switch c.Perfil {
	case models.ModoAtleta:
		return nil
	case models.ModoInstrutor:
		return ErroModoIncorreto(models.ModoAtleta)
	}
	return erroProibido("Apenas atletas podem realizar esta ação")
}

// exigirInstrutor: ação do modo instrutor (candidatura, vagas). Conta de atleta no modo atleta recebe
// 403 modo_incorreto; academia, 403 simples.
func exigirInstrutor(c *contextoUsuario, msg string) error {
	switch {
	case c.Perfil == models.ModoInstrutor && c.InstrutorID != 0:
		return nil
	case c.Perfil == models.ModoAtleta:
		return ErroModoIncorreto(models.ModoInstrutor)
	}
	return erroProibido(msg)
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

// msgPosicoesPodio é o 400 de posicoes_podio fora de 1..100 (rodada 6: "sem limite" prático).
const msgPosicoesPodio = "posicoes_podio deve estar entre 1 e 100"

// msgCheckDesafio é o 422 de um CHECK do banco (3819) na criação/edição do desafio.
const msgCheckDesafio = "Dados do desafio recusados pelo banco (endereço, horário, percentuais, premiação ou filtros). Confira e tente de novo"

// msgLeilaoRemovido é o 400 de quem ainda manda os campos do antigo leilão de instrutor.
const msgLeilaoRemovido = "O leilão de instrutor foi removido: defina instrutores_vagas, o pagamento por instrutor " +
	"(instrutor_modo_pagamento, instrutor_fixo_centavos, instrutor_pct) e, se quiser, os indicados — as vagas restantes ficam abertas à candidatura"

// intervaloEvento valida início (futuro) e fim (> início) do evento.
func (s *desafioService) intervaloEvento(inicioTxt, fimTxt string) (time.Time, time.Time, error) {
	inicio, err := parseDataEvento(inicioTxt)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !inicio.After(s.agora()) {
		return time.Time{}, time.Time{}, erroRequisicao("data_evento deve estar no futuro")
	}
	fim, err := parseDataEvento(fimTxt)
	if err != nil {
		return time.Time{}, time.Time{}, erroRequisicao("data_fim_evento inválida (use RFC3339 ou YYYY-MM-DDTHH:MM)")
	}
	if !fim.After(inicio) {
		return time.Time{}, time.Time{}, erroRequisicao("data_fim_evento deve ser depois de data_evento (início)")
	}
	return inicio, fim, nil
}

// aplicarEnderecoDesafio copia o endereço normalizado para o desafio (local = texto montado).
func aplicarEnderecoDesafio(d *models.Desafio, e *enderecoNormalizado) {
	local := e.Texto()
	d.Local = &local
	d.CEP, d.Logradouro, d.Numero, d.Bairro, d.Cidade, d.UF = &e.CEP, &e.Logradouro, &e.Numero, &e.Bairro, &e.Cidade, &e.UF
	d.Complemento = nil
	if e.Complemento != "" {
		d.Complemento = &e.Complemento
	}
}

// camposEnderecoDesafio é o mapa de colunas do endereço do desafio para o UPDATE da edição.
func camposEnderecoDesafio(e *enderecoNormalizado) map[string]any {
	c := e.campos()
	c["local"] = c["endereco"]
	delete(c, "endereco")
	return c
}

// indicadosDoPedido normaliza a lista de indicados (id_instrutor legado vira [id]); recusa repetidos.
func indicadosDoPedido(req *models.CriarDesafioRequest) ([]uint, error) {
	ids := req.Indicados
	if len(ids) == 0 && req.IDInstrutor != 0 {
		ids = []uint{req.IDInstrutor}
	} else if len(ids) > 0 && req.IDInstrutor != 0 && !contemID(ids, req.IDInstrutor) {
		return nil, erroRequisicao("Informe a equipe em indicados (id_instrutor é o formato antigo de um único indicado)")
	}
	vistos := map[uint]bool{}
	for _, id := range ids {
		if id == 0 {
			return nil, erroRequisicao("indicados contém um id inválido")
		}
		if vistos[id] {
			return nil, erroRequisicao("indicados contém o mesmo instrutor mais de uma vez")
		}
		vistos[id] = true
	}
	return ids, nil
}

func contemID(ids []uint, id uint) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// Criar cria o desafio da academia do JWT, com endereço estruturado, início e fim, e a equipe de
// instrutores: `instrutores_vagas` vagas, das quais os `indicados` (vinculados e aprovados) já entram
// (o responsável é id_responsavel ou o primeiro indicado); as demais ficam abertas à candidatura. Numa
// única transação grava o desafio e a equipe e debita da academia a reserva (prêmio fixo + vagas x fixo
// por instrutor); sem saldo a criação falha com 422. A academia não paga entrada nem vira participante.
// Uma academia pode ter vários desafios ativos ao mesmo tempo (não há limite).
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
	if req.InstrutorTetoCentavos != nil || req.InstrutorSelecao == models.SelecaoVagaAberta {
		return nil, erroRequisicao(msgLeilaoRemovido)
	}
	if req.InstrutorSelecao != "" && req.InstrutorSelecao != models.SelecaoDesignado {
		return nil, erroRequisicao("instrutor_selecao não é mais usado: a equipe é definida por instrutores_vagas e indicados")
	}
	if req.ValorEntradaCentavos < 0 || req.ValorEntradaCentavos > models.EntradaMaxCentavos {
		return nil, erroRequisicao("Valor de entrada fora do permitido (R$ 0,00 a R$ 10.000,00)")
	}
	if req.Vagas < models.VagasMin || req.Vagas > models.VagasMax {
		return nil, erroRequisicao(fmt.Sprintf("Vagas devem estar entre %d e %d", models.VagasMin, models.VagasMax))
	}
	if req.PosicoesPodio < models.PosicoesPodioMin || req.PosicoesPodio > models.PosicoesPodioMax {
		return nil, erroRequisicao(msgPosicoesPodio)
	}
	if req.PosicoesPodio > req.Vagas {
		return nil, erroRequisicao("posicoes_podio não pode ser maior que o número de vagas")
	}
	divisao, err := divisaoDoPedido(req.DivisaoPremioBP, req.PosicoesPodio)
	if err != nil {
		return nil, err
	}
	resumo, err := textoDesafio("resumo", req.Resumo, models.ResumoMinCaracteres, models.ResumoMaxCaracteres)
	if err != nil {
		return nil, err
	}
	regras, err := textoDesafio("regras", req.Regras, models.RegrasMinCaracteres, models.RegrasMaxCaracteres)
	if err != nil {
		return nil, err
	}
	nivelMin, nivelMax := filtroOpcional(req.NivelMin), filtroOpcional(req.NivelMax)
	idadeMin, idadeMax := filtroOpcional(req.IdadeMin), filtroOpcional(req.IdadeMax)
	if err := validarFiltros(nivelMin, nivelMax, idadeMin, idadeMax); err != nil {
		return nil, err
	}
	categoria, err := validarCategoria(req.Categoria)
	if err != nil {
		return nil, err
	}
	inicio, fim, err := s.intervaloEvento(req.DataEvento, req.DataFimEvento)
	if err != nil {
		return nil, err
	}
	endereco, err := normalizarEndereco(&req.EnderecoAcademiaRequest)
	if err != nil {
		return nil, err
	}
	vagasEquipe := req.InstrutoresVagas
	if vagasEquipe == 0 {
		vagasEquipe = 1
	}
	indicados, err := indicadosDoPedido(req)
	if err != nil {
		return nil, err
	}
	if len(indicados) > vagasEquipe {
		return nil, erroRequisicao(fmt.Sprintf("Há %d indicados para %d vaga(s) de instrutor: aumente instrutores_vagas ou indique menos", len(indicados), vagasEquipe))
	}
	responsavel := req.IDResponsavel
	if responsavel != 0 && !contemID(indicados, responsavel) {
		return nil, erroRequisicao("id_responsavel precisa ser um dos indicados")
	}
	if responsavel == 0 && len(indicados) > 0 {
		responsavel = indicados[0]
	}

	cfg := ConfigFinanceira{
		PremioFixo: req.PremioFixoCentavos, DividirPote: req.DividirPote == nil || *req.DividirPote,
		Taxa: s.taxaInscricaoPct, Modo: req.InstrutorModoPagamento, Fixo: req.InstrutorFixoCentavos,
		Pct: req.InstrutorPct, Vagas: vagasEquipe,
	}
	if cfg.Modo == "" {
		cfg.Modo = models.InstrutorFixo
	}
	if req.PercentualPote != nil {
		cfg.PercentualPote = *req.PercentualPote
	}
	if err := cfg.Validar(req.PercentualPote == nil); err != nil {
		return nil, err
	}

	academiaID := ctx.AcademiaID
	reserva := cfg.Reserva()
	desafio := &models.Desafio{
		Titulo:                  strings.TrimSpace(req.Titulo),
		Resumo:                  &resumo,
		Regras:                  &regras,
		NivelMin:                nivelMin,
		NivelMax:                nivelMax,
		IdadeMin:                idadeMin,
		IdadeMax:                idadeMax,
		Categoria:               categoria,
		ValorEntradaCentavos:    req.ValorEntradaCentavos,
		DataEvento:              &inicio,
		DataFimEvento:           &fim,
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
		InstrutoresVagas:        vagasEquipe,
		InstrutorSelecao:        models.SelecaoDesignado, // legado: a equipe substituiu a vaga aberta
		ReservaAcademiaCentavos: reserva,
	}
	if d := strings.TrimSpace(req.Descricao); d != "" { // legado: guardada, mas resumo/regras é que valem
		desafio.Descricao = &d
	}
	aplicarEnderecoDesafio(desafio, endereco)

	var avisar []uint
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Trava os indicados (ordem estável de id, a mesma do desvínculo) antes de gravar.
		ordenados := append([]uint(nil), indicados...)
		sort.Slice(ordenados, func(i, j int) bool { return ordenados[i] < ordenados[j] })
		for _, idInst := range ordenados {
			i, err := s.instrutorVinculadoTravado(tx, idInst, ctx.AcademiaID)
			if err != nil {
				return err
			}
			avisar = append(avisar, i.IDUsuario)
		}
		if responsavel != 0 {
			desafio.IDInstrutor = &responsavel // espelho legado do responsável
		}
		repo := s.desafios.ComTx(tx)
		if err := repo.Criar(desafio); err != nil {
			return err
		}
		// Divisão do prêmio por posição (U1) na mesma transação do desafio.
		if err := repo.SubstituirDivisaoPremio(desafio.ID, divisao); err != nil {
			return err
		}
		for _, idInst := range indicados {
			if err := repo.AdicionarMembro(&models.DesafioInstrutor{IDDesafio: desafio.ID, IDInstrutor: idInst,
				Origem: models.EquipeOrigemIndicado, Responsavel: idInst == responsavel}); err != nil {
				return err
			}
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
	if repositories.EhViolacaoCheck(err) { // CHECK de CEP/UF/fim/vagas/soma/filtros/premiação do banco (3819)
		return nil, erro(http.StatusUnprocessableEntity, msgCheckDesafio)
	}
	if err != nil {
		return nil, err
	}
	if len(avisar) > 0 {
		s.notif.NotificarVarios(avisar, models.NotifInstrutorDesignado, "Você foi indicado para uma equipe",
			fmt.Sprintf("A academia indicou você para a equipe de instrutores do desafio \"%s\".", desafio.Titulo), linkDesafio(desafio.ID))
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

// AutorizarCandidatura: desafio existe (404) e o usuário é instrutor no modo instrutor (403).
func (s *desafioService) AutorizarCandidatura(userID, id uint) error {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return err
	}
	if err := exigirInstrutor(ctx, "Apenas instrutores podem se candidatar a uma vaga"); err != nil {
		return err
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
// houver inscrito, nem as vagas abaixo dos inscritos. A equipe de instrutores muda pelas rotas
// /desafios/:id/equipe (aqui só instrutores_vagas e o pagamento por instrutor).
func (s *desafioService) Editar(userID, id uint, req *models.EditarDesafioRequest) (*models.DesafioResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	if req.IDInstrutor != nil {
		return nil, erroRequisicao("A equipe de instrutores não muda por aqui: use POST/DELETE /api/desafios/:id/equipe e PUT /api/desafios/:id/equipe/responsavel")
	}
	if req.InstrutorTetoCentavos != nil {
		return nil, erroRequisicao(msgLeilaoRemovido)
	}
	// Mudança de horário com inscritos (pendência da rodada 5): os inscritos ficam travados (desafio ->
	// usuários em ordem de id, antes da 1ª leitura consistente) para a conferência do choque enxergar as
	// inscrições já confirmadas e uma inscrição concorrente deles esperar esta edição.
	var travarInscritos func(repo *repositories.DesafioRepository) ([]uint, error)
	if req.DataEvento != nil || req.DataFimEvento != nil {
		travarInscritos = func(repo *repositories.DesafioRepository) ([]uint, error) { return repo.InscritosTravados(id) }
	}
	err = s.comDesafioTravadoE(id, travarInscritos, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
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
		if req.Resumo != nil {
			v, err := textoDesafio("resumo", *req.Resumo, models.ResumoMinCaracteres, models.ResumoMaxCaracteres)
			if err != nil {
				return err
			}
			campos["resumo"] = v
		}
		if req.Regras != nil {
			v, err := textoDesafio("regras", *req.Regras, models.RegrasMinCaracteres, models.RegrasMaxCaracteres)
			if err != nil {
				return err
			}
			campos["regras"] = v
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
		if e := enderecoDaEdicao(req); e != nil {
			end, err := normalizarEndereco(e)
			if err != nil {
				return err
			}
			for k, v := range camposEnderecoDesafio(end) {
				campos[k] = v
			}
		}
		if req.Latitude != nil {
			campos["latitude"] = *req.Latitude
		}
		if req.Longitude != nil {
			campos["longitude"] = *req.Longitude
		}
		inicio, fim := d.DataEvento, d.DataFimEvento
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
				inicio = &evento
			}
		}
		if req.DataFimEvento != nil {
			f, err := parseDataEvento(*req.DataFimEvento)
			if err != nil {
				return erroRequisicao("data_fim_evento inválida (use RFC3339 ou YYYY-MM-DDTHH:MM)")
			}
			campos["data_fim_evento"] = f
			fim = &f
		}
		if inicio != nil && fim != nil && !fim.After(*inicio) {
			return erroRequisicao("data_fim_evento deve ser depois de data_evento (início)")
		}
		if err := exigirHorarioSemChoqueDosInscritos(repo, d, inicio, fim, inscritos); err != nil {
			return err
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
				return erroRequisicao(msgPosicoesPodio)
			}
			posicoes = *req.PosicoesPodio
			campos["posicoes_podio"] = posicoes
		}
		if posicoes > vagas {
			return erroRequisicao("posicoes_podio não pode ser maior que o número de vagas")
		}
		if err := s.editarPremiacaoEFiltros(repo, d, req, posicoes, inscritos, campos); err != nil {
			return err
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
		if err := s.editarFinanceiro(tx, repo, d, req, inscritos, campos, ev); err != nil {
			return err
		}
		return repo.Atualizar(d.ID, campos)
	})
	if repositories.EhViolacaoCheck(err) { // CHECK de CEP/UF/fim/vagas/soma/filtros/premiação do banco (3819)
		return nil, erro(http.StatusUnprocessableEntity, msgCheckDesafio)
	}
	if err != nil {
		return nil, err
	}
	return s.Detalhe(userID, id)
}

// editarPremiacaoEFiltros aplica a divisão do prêmio por posição e os filtros de nível/idade do PATCH
// (U1/U4). Mudam só ANTES da 1ª inscrição (409 depois); reenviar o valor atual não conta como mudança.
// Mudar posicoes_podio sem divisao_premio_bp aplica a divisão padrão do novo tamanho.
func (s *desafioService) editarPremiacaoEFiltros(repo *repositories.DesafioRepository, d *models.Desafio,
	req *models.EditarDesafioRequest, posicoes int, inscritos int64, campos map[string]any) error {
	atual := divisaoDoDesafio(d)
	nova := atual
	switch {
	case len(req.DivisaoPremioBP) > 0:
		if err := ValidarDivisaoBP(req.DivisaoPremioBP, posicoes); err != nil {
			return err
		}
		nova = req.DivisaoPremioBP
	case posicoes != d.PosicoesPodio:
		nova = DivisaoPadraoBP(posicoes)
	}
	mudouDivisao := !igualInts(nova, atual) || posicoes != d.PosicoesPodio

	nivelMin, nivelMax, idadeMin, idadeMax := d.NivelMin, d.NivelMax, d.IdadeMin, d.IdadeMax
	if req.NivelMin != nil {
		nivelMin = filtroOpcional(req.NivelMin)
	}
	if req.NivelMax != nil {
		nivelMax = filtroOpcional(req.NivelMax)
	}
	if req.IdadeMin != nil {
		idadeMin = filtroOpcional(req.IdadeMin)
	}
	if req.IdadeMax != nil {
		idadeMax = filtroOpcional(req.IdadeMax)
	}
	mudouFiltros := !igualIntPtr(nivelMin, d.NivelMin) || !igualIntPtr(nivelMax, d.NivelMax) ||
		!igualIntPtr(idadeMin, d.IdadeMin) || !igualIntPtr(idadeMax, d.IdadeMax)
	if !mudouDivisao && !mudouFiltros {
		return nil
	}
	if inscritos > 0 {
		if mudouDivisao {
			return erroConflito("Já há atletas inscritos: o número de premiados e a divisão do prêmio não podem mais mudar")
		}
		return erroConflito("Já há atletas inscritos: os requisitos de nível e idade não podem mais mudar")
	}
	if mudouFiltros {
		if err := validarFiltros(nivelMin, nivelMax, idadeMin, idadeMax); err != nil {
			return err
		}
		campos["nivel_min"], campos["nivel_max"], campos["idade_min"], campos["idade_max"] = nivelMin, nivelMax, idadeMin, idadeMax
	}
	if mudouDivisao {
		return repo.SubstituirDivisaoPremio(d.ID, nova)
	}
	return nil
}

// exigirHorarioSemChoqueDosInscritos (pendência da rodada 5): mudar início/fim de um desafio com inscritos
// é recusado (409) se o novo intervalo se sobrepõe a outro desafio ativo da MESMA academia em que algum
// inscrito também está. Exige os inscritos travados (ver Editar).
func exigirHorarioSemChoqueDosInscritos(repo *repositories.DesafioRepository, d *models.Desafio, inicio, fim *time.Time, inscritos int64) error {
	if inscritos == 0 || d.IDAcademia == nil || inicio == nil || fim == nil {
		return nil
	}
	if d.DataEvento != nil && d.DataFimEvento != nil && inicio.Equal(*d.DataEvento) && fim.Equal(*d.DataFimEvento) {
		return nil
	}
	atletas, titulo, err := repo.ChoqueDeInscritos(d.ID, *d.IDAcademia, *inicio, *fim)
	if err != nil {
		return err
	}
	if atletas > 0 {
		return erroConflito(fmt.Sprintf("Não é possível mudar o horário: %d atleta(s) inscrito(s) também está(ão) em \"%s\" no novo horário", atletas, titulo))
	}
	return nil
}

func igualInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func igualIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// enderecoDaEdicao devolve o endereço do PATCH quando algum campo dele veio (nil = não altera). O
// endereço muda inteiro: campos ausentes viram vazios e a validação recusa o incompleto.
func enderecoDaEdicao(req *models.EditarDesafioRequest) *models.EnderecoAcademiaRequest {
	campos := []*string{req.CEP, req.Logradouro, req.Numero, req.Complemento, req.Bairro, req.Cidade, req.UF}
	algum := false
	for _, c := range campos {
		algum = algum || c != nil
	}
	if !algum {
		return nil
	}
	v := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return &models.EnderecoAcademiaRequest{CEP: v(req.CEP), Logradouro: v(req.Logradouro), Numero: v(req.Numero),
		Complemento: v(req.Complemento), Bairro: v(req.Bairro), Cidade: v(req.Cidade), UF: v(req.UF)}
}

// editarFinanceiro aplica ao mapa de campos as mudanças de premiação, de tamanho da equipe e de pagamento
// por instrutor do PATCH e ajusta a reserva da academia pela diferença (débito `reserva_desafio` ou
// crédito `estorno_reserva`). A taxa da plataforma continua a gravada na criação. Com atletas inscritos o
// prêmio só pode aumentar. O pagamento por instrutor fica travado quando há membro vindo de candidatura
// ou candidatura pendente (eles aceitaram aquelas condições). instrutores_vagas não fica abaixo da
// equipe atual; se ficar igual, as candidaturas pendentes são recusadas (equipe completa).
func (s *desafioService) editarFinanceiro(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio,
	req *models.EditarDesafioRequest, inscritos int64, campos map[string]any, ev *eventos) error {
	tocaPremio := req.PremioFixoCentavos != nil || req.DividirPote != nil || req.PercentualPote != nil
	tocaInstrutor := req.InstrutorModoPagamento != nil || req.InstrutorFixoCentavos != nil || req.InstrutorPct != nil
	tocaVagas := req.InstrutoresVagas != nil && *req.InstrutoresVagas != d.InstrutoresVagas
	if !tocaPremio && !tocaInstrutor && !tocaVagas {
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
	if req.InstrutoresVagas != nil {
		novo.Vagas = *req.InstrutoresVagas
	}
	if novo.Modo != antes.Modo || novo.Fixo != antes.Fixo || novo.Pct != antes.Pct {
		aceitaram, err := s.candidaturas.ComTx(tx).ContarAtivasDoDesafio(d.ID)
		if err != nil {
			return err
		}
		if aceitaram > 0 {
			return erroConflito("Há instrutores que se candidataram com estas condições (pendentes ou já na equipe): o pagamento por instrutor não pode mais ser alterado")
		}
	}
	if novo.Vagas < len(d.Equipe) {
		return erroConflito(fmt.Sprintf("A equipe já tem %d instrutor(es): instrutores_vagas não pode ser menor que isso", len(d.Equipe)))
	}
	// Ao desligar o % do instrutor (mudança de modo para sem %), o pct antigo deixa de valer.
	if !models.ModoTemPercentual(novo.Modo) && req.InstrutorPct == nil {
		novo.Pct = 0
	}
	if !models.ModoTemFixo(novo.Modo) && req.InstrutorFixoCentavos == nil {
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
	if novo.Vagas == len(d.Equipe) && novo.Vagas != antes.Vagas {
		recusados, err := s.candidaturas.ComTx(tx).RecusarPendentes(d.ID, 0)
		if err != nil {
			return err
		}
		titulo, idDesafio := d.Titulo, d.ID
		ev.add(func() {
			s.notif.NotificarVarios(recusados, models.NotifCandidaturaRecusada, "Candidatura não aceita",
				fmt.Sprintf("A equipe de instrutores do desafio \"%s\" foi fechada e sua candidatura foi encerrada.", titulo), linkDesafio(idDesafio))
		})
	}

	campos["premio_fixo_centavos"] = novo.PremioFixo
	campos["dividir_pote"] = novo.DividirPote
	campos["percentual_pote"] = novo.PercentualPote
	campos["instrutor_modo_pagamento"] = novo.Modo
	campos["instrutor_fixo_centavos"] = novo.Fixo
	campos["instrutor_pct"] = novo.Pct
	campos["instrutores_vagas"] = novo.Vagas

	reserva := novo.Reserva()
	campos["reserva_academia_centavos"] = reserva
	return s.ajustarReserva(tx, d, reserva, "edição do desafio")
}

// ajustarReserva debita (reserva_desafio) ou devolve (estorno_reserva) a diferença entre a reserva nova e
// a vigente do desafio. Não grava reserva_academia_centavos (o chamador grava junto com os outros campos).
// motivo entra só na descrição do estorno ("Reserva devolvida (<motivo>): <título>").
func (s *desafioService) ajustarReserva(tx *gorm.DB, d *models.Desafio, reserva int64, motivo string) error {
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
			IDDesafio: &d.ID, Descricao: "Reserva devolvida (" + motivo + "): " + d.Titulo,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ─── Transições ──────────────────────────────────────────────────────────────

// comDesafioTravado abre uma transação, trava a linha do desafio (FOR UPDATE), carrega a equipe de
// instrutores (estável enquanto a trava durar: toda mudança na equipe trava o mesmo desafio) e executa fn.
func (s *desafioService) comDesafioTravado(id uint, fn func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) error {
	return s.comDesafioEUsuarioTravados(id, 0, fn)
}

// comDesafioEUsuarioTravados é comDesafioTravado que, com travarUsuario != 0, trava também a linha do
// usuário (ordem desafio -> usuário, a mesma dos créditos) ANTES da primeira leitura consistente da
// transação: assim o snapshot começa depois da trava e enxerga o que outra transação do mesmo usuário
// acabou de confirmar (choque de horário na inscrição, T6).
func (s *desafioService) comDesafioEUsuarioTravados(id, travarUsuario uint, fn func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) error {
	var travar func(repo *repositories.DesafioRepository) ([]uint, error)
	if travarUsuario != 0 {
		travar = func(*repositories.DesafioRepository) ([]uint, error) { return []uint{travarUsuario}, nil }
	}
	return s.comDesafioTravadoE(id, travar, fn)
}

// comDesafioTravadoE é a forma geral: trava o desafio, depois (se travar != nil) as linhas dos usuários
// que travar devolver, em ordem crescente de id (a mesma dos créditos), e só então faz a primeira leitura
// consistente (equipe e divisão do prêmio). travar só pode fazer leituras de trava (FOR UPDATE/SHARE).
func (s *desafioService) comDesafioTravadoE(id uint, travar func(repo *repositories.DesafioRepository) ([]uint, error),
	fn func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) error {
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
		if travar != nil {
			ids, err := travar(repo)
			if err != nil {
				return err
			}
			if err := repositories.TravarUsuariosOrdenados(tx, idsUnicosOrdenados(ids)); err != nil {
				return err
			}
		}
		if d.Equipe, err = repo.ListarEquipe(d.ID); err != nil {
			return err
		}
		if d.PremioPosicoes, err = repo.ListarDivisaoPremio(d.ID); err != nil {
			return err
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

// acaoTravandoUsuario é acao com a linha do próprio usuário travada logo depois do desafio (ver
// comDesafioEUsuarioTravados): usada pela inscrição (choque de horário entre desafios da mesma academia).
func (s *desafioService) acaoTravandoUsuario(userID, id uint, fn func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) (*models.DesafioResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	if err := s.comDesafioEUsuarioTravados(id, userID, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		return fn(ctx, tx, repo, d, ev)
	}); err != nil {
		return nil, err
	}
	return s.Detalhe(userID, id)
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
//
// Choque de horário (T6, rodada 5): recusa (409) se o atleta já está inscrito, em desafio aberto ou em
// andamento DA MESMA academia, com intervalo [início, fim) sobreposto. A linha do usuário fica travada
// (desafio -> usuário) antes da leitura das inscrições: duas inscrições simultâneas do mesmo atleta em
// desafios sobrepostos se serializam e a segunda vê a primeira.
func (s *desafioService) Entrar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acaoTravandoUsuario(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirAtleta(ctx); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Desafio não está aberto para inscrições")
		}
		if d.DataEvento != nil && !d.DataEvento.After(s.agora()) {
			return erroConflito("O evento já começou: as inscrições estão encerradas")
		}
		// Conflito de interesse (rodada 4): reconfere o registro de instrutor sob trava (FOR SHARE), com o
		// desafio já travado, antes de qualquer escrita.
		if err := s.exigirSemConflitoComoAtleta(tx, userID, d); err != nil {
			return err
		}
		if jaParticipa, err := repo.UsuarioParticipa(d.ID, userID); err != nil {
			return err
		} else if jaParticipa {
			return erroConflito("Você já está participando deste desafio")
		}
		if err := exigirSemBloqueioAcademia(repo, userID, d, s.agora()); err != nil {
			return err
		}
		inscricoes, err := repo.InscricoesAtivasComHorario(userID)
		if err != nil {
			return err
		}
		if o := choqueDeHorario(inscricoes, d); o != nil {
			return erroConflito(msgChoqueHorario(o.Titulo))
		}
		// U4: nível (recalculado agora, com a linha do usuário travada) e idade na data do evento.
		perfil, err := elegibilidadeNaInscricao(tx, userID, s.agora())
		if err != nil {
			return err
		}
		if m := MotivoFiltroEntrada(d, perfil, s.agora()); m != "" {
			return erroConflito(m)
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
	equipeUsers := usuariosDaEquipe(d)
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
		if len(equipeUsers) > 0 {
			msgInst := fmt.Sprintf("A academia cancelou o desafio \"%s\".", titulo)
			if automatico {
				msgInst = fmt.Sprintf("O desafio \"%s\" %s.", titulo, causa)
			}
			s.notif.NotificarVarios(equipeUsers, models.NotifDesafioCancel, "Desafio cancelado", msgInst, linkDesafio(idDesafio))
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

// Iniciar (responsável da equipe ou academia dona, desafio aberto, >= 1 inscrito, >= 1 instrutor na equipe
// e responsável definido — T5: não exige a equipe completa) move para em_andamento e fecha as inscrições
// e as candidaturas (as pendentes são recusadas e avisadas). A equipe fica congelada; a parte fixa das
// vagas não preenchidas volta para a academia no encerramento (ou na reserva inteira, se cancelar).
func (s *desafioService) Iniciar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirComando(ctx, d); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Apenas desafios abertos podem ser iniciados")
		}
		if len(d.Equipe) == 0 {
			return erroConflito("A equipe de instrutores está vazia: indique um instrutor ou aceite uma candidatura antes de iniciar")
		}
		if d.ResponsavelEquipe() == nil {
			return erroConflito("Defina o instrutor responsável da equipe antes de iniciar")
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
		recusados, err := s.candidaturas.ComTx(tx).RecusarPendentes(d.ID, 0)
		if err != nil {
			return err
		}
		ids, titulo, idDesafio := idsExceto(ps, 0), d.Titulo, d.ID
		ev.add(func() {
			s.notif.NotificarVarios(ids, models.NotifDesafioIniciado, "Desafio iniciado",
				fmt.Sprintf("O desafio \"%s\" começou. Bora treinar!", titulo), linkDesafio(idDesafio))
			s.notif.NotificarVarios(recusados, models.NotifCandidaturaRecusada, "Candidatura encerrada",
				fmt.Sprintf("O desafio \"%s\" começou com a equipe atual e sua candidatura foi encerrada.", titulo), linkDesafio(idDesafio))
		})
		return nil
	})
}

// ─── Presença ────────────────────────────────────────────────────────────────

// elegivelPodio devolve o filtro de quem pode ter presença marcada e entrar no pódio: inscritos com
// conta de atleta que não sejam usuário de NENHUM membro da equipe de instrutores nem instrutor aprovado
// vinculado à academia do desafio (conflito de interesse, rodadas 4/5: o instrutor compete como atleta,
// exceto onde trabalha). Cobre inscrições legadas/anteriores à regra — a inscrição nova já é recusada em
// Entrar. Exige d.Equipe carregada.
func (s *desafioService) elegivelPodio(repo *repositories.DesafioRepository, d *models.Desafio) (func(uint) bool, error) {
	var idAcademia uint
	if d.IDAcademia != nil {
		idAcademia = *d.IDAcademia
	}
	elegiveis, err := repo.ElegibilidadeParticipantes(d.ID, idAcademia)
	if err != nil {
		return nil, err
	}
	equipe := map[uint]bool{}
	for _, uid := range usuariosDaEquipe(d) {
		equipe[uid] = true
	}
	return func(uid uint) bool { return elegiveis[uid] && !equipe[uid] }, nil
}

// usuariosDaEquipe devolve o id de USUÁRIO de cada membro da equipe (exige d.Equipe com Instrutor).
func usuariosDaEquipe(d *models.Desafio) []uint {
	ids := make([]uint, 0, len(d.Equipe))
	for _, m := range d.Equipe {
		if m.Instrutor != nil {
			ids = append(ids, m.Instrutor.IDUsuario)
		}
	}
	return ids
}

// MarcarPresenca (qualquer membro da equipe ou academia dona) confirma a presença de um inscrito, pelo
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
			return erroConflito("Só atletas inscritos podem ter presença confirmada (academia, equipe de instrutores do desafio e instrutores da academia não competem)")
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

// ListarPresentes (equipe de instrutores ou academia dona) devolve os atletas com presença confirmada.
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

// LancarPodio (instrutor RESPONSÁVEL da equipe ou academia dona, desafio em andamento) grava o pódio,
// divide o pote, registra os pagamentos (prêmios, cada membro da equipe, academia, taxa), aplica pontos,
// encerra e notifica — tudo numa transação.
//
// Regras: exatamente min(posicoes_podio, presentes) entradas, só presentes, sem repetir e com marca
// obrigatória. Pote = entrada x inscritos (todos, presentes ou não).
func (s *desafioService) LancarPodio(userID, id uint, req *models.LancarPodioRequest) (*models.DesafioResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirComando(ctx, d); err != nil {
			return err
		}
		// Q3: depois de uma anulação em relancar_podio, só a academia dona relança (o instrutor
		// responsável costuma ser o alvo da denúncia).
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

		dist, err := s.calcularDistribuicao(d, len(participantes), len(d.Equipe), esperado)
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
		// Cada membro da equipe recebe fixo + % (por instrutor), em ordem estável de usuário.
		instrutoresUsers := usuariosDaEquipe(d)
		if len(instrutoresUsers) != len(d.Equipe) {
			return fmt.Errorf("desafio %d: membro da equipe sem instrutor carregado", d.ID)
		}
		sort.Slice(instrutoresUsers, func(i, j int) bool { return instrutoresUsers[i] < instrutoresUsers[j] })
		if fin := dist.Financeiro; fin.InstrutorPorMembro > 0 {
			for _, uid := range instrutoresUsers {
				uid := uid
				lote = append(lote, pendente(&uid, models.PendentePagamentoInstrutor, fin.InstrutorPorMembro))
			}
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
		// Nível GymScore (U3): pódio + presença no desafio encerrado contam já. Recalcula os presentes na
		// mesma transação (linhas já travadas acima, em ordem de id) e de novo após o commit (corrida com
		// outro evento do mesmo atleta; ver gymscore.go).
		idsNivel := make([]uint, 0, len(resultados))
		for _, r := range resultados {
			idsNivel = append(idsNivel, r.idUsuario)
		}
		if _, err := recalcularGymScoreTx(tx, idsNivel, agora); err != nil {
			return err
		}
		ev.add(func() { s.recalcularNivelAposCommit(idsNivel) })
		academiaUser, pagoInstrutor, pagoAcademia := d.IDCriador, dist.Financeiro.InstrutorPorMembro, dist.Financeiro.Academia

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
			if pagoInstrutor > 0 {
				s.notif.NotificarVarios(instrutoresUsers, models.NotifPagamentoInstrutor, "Pagamento a liberar",
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

// recalcularNivelAposCommit recalcula o nível dos usuários fora da transação do evento (falha só loga:
// o job reconciliar-nivel corrige).
func (s *desafioService) recalcularNivelAposCommit(ids []uint) {
	if err := RecalcularGymScore(s.db, ids, s.agora()); err != nil {
		log.Printf("[GYMSCORE] recálculo do nível após o evento: %v", err)
	}
}

// distribuicao é o resultado financeiro do encerramento de um desafio.
type distribuicao struct {
	// PremioTotal é o valor a repartir entre as posições do pódio (centavos): prêmio fixo + parcela das
	// inscrições. Nunca é reduzido pela taxa da plataforma.
	PremioTotal int64
	// Premios é o prêmio de cada posição (índice 0 = 1º), já dividido por DividirPremio com a divisão
	// do desafio (renormalizada entre as posições ocupadas; sobra de centavos para o 1º).
	Premios []int64
	// Financeiro é a conta completa: arrecadação, taxa, instrutor e parte da academia.
	Financeiro ResultadoFinanceiro
}

// calcularDistribuicao é o ÚNICO ponto que decide quanto vai para cada parte ao lançar o pódio.
// inscritos = todos os inscritos que restaram (presentes ou não); instrutores = membros da equipe (cada um
// recebe fixo + %); presentesNoPodio = posições efetivamente premiadas. Garante a invariante
// entradas + reserva = prêmios + instrutores + academia + taxa (a parte fixa das vagas não preenchidas
// fica na parte da academia).
func (s *desafioService) calcularDistribuicao(d *models.Desafio, inscritos, instrutores, presentesNoPodio int) (distribuicao, error) {
	fin, err := CalcularFinanceiro(paramsDoDesafio(d, inscritos, instrutores))
	if err != nil {
		return distribuicao{}, fmt.Errorf("desafio %d: %w", d.ID, err)
	}
	// Divisão configurada pela academia (desafio_premio_posicoes, carregada sob a trava do desafio).
	premios := DividirPremio(fin.PremioTotal, divisaoDoDesafio(d), presentesNoPodio)
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
	// Avaliar é ação do modo atleta (rodada 4).
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	if err := exigirAtleta(ctx); err != nil {
		return nil, err
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
	aberto := d.Status == models.StatusAberto
	emAndamento := d.Status == models.StatusEmAndamento
	vagasLivres := d.VagasInstrutorLivres()
	vagaAberta := aberto && vagasLivres > 0
	// Previsão sobre as inscrições atuais. Aberto: a equipe completa (pior caso, igual ao reservado);
	// depois de iniciar a equipe congela e conta quem está nela.
	pagos := len(d.Equipe)
	if aberto && d.InstrutoresVagas > pagos {
		pagos = d.InstrutoresVagas
	}
	fin, errFin := CalcularFinanceiro(paramsDoDesafio(d, d.QtdParticipantes, pagos))
	if errFin != nil { // configuração inconsistente (não deveria existir): mostra só o que dá para afirmar
		fin = ResultadoFinanceiro{Arrecadacao: poteCentavos, PremioTotal: d.PremioFixoCentavos}
	}
	divisao := divisaoDoDesafio(d)
	previsao := DividirPremio(fin.PremioTotal, divisao, d.PosicoesPodio)
	if previsao == nil {
		previsao = []int64{}
	}
	if divisao == nil {
		divisao = []int{}
	}
	resp := &models.DesafioResponse{
		ID:                        d.ID,
		Titulo:                    d.Titulo,
		Categoria:                 d.Categoria,
		ValorEntradaCentavos:      d.ValorEntradaCentavos,
		ValorEntrada:              utils.CentavosParaReais(d.ValorEntradaCentavos),
		PoteCentavos:              poteCentavos,
		Pote:                      utils.CentavosParaReais(poteCentavos),
		Endereco:                  enderecoDoDesafio(d),
		Latitude:                  d.Latitude,
		Longitude:                 d.Longitude,
		DataEvento:                d.DataEvento,
		DataFimEvento:             d.DataFimEvento,
		Status:                    d.Status,
		Vagas:                     d.Vagas,
		PosicoesPodio:             d.PosicoesPodio,
		DivisaoPremioBP:           divisao,
		NivelMin:                  d.NivelMin,
		NivelMax:                  d.NivelMax,
		IdadeMin:                  d.IdadeMin,
		IdadeMax:                  d.IdadeMax,
		PremiacaoPrevistaCentavos: previsao,
		Premiacao: models.PremiacaoResponse{
			PremioTotalCentavos: fin.PremioTotal, PremioTotal: utils.CentavosParaReais(fin.PremioTotal), PorPosicaoCentavos: previsao,
			DivisaoBP: divisao,
		},
		Equipe:                equipeResponse(d),
		InstrutoresVagas:      d.InstrutoresVagas,
		VagasInstrutorAbertas: vagasLivres,
		VagaInstrutorAberta:   vagaAberta,
		ParticipantesCount:    d.QtdParticipantes,
		PresentesCount:        d.QtdPresentes,
		CriadoEm:              d.CriadoEm,
		Participantes:         []models.ParticipanteResponse{},
		Podio:                 []models.PodioResponse{},
	}
	if d.Resumo != nil {
		resp.Resumo = *d.Resumo
	}
	if d.Regras != nil {
		resp.Regras = *d.Regras
	}
	resp.Descricao = resp.Resumo // compat: telas antigas leem `descricao`
	if d.Descricao != nil && strings.TrimSpace(*d.Descricao) != "" {
		resp.Descricao = *d.Descricao
	}
	if d.Local != nil {
		resp.Local = *d.Local
	}
	if d.Academia != nil {
		resp.Academia = &models.AcademiaResumo{ID: d.Academia.ID, NomeFantasia: d.Academia.NomeFantasia}
	}
	for _, m := range resp.Equipe {
		if m.Responsavel {
			resp.Instrutor = &models.InstrutorResumo{ID: m.ID, IDUsuario: m.IDUsuario, Nome: m.Nome}
		}
	}

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
	dono, staff, comando := ctx.dono(d), ctx.staff(d), ctx.comando(d)
	eventoPassou := d.DataEvento != nil && !d.DataEvento.After(agora)

	resp.PodeEntrar = atleta && aberto && !participa && d.QtdParticipantes < d.Vagas && !eventoPassou
	if atleta && aberto && !participa && !eventoPassou {
		// B4-8: conflito de interesse (equipe / vinculado à academia / candidatura ativa) — Entrar daria 409.
		if m := motivoConflitoEntrada(ctx.conflito, d); m != "" {
			resp.PodeEntrar = false
			resp.MotivoBloqueioEntrada = m
		} else if o := choqueDeHorario(ctx.inscricoes, d); o != nil {
			// T6: já inscrito em outro desafio da mesma academia no mesmo horário.
			resp.PodeEntrar = false
			resp.MotivoBloqueioEntrada = msgChoqueHorario(o.Titulo)
		} else if ctx.elegibilidade != nil {
			// U4: requisitos de nível/idade (mesma regra e mensagem do 409 de Entrar).
			if m := MotivoFiltroEntrada(d, *ctx.elegibilidade, agora); m != "" {
				resp.PodeEntrar = false
				resp.MotivoBloqueioEntrada = m
			}
		}
	}
	resp.PodeSair = atleta && aberto && participa
	resp.PodeIniciar = comando && aberto && d.QtdParticipantes > 0 && len(d.Equipe) > 0 && d.ResponsavelEquipe() != nil
	resp.PodeMarcarPresenca = staff && d.QtdParticipantes > 0 &&
		(emAndamento || (aberto && (d.DataEvento == nil || mesmoDia(*d.DataEvento, agora))))
	resp.PodeLancarPodio = comando && emAndamento && d.QtdPresentes > 0
	resp.PodeCancelar = dono && (aberto || emAndamento)
	resp.PodeEditar = dono && aberto
	resp.PodeEditarPremiacao = dono && aberto && d.QtdParticipantes == 0
	resp.PodeAvaliar = participa && resp.VocePresente && d.Status == models.StatusEncerrado && !avaliou
	resp.PodeGerirEquipe = dono && aberto
	resp.PodeEscolherInstrutor = resp.PodeGerirEquipe && vagaAberta
	resp.PodeDefinirResponsavel = dono && (aberto || emAndamento) && len(d.Equipe) > 0
	resp.VoceNaEquipe = ctx.instrutorDe(d)
	resp.VoceResponsavel = ctx.responsavelDe(d)

	// Visões financeiras por perfil: só a academia dona vê taxa/instrutores/líquido/reserva; cada membro
	// da equipe vê o que ELE vai ganhar; instrutores veem a oferta (condições por instrutor). Atletas e
	// demais só têm `premiacao`.
	if dono {
		resp.Financeiro = &models.FinanceiroResponse{
			ArrecadacaoCentavos: fin.Arrecadacao, PremioFixoCentavos: d.PremioFixoCentavos,
			DividirPote: d.DividirPote, PercentualPote: d.PercentualPote, PotePremioCentavos: fin.PotePremio,
			PremioTotalCentavos: fin.PremioTotal, TaxaPlataformaPct: d.TaxaPlataformaPct, TaxaPlataformaCentavos: fin.Taxa,
			InstrutorModoPagamento: d.InstrutorModoPagamento,
			InstrutorFixoCentavos:  fin.InstrutorFixo, InstrutorPct: d.InstrutorPct, InstrutorPctCentavos: fin.InstrutorPct,
			InstrutorTotalCentavos: fin.InstrutorTotal, InstrutoresVagas: d.InstrutoresVagas, EquipeCount: len(d.Equipe),
			InstrutoresPagos:        pagos,
			LiquidoPrevistoCentavos: fin.Academia, ResultadoPrevistoCentavos: fin.Academia - d.ReservaAcademiaCentavos,
			ReservaCentavos: d.ReservaAcademiaCentavos, CandidaturasPendentes: d.QtdCandidaturas,
		}
	}
	if resp.VoceNaEquipe {
		resp.InstrutorGanho = &models.GanhoInstrutorResponse{
			Responsavel: resp.VoceResponsavel, ModoPagamento: d.InstrutorModoPagamento, FixoCentavos: fin.InstrutorFixo,
			Pct: d.InstrutorPct, PctCentavos: fin.InstrutorPct, TotalPrevistoCentavos: fin.InstrutorPorMembro,
		}
	}
	if ctx.InstrutorID != 0 && (vagaAberta || d.MinhaCandidatura != nil) {
		resp.Vaga = &models.VagaInstrutorResponse{
			ModoPagamento: d.InstrutorModoPagamento, FixoCentavos: fin.InstrutorFixo, Pct: d.InstrutorPct,
			PctPrevistoCentavos: fin.InstrutorPct, InstrutoresVagas: d.InstrutoresVagas, VagasAbertas: vagasLivres,
		}
		if d.MinhaCandidatura != nil {
			c := paraCandidaturaResponse(d.MinhaCandidatura, nil)
			resp.Vaga.MinhaCandidatura = &c
		}
		st := ""
		if d.MinhaCandidatura != nil {
			st = d.MinhaCandidatura.Status
		}
		resp.PodeCandidatar = vagaAberta && !eventoPassou && !resp.VoceNaEquipe && st != models.CandidaturaEscolhida
	}
	return resp
}

// enderecoDoDesafio devolve o endereço estruturado (nil em desafios antigos, só com `local`).
func enderecoDoDesafio(d *models.Desafio) *models.EnderecoResponse {
	if d.CEP == nil {
		return nil
	}
	v := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return &models.EnderecoResponse{CEP: v(d.CEP), Logradouro: v(d.Logradouro), Numero: v(d.Numero),
		Complemento: v(d.Complemento), Bairro: v(d.Bairro), Cidade: v(d.Cidade), UF: v(d.UF)}
}

// equipeResponse converte a equipe carregada (responsável primeiro, depois ordem de entrada).
func equipeResponse(d *models.Desafio) []models.MembroEquipeResponse {
	out := make([]models.MembroEquipeResponse, 0, len(d.Equipe))
	for _, m := range d.Equipe {
		r := models.MembroEquipeResponse{ID: m.IDInstrutor, Origem: m.Origem, Responsavel: m.Responsavel, EntrouEm: m.CriadoEm}
		if m.Instrutor != nil {
			r.IDUsuario = m.Instrutor.IDUsuario
			r.Vinculado = d.IDAcademia != nil && m.Instrutor.IDAcademia != nil && *m.Instrutor.IDAcademia == *d.IDAcademia
			if m.Instrutor.Usuario != nil {
				r.Nome = strings.TrimSpace(m.Instrutor.Usuario.Nome + " " + m.Instrutor.Usuario.Sobrenome)
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Responsavel && !out[j].Responsavel })
	return out
}
