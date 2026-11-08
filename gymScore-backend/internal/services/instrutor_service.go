package services

import (
	"fmt"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// InstrutorService define as operações de negócio para instrutores
type InstrutorService interface {
	// Criar ativa o perfil de instrutor do PRÓPRIO usuário logado (identidade do JWT, nunca do corpo).
	Criar(userID uint, req *models.CriarInstrutorRequest) (*models.InstrutorResponse, error)
	BuscarPorUsuario(idUsuario uint) (*models.Instrutor, error)
	// Listar é o diretório de instrutores (dados públicos: nome, CREF, especialidade).
	Listar() ([]models.InstrutorResponse, error)
	// Agenda devolve só os desafios em que o usuário é o instrutor designado.
	Agenda(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error)
	// Desvincular: o PRÓPRIO instrutor encerra o vínculo com a academia (N3). Bloqueado (409) enquanto
	// ele for o designado de desafio aberto ou em andamento.
	Desvincular(userID uint) error
	// Convite / ResponderConvite: convite de vínculo enviado por uma academia (aceitar ou recusar).
	Convite(userID uint) (*models.ConviteInstrutorResponse, error)
	ResponderConvite(userID uint, aceitar bool) error
}

type instrutorService struct {
	db          *gorm.DB
	repo        repositories.InstrutorRepository
	usuarioRepo repositories.UsuarioRepository
	vinculos    *repositories.VinculoInstrutorRepository
	desafios    *repositories.DesafioRepository
	notif       Notificador
	agora       func() time.Time
}

// NovoInstrutorService cria uma nova instância do serviço de instrutores
func NovoInstrutorService(
	db *gorm.DB,
	repo repositories.InstrutorRepository,
	usuarioRepo repositories.UsuarioRepository,
	vinculos *repositories.VinculoInstrutorRepository,
	desafios *repositories.DesafioRepository,
	notif Notificador,
) InstrutorService {
	return &instrutorService{db: db, repo: repo, usuarioRepo: usuarioRepo, vinculos: vinculos, desafios: desafios,
		notif: notif, agora: time.Now}
}

// Desvincular remove o vínculo do instrutor logado com a academia e avisa a academia.
func (s *instrutorService) Desvincular(userID uint) error {
	inst, err := s.repo.BuscarPorUsuario(userID)
	if err != nil {
		return err
	}
	if inst == nil {
		return erroProibido("Apenas instrutores podem realizar esta ação")
	}
	var avisar uint
	var nome string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		repo := s.vinculos.ComTx(tx)
		i, err := repo.TravarPorID(inst.ID)
		if err != nil {
			return err
		}
		if i == nil || i.IDAcademia == nil {
			return erroConflito("Você não está vinculado a nenhuma academia")
		}
		ativos, err := repo.DesafiosAtivosDe(i.ID, maxDesafiosNoBloqueio+1)
		if err != nil {
			return err
		}
		if len(ativos) > 0 {
			return erroConflito(strings.Replace(mensagemBloqueioDesvinculo(ativos),
				"O instrutor ainda é o responsável", "Você ainda é o responsável", 1))
		}
		idAcademia := *i.IDAcademia
		ok, err := repo.Desvincular(i.ID, idAcademia)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("O vínculo mudou. Atualize a página e tente de novo")
		}
		if avisar, err = repo.UsuarioDaAcademia(idAcademia); err != nil {
			return err
		}
		if i.Usuario != nil {
			nome = strings.TrimSpace(i.Usuario.Nome + " " + i.Usuario.Sobrenome)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if avisar != 0 && s.notif != nil {
		s.notif.Notificar(avisar, models.NotifInstrutorRemovido, "Instrutor saiu da academia",
			fmt.Sprintf("O instrutor %s encerrou o vínculo com a sua academia.", nome), "/academia/instrutores")
	}
	return nil
}

func paraInstrutorResponse(i *models.Instrutor) models.InstrutorResponse {
	r := models.InstrutorResponse{ID: i.ID, CREF: i.CREF, Especialidade: i.Especialidade, Autonomo: i.IDAcademia == nil}
	if i.Usuario != nil {
		r.Nome, r.Sobrenome = i.Usuario.Nome, i.Usuario.Sobrenome
	}
	return r
}

// Criar promove o usuário logado a instrutor (autônomo: o vínculo com a academia é feito por ela).
// Só conta de atleta pode ser promovida, e não enquanto estiver inscrita em desafio aberto ou em
// andamento (instrutor não pode sair do desafio nem receber prêmio como atleta). O perfil e o
// registro de instrutor são gravados na mesma transação.
func (s *instrutorService) Criar(userID uint, req *models.CriarInstrutorRequest) (*models.InstrutorResponse, error) {
	usuario, err := s.usuarioRepo.BuscarPorID(userID)
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		return nil, erro(401, "Usuário não encontrado")
	}
	switch usuario.Perfil {
	case "instrutor":
		return nil, erroConflito("Você já tem o perfil de instrutor")
	case "academia":
		return nil, erroProibido("Contas de academia não podem se tornar instrutor")
	}
	if existente, err := s.repo.BuscarPorUsuario(userID); err != nil {
		return nil, err
	} else if existente != nil {
		return nil, erroConflito("Você já tem o perfil de instrutor")
	}
	// O cadastro nasce PENDENTE: o perfil vira instrutor, mas ele só opera (agenda, candidaturas,
	// ser designado) depois da aprovação do admin.
	instrutor := &models.Instrutor{IDUsuario: userID, CREF: req.CREF, Especialidade: req.Especialidade,
		Status: models.CadastroPendente}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Trava a linha do usuário ANTES de conferir as inscrições: Entrar trava a mesma linha antes de
		// inscrever, então a promoção e uma inscrição simultânea se serializam (N4) e a contagem abaixo
		// (primeira leitura consistente da transação, feita depois da trava) vê a inscrição já commitada.
		if _, err := perfilTravado(tx, userID); err != nil {
			return err
		}
		ativas, err := s.desafios.ComTx(tx).ContarInscricoesAtivas(userID)
		if err != nil {
			return err
		}
		if ativas > 0 {
			return erroConflito("Você está inscrito em desafio aberto ou em andamento. Saia dele ou aguarde o encerramento antes de virar instrutor")
		}
		repo := s.vinculos.ComTx(tx)
		ok, err := repo.PromoverUsuario(userID)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("Apenas contas de atleta podem se tornar instrutor")
		}
		return repo.Criar(instrutor)
	})
	if err != nil {
		if repositories.EhDuplicata(err) { // duas requisições simultâneas
			return nil, erroConflito("Você já tem o perfil de instrutor")
		}
		return nil, err
	}
	instrutor.Usuario = usuario
	r := paraInstrutorResponse(instrutor)
	r.Status = instrutor.Status
	return &r, nil
}

