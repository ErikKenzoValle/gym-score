package services

import (
	"errors"
	"fmt"
	"time"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/pkg/utils"
)

// DesafioService define as operações de negócio para desafios
type DesafioService interface {
	CriarDesafio(req *models.CriarDesafioRequest) (*models.Desafio, error)
	AceitarDesafio(req *models.AceitarDesafioRequest) (*models.Desafio, error)
	IniciarDesafio(req *models.IniciarDesafioRequest) (*models.Desafio, error)
	EncerrarDesafio(req *models.EncerrarDesafioRequest) (*models.Desafio, error)
	CancelarDesafio(idDesafio, idCriador uint) error
	Listar(idUsuario uint) ([]models.Desafio, error)
	ListarPorUsuario(idUsuario uint) ([]models.Desafio, error)
	BuscarPorID(id uint) (*models.Desafio, error)
}

// desafioService é a implementação concreta da camada de serviço
type desafioService struct {
	desafioRepo repositories.DesafioRepository
	usuarioRepo repositories.UsuarioRepository
}

// NovoDesafioService cria uma nova instância do serviço de desafios
func NovoDesafioService(
	desafioRepo repositories.DesafioRepository,
	usuarioRepo repositories.UsuarioRepository,
) DesafioService {
	return &desafioService{
		desafioRepo: desafioRepo,
		usuarioRepo: usuarioRepo,
	}
}

// CriarDesafio valida e persiste um novo desafio
func (s *desafioService) CriarDesafio(req *models.CriarDesafioRequest) (*models.Desafio, error) {
	// Verificar se o criador existe
	criador, err := s.usuarioRepo.BuscarPorID(req.IDCriador)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar criador: %w", err)
	}
	if criador == nil {
		return nil, errors.New("usuário criador não encontrado")
	}

	// Verificar saldo suficiente (equivalente ao endpoint /validar-saldo do Java)
	if !utils.ValidarSaldo(criador.Saldo, req.Valor) {
		return nil, errors.New("saldo insuficiente para criar o desafio")
	}

	// Número de vagas: mínimo 2 (criador + 1), padrão 2
	vagas := req.Vagas
	if vagas < 2 {
		vagas = 2
	}

	desafio := &models.Desafio{
		Titulo:    req.Titulo,
		Descricao: req.Descricao,
		Valor:     req.Valor,
		Local:     req.Local,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		IDCriador: req.IDCriador,
		Vagas:     vagas,
		Status:    models.StatusAberto,
	}
	if req.DataEncerramento != "" {
		if t, err := time.Parse("2006-01-02", req.DataEncerramento); err == nil {
			desafio.DataEncerramento = &t
		}
	}

	if err := s.desafioRepo.Criar(desafio); err != nil {
		return nil, fmt.Errorf("erro ao criar desafio: %w", err)
	}

	// O criador ocupa automaticamente a primeira vaga
	if err := s.desafioRepo.AdicionarParticipante(&models.DesafioParticipante{
		IDDesafio: desafio.ID,
		IDUsuario: req.IDCriador,
	}); err != nil {
		return nil, fmt.Errorf("erro ao inscrever o criador: %w", err)
	}

	return desafio, nil
}

// AceitarDesafio inscreve um usuário no desafio, ocupando uma vaga
func (s *desafioService) AceitarDesafio(req *models.AceitarDesafioRequest) (*models.Desafio, error) {
	desafio, err := s.desafioRepo.BuscarPorID(req.IDDesafio)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar desafio: %w", err)
	}
	if desafio == nil {
		return nil, errors.New("desafio não encontrado")
	}
	if desafio.Status != models.StatusAberto {
		return nil, errors.New("desafio não está aberto para inscrições")
	}

	// Verificar se o usuário existe
	usuario, err := s.usuarioRepo.BuscarPorID(req.IDUsuario)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar usuário: %w", err)
	}
	if usuario == nil {
		return nil, errors.New("usuário não encontrado")
	}

	// Já está inscrito?
	jaParticipa, err := s.desafioRepo.UsuarioParticipa(req.IDDesafio, req.IDUsuario)
	if err != nil {
		return nil, fmt.Errorf("erro ao verificar inscrição: %w", err)
	}
	if jaParticipa {
		return nil, errors.New("você já está participando deste desafio")
	}

	// Ainda há vaga?
	inscritos, err := s.desafioRepo.ContarParticipantes(req.IDDesafio)
	if err != nil {
		return nil, fmt.Errorf("erro ao contar participantes: %w", err)
	}
	if int(inscritos) >= desafio.Vagas {
		return nil, errors.New("não há mais vagas neste desafio")
	}

	if err := s.desafioRepo.AdicionarParticipante(&models.DesafioParticipante{
		IDDesafio: req.IDDesafio,
		IDUsuario: req.IDUsuario,
	}); err != nil {
		return nil, fmt.Errorf("erro ao entrar no desafio: %w", err)
	}

	// Recarrega com a lista atualizada de participantes
	return s.desafioRepo.BuscarPorID(req.IDDesafio)
}

