package repositories

import (
	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// AvaliacaoRepository define a interface de acesso a dados para avaliações de desafios
type AvaliacaoRepository interface {
	Criar(a *models.AvaliacaoDesafio) error
	MediaPorDesafio(idDesafio uint) (float64, error)
	MediaGlobal() (float64, error)
}

type avaliacaoRepository struct{ db *gorm.DB }

// NovoAvaliacaoRepository cria uma nova instância do repositório de avaliações
func NovoAvaliacaoRepository(db *gorm.DB) AvaliacaoRepository {
	return &avaliacaoRepository{db}
}

// Criar insere uma avaliação no banco de dados
func (r *avaliacaoRepository) Criar(a *models.AvaliacaoDesafio) error {
	return r.db.Create(a).Error
}

// MediaPorDesafio retorna a média de notas de um desafio específico
func (r *avaliacaoRepository) MediaPorDesafio(idDesafio uint) (float64, error) {
	var media float64
	err := r.db.Model(&models.AvaliacaoDesafio{}).
		Where("id_desafio = ?", idDesafio).
		Select("COALESCE(AVG(nota), 0)").Scan(&media).Error
	return media, err
}

// MediaGlobal retorna a média global de todas as avaliações
func (r *avaliacaoRepository) MediaGlobal() (float64, error) {
	var media float64
	err := r.db.Model(&models.AvaliacaoDesafio{}).
		Select("COALESCE(AVG(nota), 0)").Scan(&media).Error
	return media, err
}
