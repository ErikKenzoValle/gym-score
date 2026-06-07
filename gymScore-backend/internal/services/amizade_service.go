package services

import (
	"errors"
	"fmt"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
)

// AmizadeService define as operações de negócio para amizades
type AmizadeService interface {
	AdicionarAmigo(req *models.AdicionarAmigoRequest) error
	AceitarAmizade(req *models.AceitarAmizadeRequest) error
	RemoverAmigo(req *models.RemoverAmigoRequest) error
	ListarAmigos(idUsuario uint) ([]models.AmigoResponse, error)
}

// amizadeService é a implementação concreta da camada de serviço
type amizadeService struct {
	amizadeRepo repositories.AmizadeRepository
	usuarioRepo repositories.UsuarioRepository
}

// NovoAmizadeService cria uma nova instância do serviço de amizades
func NovoAmizadeService(
	amizadeRepo repositories.AmizadeRepository,
	usuarioRepo repositories.UsuarioRepository,
) AmizadeService {
	return &amizadeService{
		amizadeRepo: amizadeRepo,
		usuarioRepo: usuarioRepo,
	}
}

// AdicionarAmigo envia uma solicitação de amizade
func (s *amizadeService) AdicionarAmigo(req *models.AdicionarAmigoRequest) error {
	if req.IDUsuario == req.IDAmigo {
		return errors.New("não é possível adicionar a si mesmo como amigo")
	}

	// Verificar se o amigo existe
	amigo, err := s.usuarioRepo.BuscarPorID(req.IDAmigo)
	if err != nil {
		return fmt.Errorf("erro ao buscar usuário: %w", err)
	}
	if amigo == nil {
		return errors.New("usuário não encontrado")
	}

	// Verificar se já existe relação
	relacao, err := s.amizadeRepo.BuscarRelacao(req.IDUsuario, req.IDAmigo)
	if err != nil {
		return fmt.Errorf("erro ao verificar amizade: %w", err)
	}
	if relacao != nil {
		return errors.New("solicitação de amizade já existe ou vocês já são amigos")
	}

	amizade := &models.Amizade{
		IDUsuario: req.IDUsuario,
		IDAmigo:   req.IDAmigo,
		Status:    models.StatusAmizadePendente,
	}

	return s.amizadeRepo.Criar(amizade)
}

// AceitarAmizade aceita uma solicitação de amizade pendente
func (s *amizadeService) AceitarAmizade(req *models.AceitarAmizadeRequest) error {
	relacao, err := s.amizadeRepo.BuscarRelacao(req.IDUsuario, req.IDAmigo)
	if err != nil {
		return fmt.Errorf("erro ao buscar amizade: %w", err)
	}
	if relacao == nil {
		return errors.New("solicitação de amizade não encontrada")
	}
	if relacao.Status != models.StatusAmizadePendente {
		return errors.New("solicitação já foi processada")
	}

	relacao.Status = models.StatusAmizadeAceita
	return s.amizadeRepo.Atualizar(relacao)
}

// RemoverAmigo remove o vínculo de amizade entre dois usuários
func (s *amizadeService) RemoverAmigo(req *models.RemoverAmigoRequest) error {
	relacao, err := s.amizadeRepo.BuscarRelacao(req.IDUsuario, req.IDAmigo)
	if err != nil {
		return fmt.Errorf("erro ao buscar amizade: %w", err)
	}
	if relacao == nil {
		return errors.New("amizade não encontrada")
	}

	return s.amizadeRepo.Deletar(req.IDUsuario, req.IDAmigo)
}

// ListarAmigos retorna a lista de amigos de um usuário
func (s *amizadeService) ListarAmigos(idUsuario uint) ([]models.AmigoResponse, error) {
	amizades, err := s.amizadeRepo.ListarAmigos(idUsuario)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar amigos: %w", err)
	}

	var amigos []models.AmigoResponse
	for _, a := range amizades {
		var amigo *models.Usuario
		// Determinar qual dos dois usuários é o "amigo" (não o solicitante)
		if a.IDUsuario == idUsuario {
			amigo = a.Amigo
		} else {
			amigo = a.Usuario
		}

		if amigo != nil {
			// Recebida = solicitação pendente em que o usuário consultado é o destinatário
			recebida := a.Status == models.StatusAmizadePendente && a.IDAmigo == idUsuario
			amigos = append(amigos, models.AmigoResponse{
				ID:        amigo.ID,
				Nome:      amigo.Nome,
				Sobrenome: amigo.Sobrenome,
				Email:     amigo.Email,
				Status:    string(a.Status),
				Recebida:  recebida,
			})
		}
	}

	return amigos, nil
}