// IniciarDesafio muda o status do desafio para "em andamento"
func (s *desafioService) IniciarDesafio(req *models.IniciarDesafioRequest) (*models.Desafio, error) {
	desafio, err := s.desafioRepo.BuscarPorID(req.IDDesafio)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar desafio: %w", err)
	}
	if desafio == nil {
		return nil, errors.New("desafio não encontrado")
	}
	if desafio.Status != models.StatusPendente {
		return nil, errors.New("desafio não está no estado correto para ser iniciado")
	}

	desafio.Status = models.StatusEmAndamento

	if err := s.desafioRepo.Atualizar(desafio); err != nil {
		return nil, fmt.Errorf("erro ao iniciar desafio: %w", err)
	}

	return desafio, nil
}

// EncerrarDesafio finaliza o desafio: o vencedor (que deve ser um participante)
// recebe o prêmio bancado pelo criador. Se o vencedor for o próprio criador, o saldo
// fica neutro (ele banca e recebe o mesmo valor).
func (s *desafioService) EncerrarDesafio(req *models.EncerrarDesafioRequest) (*models.Desafio, error) {
	desafio, err := s.desafioRepo.BuscarPorID(req.IDDesafio)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar desafio: %w", err)
	}
	if desafio == nil {
		return nil, errors.New("desafio não encontrado")
	}
	if desafio.Status == models.StatusEncerrado {
		return nil, errors.New("desafio já foi encerrado")
	}

	// O vencedor precisa estar inscrito no desafio
	participa, err := s.desafioRepo.UsuarioParticipa(req.IDDesafio, req.IDVencedor)
	if err != nil {
		return nil, fmt.Errorf("erro ao verificar participante: %w", err)
	}
	if !participa {
		return nil, errors.New("o vencedor precisa ser um participante do desafio")
	}

	// Se o criador for o vencedor, não há transferência (banca e recebe o mesmo valor)
	if req.IDVencedor != desafio.IDCriador {
		vencedor, err := s.usuarioRepo.BuscarPorID(req.IDVencedor)
		if err != nil || vencedor == nil {
			return nil, errors.New("vencedor não encontrado")
		}
		criador, err := s.usuarioRepo.BuscarPorID(desafio.IDCriador)
		if err != nil || criador == nil {
			return nil, errors.New("criador não encontrado")
		}
		if !utils.ValidarSaldo(criador.Saldo, desafio.Valor) {
			return nil, errors.New("o criador não tem saldo suficiente para pagar o prêmio")
		}

		criador.Saldo -= desafio.Valor
		vencedor.Saldo += desafio.Valor

		if err := s.usuarioRepo.Atualizar(criador); err != nil {
			return nil, fmt.Errorf("erro ao debitar saldo do criador: %w", err)
		}
		if err := s.usuarioRepo.Atualizar(vencedor); err != nil {
			return nil, fmt.Errorf("erro ao creditar saldo do vencedor: %w", err)
		}
	}

	desafio.Status = models.StatusEncerrado
	desafio.IDVencedor = &req.IDVencedor

	if err := s.desafioRepo.Atualizar(desafio); err != nil {
		return nil, fmt.Errorf("erro ao encerrar desafio: %w", err)
	}

	return desafio, nil
}

// CancelarDesafio encerra um desafio aberto sem adversário, sem alterar saldos
func (s *desafioService) CancelarDesafio(idDesafio, idCriador uint) error {
	desafio, err := s.desafioRepo.BuscarPorID(idDesafio)
	if err != nil {
		return fmt.Errorf("erro ao buscar desafio: %w", err)
	}
	if desafio == nil {
		return errors.New("desafio não encontrado")
	}
	if desafio.IDCriador != idCriador {
		return errors.New("apenas o criador pode cancelar o desafio")
	}
	if desafio.Status != models.StatusAberto {
		return errors.New("apenas desafios abertos podem ser cancelados")
	}
	desafio.Status = models.StatusEncerrado
	return s.desafioRepo.Atualizar(desafio)
}

// Listar retorna os desafios visíveis para o usuário. Desafios ainda ativos e
// já lotados (vagas esgotadas) só aparecem para quem participa (incluindo o criador).
func (s *desafioService) Listar(idUsuario uint) ([]models.Desafio, error) {
	desafios, err := s.desafioRepo.Listar()
	if err != nil {
		return nil, err
	}
	visiveis := make([]models.Desafio, 0, len(desafios))
	for _, d := range desafios {
		lotado := len(d.Participantes) >= d.Vagas
		if d.Status != models.StatusEncerrado && lotado && !usuarioParticipaDe(d, idUsuario) {
			continue
		}
		visiveis = append(visiveis, d)
	}
	return visiveis, nil
}

// usuarioParticipaDe indica se o usuário é o criador ou um participante do desafio.
func usuarioParticipaDe(d models.Desafio, idUsuario uint) bool {
	if d.IDCriador == idUsuario {
		return true
	}
	for _, p := range d.Participantes {
		if p.IDUsuario == idUsuario {
			return true
		}
	}
	return false
}

// ListarPorUsuario retorna os desafios ativos de um usuário
func (s *desafioService) ListarPorUsuario(idUsuario uint) ([]models.Desafio, error) {
	return s.desafioRepo.ListarPorUsuario(idUsuario)
}

// BuscarPorID retorna um desafio pelo seu ID
func (s *desafioService) BuscarPorID(id uint) (*models.Desafio, error) {
	return s.desafioRepo.BuscarPorID(id)
}
