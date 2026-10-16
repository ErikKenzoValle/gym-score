package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"gorm.io/gorm"
)

// Prazo mínimo (após abrir a votação) para o criador apurar sem a maioria dos votos.
const prazoApuracaoSemQuorum = 24 * time.Hour

// DesafioService define as operações de negócio para desafios. Todas recebem o
// userID do JWT: a identidade nunca vem do corpo da requisição.
type DesafioService interface {
	Criar(userID uint, req *models.CriarDesafioRequest) (*models.DesafioResponse, error)
	Listar(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error)
	ListarMeus(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error)
	Detalhe(userID, id uint) (*models.DesafioResponse, error)
	Entrar(userID, id uint) (*models.DesafioResponse, error)
	Sair(userID, id uint) (*models.DesafioResponse, error)
	Cancelar(userID, id uint) (*models.DesafioResponse, error)
	Iniciar(userID, id uint) (*models.DesafioResponse, error)
	AbrirVotacao(userID, id uint) (*models.DesafioResponse, error)
	Votar(userID, id, idVotado uint) (*models.DesafioResponse, error)
	Apurar(userID, id uint) (*models.DesafioResponse, error)
	Convidar(userID, id, idAmigo uint) error
	Checkin(userID, id uint, in CheckinInput) (*models.CheckinResponse, error)
	ListarCheckins(userID, id uint) ([]models.CheckinResponse, error)
}

type desafioService struct {
	db       *gorm.DB
	desafios *repositories.DesafioRepository
	carteira *repositories.CarteiraRepository
	amizades repositories.AmizadeRepository
	notif    Notificador
	fotos    FotoStorage
	agora    func() time.Time
}

// NovoDesafioService cria uma nova instância do serviço de desafios
func NovoDesafioService(
	db *gorm.DB,
	desafios *repositories.DesafioRepository,
	carteira *repositories.CarteiraRepository,
	amizades repositories.AmizadeRepository,
	notif Notificador,
	fotos FotoStorage,
) DesafioService {
	return &desafioService{db: db, desafios: desafios, carteira: carteira, amizades: amizades,
		notif: notif, fotos: fotos, agora: time.Now}
}

// NovoDesafioServiceComRelogio permite injetar o relógio (testes de prazo de apuração).
func NovoDesafioServiceComRelogio(
	db *gorm.DB,
	desafios *repositories.DesafioRepository,
	carteira *repositories.CarteiraRepository,
	amizades repositories.AmizadeRepository,
	notif Notificador,
	fotos FotoStorage,
	agora func() time.Time,
) DesafioService {
	s := NovoDesafioService(db, desafios, carteira, amizades, notif, fotos).(*desafioService)
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

// parseDataEncerramento aceita "YYYY-MM-DD" (fim do dia, horário local) ou RFC3339.
func parseDataEncerramento(s string, agora time.Time) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	var t time.Time
	if d, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		t = d.Add(24*time.Hour - time.Millisecond)
	} else if d, err := time.Parse(time.RFC3339, s); err == nil {
		t = d
	} else {
		return nil, erroRequisicao("data_encerramento inválida (use YYYY-MM-DD)")
	}
	if !t.After(agora) {
		return nil, erroRequisicao("data_encerramento deve estar no futuro")
	}
	return &t, nil
}

// Criar cria o desafio, debita a entrada do criador (ledger) e o inscreve — tudo numa transação.
func (s *desafioService) Criar(userID uint, req *models.CriarDesafioRequest) (*models.DesafioResponse, error) {
	if req.ValorEntradaCentavos < models.EntradaMinCentavos || req.ValorEntradaCentavos > models.EntradaMaxCentavos {
		return nil, erroRequisicao("Valor de entrada fora do permitido (R$ 1,00 a R$ 10.000,00)")
	}
	vagas := req.Vagas
	if vagas < 2 {
		vagas = 2
	}
	encerramento, err := parseDataEncerramento(req.DataEncerramento, s.agora())
	if err != nil {
		return nil, err
	}

	desafio := &models.Desafio{
		Titulo:               req.Titulo,
		ValorEntradaCentavos: req.ValorEntradaCentavos,
		Latitude:             req.Latitude,
		Longitude:            req.Longitude,
		IDCriador:            userID,
		Vagas:                vagas,
		Status:               models.StatusAberto,
		DataEncerramento:     encerramento,
	}
	if req.Descricao != "" {
		desafio.Descricao = &req.Descricao
	}
	if req.Local != "" {
		desafio.Local = &req.Local
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		repo := s.desafios.ComTx(tx)
		if err := repo.Criar(desafio); err != nil {
			return err
		}
		if _, err := s.carteira.Debitar(tx, repositories.Lancamento{
			IDUsuario: userID, Tipo: models.TipoEntradaDesafio, ValorCentavos: desafio.ValorEntradaCentavos,
			IDDesafio: &desafio.ID, Descricao: "Entrada no desafio: " + desafio.Titulo,
		}); err != nil {
			return err
		}
		return repo.AdicionarParticipante(&models.DesafioParticipante{IDDesafio: desafio.ID, IDUsuario: userID})
	})
	if err != nil {
		return nil, traduzir(err)
	}
	return s.Detalhe(userID, desafio.ID)
}

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

