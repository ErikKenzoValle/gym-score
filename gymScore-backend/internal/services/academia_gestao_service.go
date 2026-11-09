package services

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"gorm.io/gorm"
)

// AcademiaGestaoService reúne o painel da academia logada: seus desafios, o dashboard e o vínculo
// de instrutores. Toda operação resolve a academia pelo JWT (nunca por id vindo do cliente), então
// uma academia só enxerga e altera os próprios dados.
type AcademiaGestaoService interface {
	ListarDesafios(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error)
	Dashboard(userID uint) (*models.DashboardAcademiaResponse, error)
	ListarInstrutores(userID uint, pagina, limite int) (*models.ListaInstrutoresVinculadosResponse, error)
	// VincularInstrutor envia um CONVITE de vínculo (o instrutor aceita ou recusa).
	VincularInstrutor(userID uint, email string) (*models.InstrutorVinculadoResponse, error)
	ListarConvites(userID uint) ([]models.InstrutorVinculadoResponse, error)
	CancelarConvite(userID, idInstrutor uint) error
	DesvincularInstrutor(userID, idInstrutor uint) error
	// Perfil devolve os dados cadastrais da academia logada (inclui o endereço estruturado).
	Perfil(userID uint) (*models.Academia, error)
	// AtualizarPerfil grava o endereço estruturado (monta o texto `endereco`) e, opcional, o telefone.
	AtualizarPerfil(userID uint, req *models.AtualizarAcademiaRequest) (*models.Academia, error)
}

type academiaGestaoService struct {
	db        *gorm.DB
	academias repositories.AcademiaRepository
	desafios  *repositories.DesafioRepository
	painel    *repositories.PainelRepository
	vinculos  *repositories.VinculoInstrutorRepository
	notif     Notificador
	agora     func() time.Time
}

// NovoAcademiaGestaoService cria o serviço de gestão da academia.
func NovoAcademiaGestaoService(
	db *gorm.DB,
	academias repositories.AcademiaRepository,
	desafios *repositories.DesafioRepository,
	painel *repositories.PainelRepository,
	vinculos *repositories.VinculoInstrutorRepository,
	notif Notificador,
) AcademiaGestaoService {
	return &academiaGestaoService{db: db, academias: academias, desafios: desafios, painel: painel,
		vinculos: vinculos, notif: notif, agora: time.Now}
}

// statusAtivos são os estados em que o desafio ainda "ocupa" o instrutor.
var statusAtivos = []models.StatusDesafio{models.StatusAberto, models.StatusEmAndamento}

// maxDesafiosNoBloqueio limita quantos desafios aparecem na mensagem de desvínculo bloqueado.
const maxDesafiosNoBloqueio = 3

func statusValido(s models.StatusDesafio) bool {
	switch s {
	case models.StatusAberto, models.StatusEmAndamento, models.StatusEncerrado, models.StatusCancelado:
		return true
	}
	return false
}

// academiaDe devolve a academia da conta logada (403 se a conta não tem registro de academia).
func (s *academiaGestaoService) academiaDe(userID uint) (*models.Academia, error) {
	a, err := s.academias.BuscarPorUsuario(userID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, erroProibido("Conta de academia não encontrada")
	}
	if a.Status != models.CadastroAprovado {
		return nil, ErroCadastroNaoAprovado
	}
	return a, nil
}

// respostasDe converte os desafios no DTO público calculando as flags para o contexto informado.
// Único ponto da gestão que depende de montarResposta.
func (s *academiaGestaoService) respostasDe(ds []models.Desafio, ctx *contextoUsuario) []models.DesafioResponse {
	agora := s.agora()
	itens := make([]models.DesafioResponse, 0, len(ds))
	for i := range ds {
		itens = append(itens, *montarResposta(&ds[i], ctx, false, agora))
	}
	return itens
}

