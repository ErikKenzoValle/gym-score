package repositories

import (
	"errors"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// TransacaoRepository consulta e cria linhas do ledger que NÃO movimentam saldo
// (a movimentação em si é sempre feita pelo CarteiraRepository).
type TransacaoRepository interface {
	// CriarDepositoPendente grava o depósito PIX pendente (sem tocar no saldo).
	CriarDepositoPendente(t *models.Transacao) error
	// BuscarDepositoDoUsuario devolve nil se o depósito não existir OU não for do usuário.
	BuscarDepositoDoUsuario(asaasID string, idUsuario uint) (*models.Transacao, error)
	// BuscarDeposito busca pelo ID do Asaas (uso interno: webhook).
	BuscarDeposito(asaasID string) (*models.Transacao, error)
}

type transacaoRepository struct {
	db *gorm.DB
}

func NovoTransacaoRepository(db *gorm.DB) TransacaoRepository {
	return &transacaoRepository{db}
}

func (r *transacaoRepository) CriarDepositoPendente(t *models.Transacao) error {
	t.Tipo = models.TipoDeposito
	t.Status = models.TransacaoPending
	return r.db.Create(t).Error
}

func (r *transacaoRepository) BuscarDepositoDoUsuario(asaasID string, idUsuario uint) (*models.Transacao, error) {
	var t models.Transacao
	err := r.db.Where("asaas_payment_id = ? AND id_usuario = ? AND tipo = ?", asaasID, idUsuario, models.TipoDeposito).
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}

func (r *transacaoRepository) BuscarDeposito(asaasID string) (*models.Transacao, error) {
	var t models.Transacao
	err := r.db.Where("asaas_payment_id = ? AND tipo = ?", asaasID, models.TipoDeposito).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}
