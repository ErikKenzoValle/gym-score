package repositories

import (
	"errors"

	"gorm.io/gorm"
	"gymScore-backend/internal/models"
)

// InstrutorRepository define a interface de acesso a dados para instrutores
type InstrutorRepository interface {
	Criar(i *models.Instrutor) error
	BuscarPorUsuario(idUsuario uint) (*models.Instrutor, error)
	// BuscarPorID devolve o instrutor com o Usuario carregado (nil se não existir)
	BuscarPorID(id uint) (*models.Instrutor, error)
	Listar() ([]models.Instrutor, error)
}

type instrutorRepository struct{ db *gorm.DB }

// NovoInstrutorRepository cria uma nova instância do repositório de instrutores
func NovoInstrutorRepository(db *gorm.DB) InstrutorRepository {
	return &instrutorRepository{db}
}

// Criar insere um novo instrutor no banco de dados
func (r *instrutorRepository) Criar(i *models.Instrutor) error {
	return r.db.Create(i).Error
}

// BuscarPorUsuario retorna o instrutor vinculado ao ID de usuário informado
func (r *instrutorRepository) BuscarPorUsuario(idUsuario uint) (*models.Instrutor, error) {
	var i models.Instrutor
	err := r.db.Where("id_usuario = ?", idUsuario).First(&i).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &i, nil
}

// Listar retorna todos os instrutores com seus usuários carregados
func (r *instrutorRepository) Listar() ([]models.Instrutor, error) {
	var lista []models.Instrutor
	return lista, r.db.Preload("Usuario").Find(&lista).Error
}

// BuscarPorID retorna o instrutor (com Usuario) pelo id da tabela instrutores
func (r *instrutorRepository) BuscarPorID(id uint) (*models.Instrutor, error) {
	var i models.Instrutor
	err := r.db.Preload("Usuario").First(&i, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &i, nil
}
