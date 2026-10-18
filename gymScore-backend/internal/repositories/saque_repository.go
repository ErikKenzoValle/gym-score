package repositories

import (
	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// SaqueRepository define a interface de acesso a dados para saques
type SaqueRepository interface {
	Criar(s *models.Saque) error
	ListarPorUsuario(idUsuario uint) ([]models.Saque, error)
	BuscarPorID(id uint) (*models.Saque, error)
}

type saqueRepository struct{ db *gorm.DB }

// NovoSaqueRepository cria uma nova instância do repositório de saques
func NovoSaqueRepository(db *gorm.DB) SaqueRepository {
	return &saqueRepository{db}
}

// Criar insere um novo saque no banco de dados
func (r *saqueRepository) Criar(s *models.Saque) error {
	return r.db.Create(s).Error
}

// ListarPorUsuario retorna os saques do usuário ordenados por data decrescente
func (r *saqueRepository) ListarPorUsuario(idUsuario uint) ([]models.Saque, error) {
	var lista []models.Saque
	return lista, r.db.Where("id_usuario = ?", idUsuario).Order("criado_em DESC").Find(&lista).Error
}

// BuscarPorID retorna um saque pelo seu ID
func (r *saqueRepository) BuscarPorID(id uint) (*models.Saque, error) {
	var s models.Saque
	return &s, r.db.First(&s, id).Error
}
