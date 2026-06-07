package repositories

import (
	"gynScore-backend/internal/models"

	"gorm.io/gorm"
)

type TransacaoRepository interface {
	Criar(t *models.Transacao) error
	BuscarPorAsaasID(asaasID string) (*models.Transacao, error)
	AtualizarStatus(asaasID, status string) error
	// MarcarRecebidoSePendente muda o status para "received" somente se ainda estiver
	// "pending". Retorna true se esta chamada foi a que efetivou a mudança — garantindo
	// que o saldo seja creditado uma única vez mesmo com polling concorrente.
	MarcarRecebidoSePendente(asaasID string) (bool, error)
}

type transacaoRepository struct {
	db *gorm.DB
}

func NovoTransacaoRepository(db *gorm.DB) TransacaoRepository {
	return &transacaoRepository{db}
}

func (r *transacaoRepository) Criar(t *models.Transacao) error {
	return r.db.Create(t).Error
}

func (r *transacaoRepository) BuscarPorAsaasID(asaasID string) (*models.Transacao, error) {
	var t models.Transacao
	if err := r.db.Where("asaas_payment_id = ?", asaasID).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *transacaoRepository) AtualizarStatus(asaasID, status string) error {
	return r.db.Model(&models.Transacao{}).
		Where("asaas_payment_id = ?", asaasID).
		Update("status", status).Error
}

func (r *transacaoRepository) MarcarRecebidoSePendente(asaasID string) (bool, error) {
	res := r.db.Model(&models.Transacao{}).
		Where("asaas_payment_id = ? AND status = ?", asaasID, "pending").
		Update("status", "received")
	return res.RowsAffected > 0, res.Error
}