// BuscarPorUsuario retorna o instrutor vinculado ao ID de usuário informado
func (s *instrutorService) BuscarPorUsuario(idUsuario uint) (*models.Instrutor, error) {
	return s.repo.BuscarPorUsuario(idUsuario)
}

// Listar retorna o diretório de instrutores (sem e-mail, CPF nem id de usuário).
func (s *instrutorService) Listar() ([]models.InstrutorResponse, error) {
	lista, err := s.vinculos.ListarTodos()
	if err != nil {
		return nil, err
	}
	out := make([]models.InstrutorResponse, 0, len(lista))
	for i := range lista {
		out = append(out, paraInstrutorResponse(&lista[i]))
	}
	return out, nil
}

// Agenda devolve os desafios em que o usuário é o instrutor DESIGNADO, paginados. Sem status: abertos
// e em andamento, do evento mais próximo ao mais distante. Com status: só os desse status (aberto e
// em andamento do mais próximo ao mais distante; encerrado e cancelado do mais recente ao mais antigo).
func (s *instrutorService) Agenda(userID uint, status models.StatusDesafio, pagina, limite int) (*models.ListaDesafiosResponse, error) {
	if status != "" && !statusValido(status) {
		return nil, erroRequisicao("status inválido (use aberto, em_andamento, encerrado ou cancelado)")
	}
	pagina, limite = normalizarPagina(pagina, limite)
	vazio := &models.ListaDesafiosResponse{Itens: []models.DesafioResponse{}, Pagina: pagina, Limite: limite}
	inst, err := s.repo.BuscarPorUsuario(userID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return vazio, nil
	}
	filtro := repositories.FiltroDesafios{
		IDUsuario: userID, Todos: true, IDInstrutor: inst.ID, Status: status,
		Agora: s.agora(), Pagina: pagina, Limite: limite, Ordem: repositories.OrdemEventoProx,
	}
	switch status {
	case "":
		filtro.Statuses = statusAtivos
	case models.StatusEncerrado, models.StatusCancelado:
		filtro.Ordem = repositories.OrdemEventoRec
	}
	ds, total, err := s.desafios.Listar(filtro)
	if err != nil {
		return nil, err
	}
	ctx := &contextoUsuario{ID: userID, Perfil: "instrutor", InstrutorID: inst.ID,
		InstrutorAprovado: inst.Status == models.CadastroAprovado}
	agora := s.agora()
	itens := make([]models.DesafioResponse, 0, len(ds))
	for i := range ds {
		itens = append(itens, *montarResposta(&ds[i], ctx, false, agora))
	}
	return &models.ListaDesafiosResponse{Itens: itens, Total: total, Pagina: pagina, Limite: limite}, nil
}

