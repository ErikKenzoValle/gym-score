package services

import (
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
)

// RankingService calcula o ranking entre amigos.
type RankingService interface {
	Amigos(userID uint, dias int) ([]models.RankingItem, error)
}

type rankingService struct {
	repo  repositories.RankingRepository
	agora func() time.Time
}

func NovoRankingService(repo repositories.RankingRepository) RankingService {
	return &rankingService{repo: repo, agora: time.Now}
}

// Amigos: eu + amigos aceitos; pontos = exercícios concluídos + 3 x check-ins nos últimos `dias` (1..365, padrão 30).
func (s *rankingService) Amigos(userID uint, dias int) ([]models.RankingItem, error) {
	if dias == 0 {
		dias = 30
	}
	if dias < 1 || dias > 365 {
		return nil, erroRequisicao("dias deve estar entre 1 e 365")
	}
	itens, err := s.repo.Amigos(userID, s.agora().AddDate(0, 0, -dias))
	if err != nil {
		return nil, err
	}
	if itens == nil {
		itens = []models.RankingItem{}
	}
	return itens, nil
}