func (s *desafioService) acao(userID, id uint, fn func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error) (*models.DesafioResponse, error) {
	if err := s.comDesafioTravado(id, fn); err != nil {
		return nil, err
	}
	return s.Detalhe(userID, id)
}

// Entrar debita a entrada e ocupa uma vaga (atômico; a trava do desafio evita estourar as vagas).
func (s *desafioService) Entrar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if d.Status != models.StatusAberto {
			return erroConflito("Desafio não está aberto para inscrições")
		}
		if jaParticipa, err := repo.UsuarioParticipa(d.ID, userID); err != nil {
			return err
		} else if jaParticipa {
			return erroConflito("Você já está participando deste desafio")
		}
		inscritos, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		if int(inscritos) >= d.Vagas {
			return erroConflito("Não há mais vagas neste desafio")
		}
		if _, err := s.carteira.Debitar(tx, repositories.Lancamento{
			IDUsuario: userID, Tipo: models.TipoEntradaDesafio, ValorCentavos: d.ValorEntradaCentavos,
			IDDesafio: &d.ID, Descricao: "Entrada no desafio: " + d.Titulo,
		}); err != nil {
			return err
		}
		if err := repo.AdicionarParticipante(&models.DesafioParticipante{IDDesafio: d.ID, IDUsuario: userID}); err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("Você já está participando deste desafio")
			}
			return err
		}
		return nil
	})
}

// Sair devolve a entrada e libera a vaga. O criador não pode sair (deve cancelar).
func (s *desafioService) Sair(userID, id uint) (*models.DesafioResponse, error) {
	if err := s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if d.IDCriador == userID {
			return erroConflito("O criador não pode sair do desafio; cancele-o")
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
		_, err = s.carteira.Creditar(tx, repositories.Lancamento{
			IDUsuario: userID, Tipo: models.TipoEstornoDesafio, ValorCentavos: d.ValorEntradaCentavos,
			IDDesafio: &d.ID, Descricao: "Reembolso (saída do desafio): " + d.Titulo,
		})
		return err
	}); err != nil {
		return nil, err
	}
	// Quem saiu não participa mais: devolve o desafio aberto visto por ele
	return s.Detalhe(userID, id)
}

func exigirCriador(d *models.Desafio, userID uint) error {
	if d.IDCriador != userID {
		return erroProibido("Apenas o criador pode realizar esta ação")
	}
	return nil
}

// Cancelar (só criador, só aberto) cancela e reembolsa integralmente todos os participantes.
func (s *desafioService) Cancelar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirCriador(d, userID); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Apenas desafios abertos podem ser cancelados")
		}
		mudou, err := repo.MudarStatus(d.ID, models.StatusAberto, models.StatusCancelado, nil)
		if err != nil {
			return err
		}
		if !mudou {
			return erroConflito("Apenas desafios abertos podem ser cancelados")
		}
		participantes, err := repo.ListarParticipantes(d.ID)
		if err != nil {
			return err
		}
		for _, p := range participantes { // ordenados por id_usuario (ordem de locks estável)
			if _, err := s.carteira.Creditar(tx, repositories.Lancamento{
				IDUsuario: p.IDUsuario, Tipo: models.TipoEstornoDesafio, ValorCentavos: d.ValorEntradaCentavos,
				IDDesafio: &d.ID, Descricao: "Reembolso (desafio cancelado): " + d.Titulo,
			}); err != nil {
				return err
			}
		}
		ids, titulo := idsExceto(participantes, d.IDCriador), d.Titulo
		ev.add(func() {
			s.notif.NotificarVarios(ids, models.NotifDesafioCancel, "Desafio cancelado",
				fmt.Sprintf("O desafio \"%s\" foi cancelado e sua entrada foi reembolsada.", titulo), linkDesafio(d.ID))
		})
		return nil
	})
}

