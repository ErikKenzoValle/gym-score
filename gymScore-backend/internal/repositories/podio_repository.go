package repositories

import (
	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// PodioRepository define a interface de acesso a dados para pódios de desafios
type PodioRepository interface {
	Criar(p *models.PodioDesafio) error
	ListarPorDesafio(idDesafio uint) ([]models.PodioDesafio, error)
	ExistePodio(idDesafio uint) (bool, error)
}

type podioRepository struct{ db *gorm.DB }

// NovoPodioRepository cria uma nova instância do repositório de pódios
func NovoPodioRepository(db *gorm.DB) PodioRepository {
	return &podioRepository{db}
}

// Criar insere uma entrada do pódio no banco de dados
func (r *podioRepository) Criar(p *models.PodioDesafio) error {
	return r.db.Create(p).Error
}

// ListarPorDesafio retorna as posições do pódio de um desafio ordenadas por posição
func (r *podioRepository) ListarPorDesafio(idDesafio uint) ([]models.PodioDesafio, error) {
	var lista []models.PodioDesafio
	return lista, r.db.Preload("Participante").
		Where("id_desafio = ?", idDesafio).
		Order("posicao ASC").Find(&lista).Error
}

// ExistePodio verifica se já existe pódio lançado para o desafio
func (r *podioRepository) ExistePodio(idDesafio uint) (bool, error) {
	var total int64
	err := r.db.Model(&models.PodioDesafio{}).Where("id_desafio = ?", idDesafio).Count(&total).Error
	return total > 0, err
}
