package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// NotificacaoRepository persiste notificações in-app.
type NotificacaoRepository interface {
	Criar(n *models.Notificacao) error
	CriarVarias(ns []models.Notificacao) error
	// Listar devolve a página (não lidas primeiro, depois mais recentes) e o total.
	Listar(idUsuario uint, pagina, limite int) ([]models.Notificacao, int64, error)
	ContarNaoLidas(idUsuario uint) (int64, error)
	// MarcarLida: existe=false se a notificação não for do usuário/não existir.
	MarcarLida(id, idUsuario uint, agora time.Time) (existe bool, err error)
	MarcarTodasLidas(idUsuario uint, agora time.Time) (int64, error)
}

type notificacaoRepository struct{ db *gorm.DB }

func NovoNotificacaoRepository(db *gorm.DB) NotificacaoRepository {
	return &notificacaoRepository{db}
}

func (r *notificacaoRepository) Criar(n *models.Notificacao) error { return r.db.Create(n).Error }

func (r *notificacaoRepository) CriarVarias(ns []models.Notificacao) error {
	if len(ns) == 0 {
		return nil
	}
	return r.db.Create(&ns).Error
}

func (r *notificacaoRepository) Listar(idUsuario uint, pagina, limite int) ([]models.Notificacao, int64, error) {
	q := r.db.Model(&models.Notificacao{}).Where("id_usuario = ?", idUsuario)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var itens []models.Notificacao
	err := q.Order("(lida_em IS NOT NULL) ASC, criado_em DESC, id DESC").
		Offset((pagina - 1) * limite).Limit(limite).Find(&itens).Error
	return itens, total, err
}

func (r *notificacaoRepository) ContarNaoLidas(idUsuario uint) (int64, error) {
	var n int64
	err := r.db.Model(&models.Notificacao{}).Where("id_usuario = ? AND lida_em IS NULL", idUsuario).Count(&n).Error
	return n, err
}

func (r *notificacaoRepository) MarcarLida(id, idUsuario uint, agora time.Time) (bool, error) {
	var total int64
	if err := r.db.Model(&models.Notificacao{}).Where("id = ? AND id_usuario = ?", id, idUsuario).Count(&total).Error; err != nil || total == 0 {
		return false, err
	}
	err := r.db.Model(&models.Notificacao{}).
		Where("id = ? AND id_usuario = ? AND lida_em IS NULL", id, idUsuario).
		Update("lida_em", agora).Error
	return true, err
}

func (r *notificacaoRepository) MarcarTodasLidas(idUsuario uint, agora time.Time) (int64, error) {
	res := r.db.Model(&models.Notificacao{}).
		Where("id_usuario = ? AND lida_em IS NULL", idUsuario).Update("lida_em", agora)
	return res.RowsAffected, res.Error
}