// Iniciar (só criador, aberto, >= 2 participantes) move para em_andamento.
func (s *desafioService) Iniciar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirCriador(d, userID); err != nil {
			return err
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Apenas desafios abertos podem ser iniciados")
		}
		n, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		if n < 2 {
			return erroConflito("São necessários pelo menos 2 participantes para iniciar")
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
		ids, titulo := idsExceto(ps, d.IDCriador), d.Titulo
		ev.add(func() {
			s.notif.NotificarVarios(ids, models.NotifDesafioIniciado, "Desafio iniciado",
				fmt.Sprintf("O desafio \"%s\" começou. Bora treinar!", titulo), linkDesafio(d.ID))
		})
		return nil
	})
}

// AbrirVotacao (só criador, em_andamento) move para em_votacao e registra o horário.
func (s *desafioService) AbrirVotacao(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirCriador(d, userID); err != nil {
			return err
		}
		if d.Status != models.StatusEmAndamento {
			return erroConflito("A votação só pode ser aberta em desafios em andamento")
		}
		mudou, err := repo.MudarStatus(d.ID, models.StatusEmAndamento, models.StatusEmVotacao,
			map[string]any{"votacao_aberta_em": s.agora()})
		if err != nil {
			return err
		}
		if !mudou {
			return erroConflito("A votação só pode ser aberta em desafios em andamento")
		}
		ps, err := repo.ListarParticipantes(d.ID)
		if err != nil {
			return err
		}
		ids, titulo := idsExceto(ps, d.IDCriador), d.Titulo
		ev.add(func() {
			s.notif.NotificarVarios(ids, models.NotifVotacaoAberta, "Votação aberta",
				fmt.Sprintf("A votação do desafio \"%s\" foi aberta. Vote em quem mais se destacou.", titulo), linkDesafio(d.ID))
		})
		return nil
	})
}

// Votar registra o voto (1 por participante, em outro participante). Quando todos
// votaram, apura automaticamente na mesma transação.
func (s *desafioService) Votar(userID, id, idVotado uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if d.Status != models.StatusEmVotacao {
			return erroConflito("O desafio não está em votação")
		}
		if idVotado == userID {
			return erroRequisicao("Você não pode votar em si mesmo")
		}
		if ok, err := repo.UsuarioParticipa(d.ID, userID); err != nil {
			return err
		} else if !ok {
			return erroProibido("Apenas participantes podem votar")
		}
		if ok, err := repo.UsuarioParticipa(d.ID, idVotado); err != nil {
			return err
		} else if !ok {
			return erroRequisicao("O voto deve ser em outro participante do desafio")
		}
		if err := repo.InserirVoto(&models.DesafioVoto{IDDesafio: d.ID, IDEleitor: userID, IDVotado: idVotado}); err != nil {
			if errors.Is(err, repositories.ErrVotoDuplicado) {
				return erroConflito("Você já votou neste desafio")
			}
			return err
		}
		votos, err := repo.ContarVotos(d.ID)
		if err != nil {
			return err
		}
		n, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		if votos >= n {
			return s.apurarNaTransacao(tx, repo, d, ev)
		}
		return nil
	})
}

// Apurar (só criador, em_votacao) encerra antecipadamente: exige metade+1 dos votos, ou
// que a votação esteja aberta há 24h, ou que data_encerramento já tenha passado.
func (s *desafioService) Apurar(userID, id uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirCriador(d, userID); err != nil {
			return err
		}
		if d.Status != models.StatusEmVotacao {
			return erroConflito("O desafio não está em votação")
		}
		votos, err := repo.ContarVotos(d.ID)
		if err != nil {
			return err
		}
		n, err := repo.ContarParticipantes(d.ID)
		if err != nil {
			return err
		}
		agora := s.agora()
		quorum := votos >= n/2+1
		prazoVotacao := d.VotacaoAbertaEm != nil && !agora.Before(d.VotacaoAbertaEm.Add(prazoApuracaoSemQuorum))
		prazoDesafio := d.DataEncerramento != nil && !agora.Before(*d.DataEncerramento)
		if !quorum && !prazoVotacao && !prazoDesafio {
			return erroConflito(fmt.Sprintf(
				"Ainda não é possível apurar: faltam votos (%d de %d necessários) e o prazo não terminou", votos, n/2+1))
		}
		return s.apurarNaTransacao(tx, repo, d, ev)
	})
}