// ListarDesafios devolve os desafios DA academia logada, paginados, com filtro opcional de status.
// Por padrão (sem status) todos os status, do evento mais recente ao mais antigo; abertos e em
// andamento saem do evento mais próximo ao mais distante.
func (s *academiaGestaoService) ListarDesafios(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error) {
	a, err := s.academiaDe(userID)
	if err != nil {
		return nil, err
	}
	if status != "" && !statusValido(status) {
		return nil, erroRequisicao("status inválido (use aberto, em_andamento, encerrado ou cancelado)")
	}
	pagina, limite = normalizarPagina(pagina, limite)
	filtro := repositories.FiltroDesafios{
		IDUsuario: userID, Todos: true, IDAcademia: a.ID, Status: status,
		Agora: s.agora(), Pagina: pagina, Limite: limite, Ordem: repositories.OrdemEventoRec,
	}
	if status == models.StatusAberto || status == models.StatusEmAndamento {
		filtro.Ordem = repositories.OrdemEventoProx
	}
	ds, total, err := s.desafios.Listar(filtro)
	if err != nil {
		return nil, err
	}
	ctx := &contextoUsuario{ID: userID, Perfil: "academia", AcademiaID: a.ID, AcademiaAprovada: true}
	return &models.ListaDesafiosResponse{Itens: s.respostasDe(ds, ctx), Total: total, Pagina: pagina, Limite: limite}, nil
}

// Dashboard calcula os indicadores só com os dados da academia logada.
func (s *academiaGestaoService) Dashboard(userID uint) (*models.DashboardAcademiaResponse, error) {
	a, err := s.academiaDe(userID)
	if err != nil {
		return nil, err
	}
	agora := s.agora()
	resp := &models.DashboardAcademiaResponse{
		Academia: models.AcademiaResumo{ID: a.ID, NomeFantasia: a.NomeFantasia},
		DesafiosPorStatus: map[models.StatusDesafio]int64{
			models.StatusAberto: 0, models.StatusEmAndamento: 0, models.StatusEncerrado: 0, models.StatusCancelado: 0,
		},
		ProximosEventos: []models.DesafioResponse{},
	}

	porStatus, err := s.painel.ContagemPorStatus(a.ID)
	if err != nil {
		return nil, err
	}
	for st, n := range porStatus {
		resp.DesafiosPorStatus[st] = n
		resp.TotalDesafios += n
	}

	inicioMes := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, agora.Location())
	if resp.InscritosMes, err = s.painel.InscritosDesde(a.ID, inicioMes); err != nil {
		return nil, err
	}
	if resp.PoteEmJogoCentavos, err = s.painel.PoteEmJogo(a.ID); err != nil {
		return nil, err
	}
	resp.PoteEmJogo = utils.CentavosParaReais(resp.PoteEmJogoCentavos)

	presentes, inscritos, err := s.painel.Presenca(a.ID)
	if err != nil {
		return nil, err
	}
	resp.TaxaPresenca = percentual(presentes, inscritos)

	media, n, err := s.painel.AvaliacaoMedia(a.ID)
	if err != nil {
		return nil, err
	}
	resp.AvaliacaoMedia, resp.AvaliacoesTotal = math.Round(media*100)/100, n

	if resp.InstrutoresVinculados, err = s.vinculos.ContarPorAcademia(a.ID); err != nil {
		return nil, err
	}

	prox, _, err := s.desafios.Listar(repositories.FiltroDesafios{
		IDUsuario: userID, Todos: true, IDAcademia: a.ID, Status: models.StatusAberto, Futuros: true,
		Agora: agora, Ordem: repositories.OrdemEventoProx, Pagina: 1, Limite: 5,
	})
	if err != nil {
		return nil, err
	}
	resp.ProximosEventos = s.respostasDe(prox, &contextoUsuario{ID: userID, Perfil: "academia", AcademiaID: a.ID, AcademiaAprovada: true})
	return resp, nil
}

// percentual devolve parte/total em % com uma casa decimal (0 quando total == 0).
func percentual(parte, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(parte)*1000/float64(total)) / 10
}

