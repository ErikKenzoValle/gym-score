package repositories

import (
	"errors"

	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// AmizadeRepository define a interface de acesso a dados para amizades
type AmizadeRepository interface {
	Criar(amizade *models.Amizade) error
	BuscarRelacao(idUsuario, idAmigo uint) (*models.Amizade, error)
	ListarAmigos(idUsuario uint) ([]models.Amizade, error)
	Atualizar(amizade *models.Amizade) error
	Deletar(idUsuario, idAmigo uint) error
}

// amizadeRepository é a implementação concreta usando GORM
type amizadeRepository struct {
	db *gorm.DB
}

// NovoAmizadeRepository cria uma nova instância do repositório de amizades
func NovoAmizadeRepository(db *gorm.DB) AmizadeRepository {
	return &amizadeRepository{db: db}
}

// Criar insere uma nova solicitação de amizade no banco de dados
func (r *amizadeRepository) Criar(amizade *models.Amizade) error {
	return r.db.Create(amizade).Error
}

// BuscarRelacao retorna a relação de amizade entre dois usuários (em qualquer direção)
func (r *amizadeRepository) BuscarRelacao(idUsuario, idAmigo uint) (*models.Amizade, error) {
	var amizade models.Amizade
	err := r.db.Where(
		"(id_usuario = ? AND id_amigo = ?) OR (id_usuario = ? AND id_amigo = ?)",
		idUsuario, idAmigo, idAmigo, idUsuario,
	).First(&amizade).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &amizade, nil
}

// ListarAmigos retorna as amizades aceitas e as solicitações pendentes de um usuário
func (r *amizadeRepository) ListarAmigos(idUsuario uint) ([]models.Amizade, error) {
	var amizades []models.Amizade
	err := r.db.Preload("Usuario").Preload("Amigo").
		Where(
			"(id_usuario = ? OR id_amigo = ?) AND status IN ?",
			idUsuario, idUsuario,
			[]models.StatusAmizade{models.StatusAmizadeAceita, models.StatusAmizadePendente},
		).Find(&amizades).Error
	return amizades, err
}

// Atualizar salva as alterações de uma amizade existente
func (r *amizadeRepository) Atualizar(amizade *models.Amizade) error {
	return r.db.Save(amizade).Error
}

// Deletar remove o vínculo de amizade entre dois usuários
func (r *amizadeRepository) Deletar(idUsuario, idAmigo uint) error {
	return r.db.Where(
		"(id_usuario = ? AND id_amigo = ?) OR (id_usuario = ? AND id_amigo = ?)",
		idUsuario, idAmigo, idAmigo, idUsuario,
	).Delete(&models.Amizade{}).Error
}
