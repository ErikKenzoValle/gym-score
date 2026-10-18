package repositories

import (
	"errors"

	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// AcademiaRepository define a interface de acesso a dados para o cadastro de academias
type AcademiaRepository interface {
	Criar(tx *gorm.DB, academia *models.Academia) error
	BuscarPorUsuario(idUsuario uint) (*models.Academia, error)
	BuscarPorCNPJ(cnpj string) (*models.Academia, error)
}

// academiaRepository é a implementação concreta usando GORM
type academiaRepository struct {
	db *gorm.DB
}

// NovoAcademiaRepository cria uma nova instância do repositório de academias
func NovoAcademiaRepository(db *gorm.DB) AcademiaRepository {
	return &academiaRepository{db: db}
}

// Criar insere um novo registro de academia. Se tx for informado, executa dentro da transação
func (r *academiaRepository) Criar(tx *gorm.DB, academia *models.Academia) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.Create(academia).Error
}

// BuscarPorUsuario retorna a academia vinculada ao ID de usuário informado
func (r *academiaRepository) BuscarPorUsuario(idUsuario uint) (*models.Academia, error) {
	var academia models.Academia
	err := r.db.Where("id_usuario = ?", idUsuario).First(&academia).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &academia, nil
}

// BuscarPorCNPJ retorna a academia pelo CNPJ informado (usado para checar duplicidade)
func (r *academiaRepository) BuscarPorCNPJ(cnpj string) (*models.Academia, error) {
	var academia models.Academia
	err := r.db.Where("cnpj = ?", cnpj).First(&academia).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &academia, nil
}
