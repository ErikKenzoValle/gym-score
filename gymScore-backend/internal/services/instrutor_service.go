package services

import (
	"errors"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
)

// InstrutorService define as operações de negócio para instrutores
type InstrutorService interface {
	Criar(req *models.CriarInstrutorRequest) (*models.Instrutor, error)
	BuscarPorUsuario(idUsuario uint) (*models.Instrutor, error)
	Listar() ([]models.Instrutor, error)
}

type instrutorService struct {
	repo        repositories.InstrutorRepository
	usuarioRepo repositories.UsuarioRepository
}

// NovoInstrutorService cria uma nova instância do serviço de instrutores
func NovoInstrutorService(repo repositories.InstrutorRepository, usuarioRepo repositories.UsuarioRepository) InstrutorService {
	return &instrutorService{repo, usuarioRepo}
}

// Criar valida e persiste um novo instrutor, atualizando o perfil do usuário para "instrutor"
func (s *instrutorService) Criar(req *models.CriarInstrutorRequest) (*models.Instrutor, error) {
	usuario, err := s.usuarioRepo.BuscarPorID(req.IDUsuario)
	if err != nil || usuario == nil {
		return nil, errors.New("usuário não encontrado")
	}
	existente, _ := s.repo.BuscarPorUsuario(req.IDUsuario)
	if existente != nil {
		return nil, errors.New("instrutor já cadastrado para este usuário")
	}
	// Atualizar perfil do usuário para instrutor
	usuario.Perfil = "instrutor"
	_ = s.usuarioRepo.Atualizar(usuario)

	instrutor := &models.Instrutor{
		IDUsuario:     req.IDUsuario,
		CREF:          req.CREF,
		Especialidade: req.Especialidade,
		IDAcademia:    req.IDAcademia,
	}
	return instrutor, s.repo.Criar(instrutor)
}

// BuscarPorUsuario retorna o instrutor vinculado ao ID de usuário informado
func (s *instrutorService) BuscarPorUsuario(idUsuario uint) (*models.Instrutor, error) {
	return s.repo.BuscarPorUsuario(idUsuario)
}

// Listar retorna todos os instrutores cadastrados
func (s *instrutorService) Listar() ([]models.Instrutor, error) {
	return s.repo.Listar()
}
