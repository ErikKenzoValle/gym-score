package repositories

import (
	"errors"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ErrAmizadeDuplicada indica violação da unicidade do par (em qualquer direção).
var ErrAmizadeDuplicada = errors.New("amizade já existe")

// AmizadeRepository define a interface de acesso a dados para amizades
type AmizadeRepository interface {
	Criar(amizade *models.Amizade) error
	BuscarRelacao(idUsuario, idAmigo uint) (*models.Amizade, error)
	ListarAmigos(idUsuario uint) ([]models.Amizade, error)
	// Reenviar reabre uma solicitação recusada na direção remetente -> destinatário.
	Reenviar(id, remetente, destinatario uint) (bool, error)
	// Responder muda pendente -> novoStatus SOMENTE se o destinatário for idDestinatario.
	Responder(idRemetente, idDestinatario uint, novoStatus models.StatusAmizade) (bool, error)
	// Remover apaga a relação aceita/pendente entre os dois usuários. Retorna se apagou algo.
	Remover(idUsuario, idAmigo uint) (bool, error)
	SaoAmigos(a, b uint) (bool, error)
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
	if err := r.db.Create(amizade).Error; err != nil {
		if EhDuplicata(err) {
			return ErrAmizadeDuplicada
		}
		return err
	}
	return nil
}

const condPar = "(id_usuario = ? AND id_amigo = ?) OR (id_usuario = ? AND id_amigo = ?)"

// BuscarRelacao retorna a relação de amizade entre dois usuários (em qualquer direção)
func (r *amizadeRepository) BuscarRelacao(idUsuario, idAmigo uint) (*models.Amizade, error) {
	var amizade models.Amizade
	err := r.db.Where(condPar, idUsuario, idAmigo, idAmigo, idUsuario).First(&amizade).Error
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
		).Order("id ASC").Find(&amizades).Error
	return amizades, err
}

func (r *amizadeRepository) Reenviar(id, remetente, destinatario uint) (bool, error) {
	res := r.db.Model(&models.Amizade{}).
		Where("id = ? AND status = ?", id, models.StatusAmizadeRecusada).
		Updates(map[string]any{
			"id_usuario": remetente,
			"id_amigo":   destinatario,
			"status":     models.StatusAmizadePendente,
		})
	return res.RowsAffected > 0, res.Error
}

func (r *amizadeRepository) Responder(idRemetente, idDestinatario uint, novoStatus models.StatusAmizade) (bool, error) {
	res := r.db.Model(&models.Amizade{}).
		Where("id_usuario = ? AND id_amigo = ? AND status = ?", idRemetente, idDestinatario, models.StatusAmizadePendente).
		Update("status", novoStatus)
	return res.RowsAffected > 0, res.Error
}

func (r *amizadeRepository) Remover(idUsuario, idAmigo uint) (bool, error) {
	res := r.db.Where("("+condPar+") AND status IN ?", idUsuario, idAmigo, idAmigo, idUsuario,
		[]models.StatusAmizade{models.StatusAmizadeAceita, models.StatusAmizadePendente}).
		Delete(&models.Amizade{})
	return res.RowsAffected > 0, res.Error
}

func (r *amizadeRepository) SaoAmigos(a, b uint) (bool, error) {
	var total int64
	err := r.db.Model(&models.Amizade{}).
		Where("("+condPar+") AND status = ?", a, b, b, a, models.StatusAmizadeAceita).
		Count(&total).Error
	return total > 0, err
}