func paraInstrutorVinculado(i *models.Instrutor, ativos int) models.InstrutorVinculadoResponse {
	r := models.InstrutorVinculadoResponse{
		ID: i.ID, IDUsuario: i.IDUsuario, CREF: i.CREF, Especialidade: i.Especialidade, DesafiosAtivos: ativos,
		ConvitePendente: i.IDAcademia == nil && i.IDAcademiaConvite != nil, ConviteEm: i.ConviteEm,
	}
	if i.Usuario != nil {
		r.Nome, r.Sobrenome, r.Email = i.Usuario.Nome, i.Usuario.Sobrenome, i.Usuario.Email
	}
	return r
}

// ListarInstrutores devolve os instrutores vinculados à academia logada.
func (s *academiaGestaoService) ListarInstrutores(userID uint, pagina, limite int) (*models.ListaInstrutoresVinculadosResponse, error) {
	a, err := s.academiaDe(userID)
	if err != nil {
		return nil, err
	}
	pagina, limite = normalizarPagina(pagina, limite)
	lista, total, err := s.vinculos.ListarPorAcademia(a.ID, pagina, limite)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, len(lista))
	for i := range lista {
		ids[i] = lista[i].ID
	}
	ativos, err := s.vinculos.ContarDesafiosAtivos(ids)
	if err != nil {
		return nil, err
	}
	itens := make([]models.InstrutorVinculadoResponse, 0, len(lista))
	for i := range lista {
		itens = append(itens, paraInstrutorVinculado(&lista[i], ativos[lista[i].ID]))
	}
	return &models.ListaInstrutoresVinculadosResponse{Itens: itens, Total: total, Pagina: pagina, Limite: limite}, nil
}

// VincularInstrutor CONVIDA a conta de instrutor com esse e-mail; o vínculo só existe depois que o
// instrutor aceitar (POST /api/instrutor/convite/aceitar). Só instrutor sem vínculo e sem convite
// pendente de OUTRA academia pode ser convidado (UPDATE condicional: duas academias disputando o mesmo
// instrutor, só uma vence). Convidar de novo o mesmo instrutor renova o convite.
func (s *academiaGestaoService) VincularInstrutor(userID uint, email string) (*models.InstrutorVinculadoResponse, error) {
	a, err := s.academiaDe(userID)
	if err != nil {
		return nil, err
	}
	email = strings.TrimSpace(email)
	if !utils.ValidarEmail(email) {
		return nil, erroRequisicao("e-mail inválido")
	}
	i, err := s.vinculos.BuscarPorEmail(email)
	if err != nil {
		return nil, err
	}
	if i == nil {
		return nil, erroNaoEncontrado("Nenhum instrutor cadastrado com este e-mail")
	}
	if i.IDAcademia != nil && *i.IDAcademia == a.ID {
		return nil, erroConflito("Este instrutor já está vinculado à sua academia")
	}
	errOutra := erroConflito("Este instrutor já está vinculado a outra academia ou tem convite pendente de outra academia")
	if i.IDAcademia != nil {
		return nil, errOutra
	}
	agora := s.agora()
	ok, err := s.vinculos.Convidar(i.ID, a.ID, agora)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errOutra
	}
	s.notif.Notificar(i.IDUsuario, models.NotifConviteVinculo, "Convite de academia",
		fmt.Sprintf("A academia %s convidou você para ser instrutor dela. Aceite ou recuse o convite na sua agenda.", a.NomeFantasia),
		"/instrutor")
	i.IDAcademiaConvite, i.ConviteEm = &a.ID, &agora
	resp := paraInstrutorVinculado(i, 0)
	return &resp, nil
}

// ListarConvites devolve os convites de vínculo pendentes enviados pela academia logada.
func (s *academiaGestaoService) ListarConvites(userID uint) ([]models.InstrutorVinculadoResponse, error) {
	a, err := s.academiaDe(userID)
	if err != nil {
		return nil, err
	}
	lista, err := s.vinculos.ListarConvitesDaAcademia(a.ID)
	if err != nil {
		return nil, err
	}
	out := make([]models.InstrutorVinculadoResponse, 0, len(lista))
	for i := range lista {
		out = append(out, paraInstrutorVinculado(&lista[i], 0))
	}
	return out, nil
}