// instrutorDoUsuario devolve o registro de instrutor da conta logada (403 se não for instrutor).
func (s *instrutorService) instrutorDoUsuario(userID uint) (*models.Instrutor, error) {
	inst, err := s.repo.BuscarPorUsuario(userID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, erroProibido("Apenas instrutores podem realizar esta ação")
	}
	return inst, nil
}

// Convite devolve o convite de vínculo pendente do instrutor logado (Convite nil se não houver).
// Vale também com o cadastro pendente de aprovação (o vínculo não dá acesso a nada sozinho).
func (s *instrutorService) Convite(userID uint) (*models.ConviteInstrutorResponse, error) {
	inst, err := s.instrutorDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	resp := &models.ConviteInstrutorResponse{IDAcademia: inst.IDAcademia}
	if inst.IDAcademiaConvite != nil && inst.IDAcademia == nil {
		nome, err := s.vinculos.NomeAcademia(*inst.IDAcademiaConvite)
		if err != nil {
			return nil, err
		}
		resp.Convite = &models.ConviteAcademiaResumo{IDAcademia: *inst.IDAcademiaConvite, NomeFantasia: nome, ConviteEm: inst.ConviteEm}
	}
	return resp, nil
}

// ResponderConvite aceita (vira vínculo) ou recusa o convite pendente e avisa a academia.
// 404 se não houver convite; aceitar com vínculo já existente também é 404 (o UPDATE exige sem vínculo).
func (s *instrutorService) ResponderConvite(userID uint, aceitar bool) error {
	inst, err := s.instrutorDoUsuario(userID)
	if err != nil {
		return err
	}
	if inst.IDAcademiaConvite == nil {
		return erroNaoEncontrado("Você não tem convite de academia pendente")
	}
	idAcademia := *inst.IDAcademiaConvite
	var ok bool
	if aceitar {
		ok, err = s.vinculos.AceitarConvite(inst.ID, idAcademia)
	} else {
		ok, err = s.vinculos.LimparConvite(inst.ID, idAcademia)
	}
	if err != nil {
		return err
	}
	if !ok {
		if aceitar && inst.IDAcademia != nil {
			return erroConflito("Você já está vinculado a uma academia. Saia dela antes de aceitar outro convite")
		}
		return erroNaoEncontrado("O convite não está mais disponível")
	}
	avisar, err := s.vinculos.UsuarioDaAcademia(idAcademia)
	if err != nil || avisar == 0 || s.notif == nil {
		return nil // o vínculo já mudou; a notificação é acessória
	}
	nome := ""
	if full, err := s.repo.BuscarPorID(inst.ID); err == nil && full != nil && full.Usuario != nil {
		nome = strings.TrimSpace(full.Usuario.Nome + " " + full.Usuario.Sobrenome)
	}
	if aceitar {
		s.notif.Notificar(avisar, models.NotifInstrutorVinculado, "Convite aceito",
			fmt.Sprintf("O instrutor %s aceitou o convite e agora está vinculado à sua academia.", nome), "/academia/instrutores")
	} else {
		s.notif.Notificar(avisar, models.NotifConviteRecusado, "Convite recusado",
			fmt.Sprintf("O instrutor %s recusou o convite de vínculo.", nome), "/academia/instrutores")
	}
	return nil
}
