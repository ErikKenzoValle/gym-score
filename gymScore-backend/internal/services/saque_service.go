package services

import (
	"errors"
	"fmt"
	"time"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
)

// SaqueService define as operações de negócio para saques
type SaqueService interface {
	Solicitar(userID uint, req *models.SolicitarSaqueRequest) (*models.SaqueResponse, error)
	Listar(userID uint) ([]models.SaqueResponse, error)
}

type saqueService struct {
	repo          repositories.SaqueRepository
	usuarioRepo   repositories.UsuarioRepository
	transacaoRepo repositories.TransacaoRepository
}

// NovoSaqueService cria uma nova instância do serviço de saques
func NovoSaqueService(repo repositories.SaqueRepository, usuarioRepo repositories.UsuarioRepository, transacaoRepo repositories.TransacaoRepository) SaqueService {
	return &saqueService{repo, usuarioRepo, transacaoRepo}
}

// Solicitar valida saldo, debita imediatamente e registra a solicitação de saque
func (s *saqueService) Solicitar(userID uint, req *models.SolicitarSaqueRequest) (*models.SaqueResponse, error) {
	if req.Valor <= 0 {
		return nil, errors.New("valor deve ser maior que zero")
	}
	if req.ChavePix == "" {
		return nil, errors.New("chave PIX é obrigatória")
	}

	usuario, err := s.usuarioRepo.BuscarPorID(userID)
	if err != nil || usuario == nil {
		return nil, errors.New("usuário não encontrado")
	}
	if usuario.Saldo < req.Valor {
		return nil, fmt.Errorf("saldo insuficiente (disponível: R$ %.2f)", usuario.Saldo)
	}

	// Debita saldo imediatamente
	usuario.Saldo -= req.Valor
	if err := s.usuarioRepo.Atualizar(usuario); err != nil {
		return nil, fmt.Errorf("erro ao debitar saldo: %w", err)
	}

	// Registra transação de saída com ID sintético (sem Asaas, pois é saque manual)
	transacao := &models.Transacao{
		IDUsuario:      userID,
		AsaasPaymentID: fmt.Sprintf("SAQUE-%d-%d", userID, time.Now().UnixNano()),
		Valor:          req.Valor,
		Status:         "received",
		Tipo:           "saida",
	}
	_ = s.transacaoRepo.Criar(transacao)

	saque := &models.Saque{
		IDUsuario: userID,
		Valor:     req.Valor,
		ChavePix:  req.ChavePix,
		Status:    "pendente",
	}
	if err := s.repo.Criar(saque); err != nil {
		return nil, fmt.Errorf("erro ao registrar saque: %w", err)
	}

	return &models.SaqueResponse{
		ID:        saque.ID,
		Valor:     saque.Valor,
		ChavePix:  saque.ChavePix,
		Status:    saque.Status,
		CriadoEm: saque.CriadoEm.Format("2006-01-02T15:04:05"),
	}, nil
}

// Listar retorna todos os saques solicitados pelo usuário
func (s *saqueService) Listar(userID uint) ([]models.SaqueResponse, error) {
	saques, err := s.repo.ListarPorUsuario(userID)
	if err != nil {
		return nil, err
	}
	var resp []models.SaqueResponse
	for _, s := range saques {
		resp = append(resp, models.SaqueResponse{
			ID:        s.ID,
			Valor:     s.Valor,
			ChavePix:  s.ChavePix,
			Status:    s.Status,
			CriadoEm: s.CriadoEm.Format("2006-01-02T15:04:05"),
		})
	}
	return resp, nil
}