// CancelarConvite retira o convite pendente da academia logada para o instrutor (404 se não houver).
func (s *academiaGestaoService) CancelarConvite(userID, idInstrutor uint) error {
	a, err := s.academiaDe(userID)
	if err != nil {
		return err
	}
	ok, err := s.vinculos.LimparConvite(idInstrutor, a.ID)
	if err != nil {
		return err
	}
	if !ok {
		return erroNaoEncontrado("Convite não encontrado")
	}
	return nil
}

// DesvincularInstrutor remove o vínculo e avisa o instrutor. Bloqueado (409) enquanto ele for o
// instrutor designado de algum desafio aberto ou em andamento: a academia precisa trocá-lo antes.
// Instrutor inexistente ou de outra academia responde 404 igual (não revela vínculo alheio).
func (s *academiaGestaoService) DesvincularInstrutor(userID, idInstrutor uint) error {
	a, err := s.academiaDe(userID)
	if err != nil {
		return err
	}
	var avisar uint
	err = s.db.Transaction(func(tx *gorm.DB) error {
		repo := s.vinculos.ComTx(tx)
		i, err := repo.TravarPorID(idInstrutor)
		if err != nil {
			return err
		}
		if i == nil || i.IDAcademia == nil || *i.IDAcademia != a.ID {
			return erroNaoEncontrado("Instrutor não encontrado entre os vinculados à sua academia")
		}
		ativos, err := repo.DesafiosAtivosDe(i.ID, maxDesafiosNoBloqueio+1)
		if err != nil {
			return err
		}
		if len(ativos) > 0 {
			return erroConflito(mensagemBloqueioDesvinculo(ativos))
		}
		ok, err := repo.Desvincular(i.ID, a.ID)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("O vínculo do instrutor mudou. Atualize a lista e tente de novo")
		}
		avisar = i.IDUsuario
		return nil
	})
	if err != nil {
		return err
	}
	s.notif.Notificar(avisar, models.NotifInstrutorRemovido, "Vínculo com a academia removido",
		fmt.Sprintf("A academia %s removeu seu vínculo como instrutor. Você segue com a conta de instrutor autônomo.", a.NomeFantasia),
		"/instrutor")
	return nil
}

func mensagemBloqueioDesvinculo(ativos []models.Desafio) string {
	partes := make([]string, 0, maxDesafiosNoBloqueio)
	for i, d := range ativos {
		if i == maxDesafiosNoBloqueio {
			partes = append(partes, "...")
			break
		}
		partes = append(partes, fmt.Sprintf("#%d %s", d.ID, d.Titulo))
	}
	return "O instrutor ainda é o responsável por desafio(s) aberto(s) ou em andamento (" +
		strings.Join(partes, ", ") + "). Troque o instrutor desses desafios ou aguarde o encerramento antes de desvincular"
}

// Perfil devolve os dados cadastrais da academia logada.
func (s *academiaGestaoService) Perfil(userID uint) (*models.Academia, error) {
	return s.academiaDe(userID)
}

// AtualizarPerfil grava o endereço estruturado (CEP 8 dígitos, UF válida, número obrigatório) e o texto
// formatado em `endereco`; o CHECK do banco (3819) vira 422.
func (s *academiaGestaoService) AtualizarPerfil(userID uint, req *models.AtualizarAcademiaRequest) (*models.Academia, error) {
	a, err := s.academiaDe(userID)
	if err != nil {
		return nil, err
	}
	e, err := normalizarEndereco(&req.EnderecoAcademiaRequest)
	if err != nil {
		return nil, err
	}
	campos := e.campos()
	if req.Telefone != nil {
		campos["telefone"] = strings.TrimSpace(*req.Telefone)
	}
	if err := s.academias.Atualizar(a.ID, campos); err != nil {
		if repositories.EhViolacaoCheck(err) {
			return nil, ErroEnderecoInvalido
		}
		return nil, err
	}
	return s.academiaDe(userID)
}