// apurarNaTransacao encerra o desafio e credita os prêmios (chamar com o desafio travado).
func (s *desafioService) apurarNaTransacao(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
	participantes, err := repo.ListarParticipantes(d.ID)
	if err != nil {
		return err
	}
	votos, err := repo.ListarVotos(d.ID)
	if err != nil {
		return err
	}
	contagem := map[uint]int{}
	for _, v := range votos {
		contagem[v.IDVotado]++
	}
	ids := make([]uint, len(participantes))
	for i, p := range participantes {
		ids[i] = p.IDUsuario
	}
	pote := d.ValorEntradaCentavos * int64(len(participantes))

	mudou, err := repo.MudarStatus(d.ID, models.StatusEmVotacao, models.StatusEncerrado, nil)
	if err != nil {
		return err
	}
	if !mudou {
		return erroConflito("O desafio já foi apurado")
	}
	premios := CalcularApuracao(ids, contagem, pote)
	ganhou := map[uint]int64{}
	for _, p := range premios {
		ganhou[p.IDUsuario] = p.Centavos
	}
	titulo := d.Titulo
	ev.add(func() {
		for _, id := range ids {
			if v, ok := ganhou[id]; ok {
				s.notif.Notificar(id, models.NotifResultado, "Resultado do desafio",
					fmt.Sprintf("Você venceu \"%s\" e recebeu %s!", titulo, reaisBR(v)), linkDesafio(d.ID))
			} else {
				s.notif.Notificar(id, models.NotifResultado, "Resultado do desafio",
					fmt.Sprintf("O desafio \"%s\" foi encerrado. Veja o resultado.", titulo), linkDesafio(d.ID))
			}
		}
	})
	for _, premio := range premios {
		if err := repo.RegistrarPremio(d.ID, premio.IDUsuario, premio.Centavos); err != nil {
			return err
		}
		if premio.Centavos == 0 {
			continue
		}
		if _, err := s.carteira.Creditar(tx, repositories.Lancamento{
			IDUsuario: premio.IDUsuario, Tipo: models.TipoPremioDesafio, ValorCentavos: premio.Centavos,
			IDDesafio: &d.ID, Descricao: "Prêmio do desafio: " + d.Titulo,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Convidar (só criador, desafio aberto) valida que o alvo é amigo aceito e ainda não participa.
// O envio da notificação in-app é da Fase B.
func (s *desafioService) Convidar(userID, id, idAmigo uint) error {
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return err
	}
	if d == nil {
		return erroNaoEncontrado("Desafio não encontrado")
	}
	if err := exigirCriador(d, userID); err != nil {
		return err
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
	if d.Criador != nil {
		nome = strings.TrimSpace(d.Criador.Nome + " " + d.Criador.Sobrenome)
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

func (s *desafioService) listar(userID uint, status models.StatusDesafio, pagina, limite int, meus bool) (*models.ListaDesafiosResponse, error) {
	pagina, limite = normalizarPagina(pagina, limite)
	desafios, total, err := s.desafios.Listar(repositories.FiltroDesafios{
		IDUsuario: userID, Status: status, SomenteMeus: meus, Pagina: pagina, Limite: limite,
	})
	if err != nil {
		return nil, err
	}
	itens := make([]models.DesafioResponse, 0, len(desafios))
	for i := range desafios {
		itens = append(itens, *montarResposta(&desafios[i], userID, nil))
	}
	return &models.ListaDesafiosResponse{Itens: itens, Total: total, Pagina: pagina, Limite: limite}, nil
}

// Listar devolve desafios abertos com vaga + os do próprio usuário (paginado no SQL).
func (s *desafioService) Listar(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error) {
	return s.listar(userID, status, pagina, limite, false)
}

// ListarMeus devolve os desafios em que o usuário criou ou participa (qualquer status).
func (s *desafioService) ListarMeus(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error) {
	return s.listar(userID, status, pagina, limite, true)
}

// Detalhe devolve o desafio. Desafios não abertos só são visíveis a quem participa.
func (s *desafioService) Detalhe(userID, id uint) (*models.DesafioResponse, error) {
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	if d.Status != models.StatusAberto && !participa(d, userID) {
		return nil, erroProibido("Você não participa deste desafio")
	}
	var votos []models.DesafioVoto
	if d.Status == models.StatusEmVotacao || d.Status == models.StatusEncerrado {
		if votos, err = s.desafios.ListarVotos(id); err != nil {
			return nil, err
		}
		if votos == nil {
			votos = []models.DesafioVoto{}
		}
	}
	resp := montarResposta(d, userID, votos)
	if d.Status != models.StatusAberto { // check-ins só existem depois do início
		resumo, err := s.desafios.ResumoCheckins(id)
		if err != nil {
			return nil, err
		}
		for i := range resp.Participantes {
			if r, ok := resumo[resp.Participantes[i].ID]; ok {
				resp.Participantes[i].Checkins = r.Total
				resp.Participantes[i].UltimoCheckin = r.Ultimo
				resp.CheckinsTotal += r.Total
			}
		}
		if resp.VoceParticipa {
			if resp.VoceFezCheckinHoje, err = s.desafios.CheckinNoDia(id, userID, s.agora().Format("2006-01-02")); err != nil {
				return nil, err
			}
		}
	}
	return resp, nil
}

func participa(d *models.Desafio, userID uint) bool {
	if d.IDCriador == userID {
		return true
	}
	for _, p := range d.Participantes {
		if p.IDUsuario == userID {
			return true
		}
	}
	return false
}

// montarResposta converte o model no DTO. votos == nil significa "não carregados" (listas).
func montarResposta(d *models.Desafio, userID uint, votos []models.DesafioVoto) *models.DesafioResponse {
	resp := &models.DesafioResponse{
		ID:                   d.ID,
		Titulo:               d.Titulo,
		ValorEntradaCentavos: d.ValorEntradaCentavos,
		ValorEntrada:         utils.CentavosParaReais(d.ValorEntradaCentavos),
		Latitude:             d.Latitude,
		Longitude:            d.Longitude,
		Status:               d.Status,
		Vagas:                d.Vagas,
		ParticipantesCount:   len(d.Participantes),
		DataEncerramento:     d.DataEncerramento,
		VotacaoAbertaEm:      d.VotacaoAbertaEm,
		IDCriador:            d.IDCriador,
		CriadoEm:             d.CriadoEm,
		VoceECriador:         d.IDCriador == userID,
		Participantes:        make([]models.ParticipanteResponse, 0, len(d.Participantes)),
	}
	if d.Descricao != nil {
		resp.Descricao = *d.Descricao
	}
	if d.Local != nil {
		resp.Local = *d.Local
	}
	if d.Criador != nil {
		pub := d.Criador.Publico()
		resp.Criador = &pub
	}
	resp.PoteCentavos = d.ValorEntradaCentavos * int64(len(d.Participantes))
	resp.Pote = utils.CentavosParaReais(resp.PoteCentavos)

	recebidos := map[uint]int{}
	jaVotaram := map[uint]bool{}
	for _, v := range votos {
		recebidos[v.IDVotado]++
		jaVotaram[v.IDEleitor] = true
	}
	resp.VoceVotou = jaVotaram[userID]

	for _, p := range d.Participantes {
		pr := models.ParticipanteResponse{
			Criador:        p.IDUsuario == d.IDCriador,
			PremioCentavos: p.PremioCentavos,
			Premio:         utils.CentavosParaReais(p.PremioCentavos),
			Vencedor:       p.Vencedor,
		}
		if p.Usuario != nil {
			pr.UsuarioPublico = p.Usuario.Publico()
		} else {
			pr.ID = p.IDUsuario
		}
		if p.IDUsuario == userID {
			resp.VoceParticipa = true
		}
		switch d.Status {
		case models.StatusEncerrado:
			n := recebidos[p.IDUsuario]
			pr.VotosRecebidos = &n
		case models.StatusEmVotacao:
			j := jaVotaram[p.IDUsuario]
			pr.JaVotou = &j
		}
		resp.Participantes = append(resp.Participantes, pr)
	}
	return resp
}

// ─── Check-ins ───────────────────────────────────────────────────────────────

// CheckinInput são os dados de um check-in. Foto == nil significa sem foto.
type CheckinInput struct {
	Texto     string
	Latitude  *float64
	Longitude *float64
	Foto      []byte
}

// TamanhoMaxFoto é o limite do upload (3 MB).
const TamanhoMaxFoto = 3 << 20

// Checkin registra a prova de treino do participante: só em_andamento, 1 por dia.
func (s *desafioService) Checkin(userID, id uint, in CheckinInput) (*models.CheckinResponse, error) {
	in.Texto = strings.TrimSpace(in.Texto)
	if len([]rune(in.Texto)) > 500 {
		return nil, erroRequisicao("O texto do check-in deve ter no máximo 500 caracteres")
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return nil, erroRequisicao("Informe latitude e longitude juntas")
	}
	if in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180) {
		return nil, erroRequisicao("Coordenadas fora do intervalo válido")
	}
	ext := ""
	if in.Foto != nil {
		if len(in.Foto) > TamanhoMaxFoto {
			return nil, erro(413, "A foto deve ter no máximo 3 MB")
		}
		var ok bool
		if ext, ok = DetectarImagem(in.Foto); !ok {
			return nil, erroRequisicao("Formato de imagem inválido (use JPG, PNG ou WEBP)")
		}
	}

	var checkin *models.DesafioCheckin
	err := s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if ok, err := repo.UsuarioParticipa(d.ID, userID); err != nil {
			return err
		} else if !ok {
			return erroProibido("Apenas participantes podem fazer check-in")
		}
		if d.Status != models.StatusEmAndamento {
			return erroConflito("Check-ins só são permitidos com o desafio em andamento")
		}
		dia := s.agora().Format("2006-01-02")
		if ja, err := repo.CheckinNoDia(d.ID, userID, dia); err != nil {
			return err
		} else if ja {
			return erroConflito("Você já fez o check-in de hoje neste desafio")
		}
		c := &models.DesafioCheckin{IDDesafio: d.ID, IDUsuario: userID, Data: dia, Latitude: in.Latitude, Longitude: in.Longitude}
		if in.Texto != "" {
			c.Texto = &in.Texto
		}
		var salvo string
		if in.Foto != nil {
			p, err := s.fotos.Salvar(in.Foto, ext)
			if err != nil {
				return err
			}
			salvo = p
			c.FotoPath = &p
		}
		if err := repo.InserirCheckin(c); err != nil {
			if salvo != "" {
				s.fotos.Remover(salvo)
			}
			if errors.Is(err, repositories.ErrCheckinDuplicado) {
				return erroConflito("Você já fez o check-in de hoje neste desafio")
			}
			return err
		}
		checkin = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	var u models.Usuario
	if err := s.db.Select("id", "nome", "sobrenome").First(&u, userID).Error; err != nil {
		return nil, err
	}
	checkin.Usuario = &u
	return paraCheckinResponse(checkin), nil
}

// ListarCheckins devolve os check-ins do desafio (somente criador/participantes).
func (s *desafioService) ListarCheckins(userID, id uint) ([]models.CheckinResponse, error) {
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	if !participa(d, userID) {
		return nil, erroProibido("Você não participa deste desafio")
	}
	cs, err := s.desafios.ListarCheckins(id)
	if err != nil {
		return nil, err
	}
	out := make([]models.CheckinResponse, 0, len(cs))
	for i := range cs {
		out = append(out, *paraCheckinResponse(&cs[i]))
	}
	return out, nil
}

func paraCheckinResponse(c *models.DesafioCheckin) *models.CheckinResponse {
	r := &models.CheckinResponse{ID: c.ID, Latitude: c.Latitude, Longitude: c.Longitude, Data: c.Data, CriadoEm: c.CriadoEm}
	if len(r.Data) > 10 {
		r.Data = r.Data[:10]
	}
	if c.Usuario != nil {
		r.Usuario = c.Usuario.Publico()
	} else {
		r.Usuario.ID = c.IDUsuario
	}
	if c.Texto != nil {
		r.Texto = *c.Texto
	}
	if c.FotoPath != nil {
		r.FotoURL = "/" + *c.FotoPath
	}
	return r
}
