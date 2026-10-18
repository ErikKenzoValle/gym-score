package repositories

import (
	"errors"
	"fmt"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ErrSaldoInsuficiente indica que o débito não pôde ser feito por falta de saldo.
var ErrSaldoInsuficiente = errors.New("saldo insuficiente")

// Lancamento descreve uma movimentação de saldo. ValorCentavos é SEMPRE positivo
// (a direção é dada por Creditar/Debitar); o ledger grava o valor assinado.
type Lancamento struct {
	IDUsuario       uint
	Tipo            models.TipoTransacao
	ValorCentavos   int64
	IDDesafio       *uint
	AsaasPaymentID  *string
	AsaasTransferID *string
	Status          string // vazio = "concluido"
	Descricao       string
}

// CarteiraRepository é o ÚNICO ponto que altera usuarios.saldo_centavos.
// Todos os métodos recebem o *gorm.DB da transação em andamento (Transacao/db.Transaction):
// o UPDATE do saldo e a linha do ledger vivem ou morrem juntos.
type CarteiraRepository struct {
	db *gorm.DB
}

// NovoCarteiraRepository cria o repositório de carteira.
func NovoCarteiraRepository(db *gorm.DB) *CarteiraRepository {
	return &CarteiraRepository{db: db}
}

// DB expõe a conexão base (para abrir transações no service).
func (r *CarteiraRepository) DB() *gorm.DB { return r.db }

// Transacao executa fn dentro de uma transação do banco.
func (r *CarteiraRepository) Transacao(fn func(tx *gorm.DB) error) error {
	return r.db.Transaction(fn)
}

func (l Lancamento) status() string {
	if l.Status == "" {
		return models.TransacaoConcluido
	}
	return l.Status
}

func (l Lancamento) novaTransacao(assinado int64) *models.Transacao {
	t := &models.Transacao{
		IDUsuario:       l.IDUsuario,
		Tipo:            l.Tipo,
		ValorCentavos:   assinado,
		IDDesafio:       l.IDDesafio,
		AsaasPaymentID:  l.AsaasPaymentID,
		AsaasTransferID: l.AsaasTransferID,
		Status:          l.status(),
	}
	if l.Descricao != "" {
		d := l.Descricao
		t.Descricao = &d
	}
	return t
}

// Creditar soma o valor ao saldo (UPDATE atômico) e grava a linha do ledger.
func (r *CarteiraRepository) Creditar(tx *gorm.DB, l Lancamento) (*models.Transacao, error) {
	if l.ValorCentavos <= 0 {
		return nil, fmt.Errorf("valor de crédito deve ser positivo")
	}
	res := tx.Exec("UPDATE usuarios SET saldo_centavos = saldo_centavos + ? WHERE id = ?", l.ValorCentavos, l.IDUsuario)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, fmt.Errorf("usuário %d não encontrado para crédito", l.IDUsuario)
	}
	t := l.novaTransacao(l.ValorCentavos)
	if err := tx.Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// Debitar subtrai o valor do saldo somente se houver saldo suficiente
// (UPDATE ... WHERE saldo_centavos >= ?; RowsAffected == 0 => ErrSaldoInsuficiente).
// SQL cru de propósito: o campo SaldoCentavos do model é somente leitura para o GORM
// e grava a linha do ledger com valor negativo.
func (r *CarteiraRepository) Debitar(tx *gorm.DB, l Lancamento) (*models.Transacao, error) {
	if l.ValorCentavos <= 0 {
		return nil, fmt.Errorf("valor de débito deve ser positivo")
	}
	res := tx.Exec("UPDATE usuarios SET saldo_centavos = saldo_centavos - ? WHERE id = ? AND saldo_centavos >= ?",
		l.ValorCentavos, l.IDUsuario, l.ValorCentavos)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrSaldoInsuficiente
	}
	t := l.novaTransacao(-l.ValorCentavos)
	if err := tx.Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// ConfirmarDeposito efetiva um depósito pendente: muda a linha do ledger de
// 'pending' para 'received' e credita o saldo na MESMA transação. Idempotente:
// só quem vencer o UPDATE ... WHERE status='pending' credita. Retorna true se creditou.
func (r *CarteiraRepository) ConfirmarDeposito(asaasPaymentID string) (bool, error) {
	creditou := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var t models.Transacao
		if err := tx.Where("asaas_payment_id = ? AND tipo = ?", asaasPaymentID, models.TipoDeposito).
			First(&t).Error; err != nil {
			return err
		}
		res := tx.Model(&models.Transacao{}).
			Where("id = ? AND status = ?", t.ID, models.TransacaoPending).
			Update("status", models.TransacaoReceived)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // já processado por outra chamada
		}
		if err := tx.Exec("UPDATE usuarios SET saldo_centavos = saldo_centavos + ? WHERE id = ?",
			t.ValorCentavos, t.IDUsuario).Error; err != nil {
			return err
		}
		creditou = true
		return nil
	})
	return creditou, err
}

// EstornarDeposito trata PAYMENT_REFUNDED: só atua em depósito 'received'. Se o
// usuário tiver saldo, debita o valor e marca 'refunded'. Se o saldo for
// insuficiente (já gastou), NÃO deixa o saldo negativo: nada muda e retorna
// (false, ErrSaldoInsuficiente) para o chamador registrar em log.
func (r *CarteiraRepository) EstornarDeposito(asaasPaymentID string) (bool, error) {
	estornou := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var t models.Transacao
		if err := tx.Where("asaas_payment_id = ? AND tipo = ?", asaasPaymentID, models.TipoDeposito).
			First(&t).Error; err != nil {
			return err
		}
		res := tx.Model(&models.Transacao{}).
			Where("id = ? AND status = ?", t.ID, models.TransacaoReceived).
			Update("status", models.TransacaoRefunded)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		deb := tx.Exec("UPDATE usuarios SET saldo_centavos = saldo_centavos - ? WHERE id = ? AND saldo_centavos >= ?",
			t.ValorCentavos, t.IDUsuario, t.ValorCentavos)
		if deb.Error != nil {
			return deb.Error
		}
		if deb.RowsAffected == 0 {
			// desfaz a mudança de status (rollback da transação inteira)
			return ErrSaldoInsuficiente
		}
		estornou = true
		return nil
	})
	return estornou, err
}

// Saldo devolve o saldo atual em centavos.
func (r *CarteiraRepository) Saldo(userID uint) (int64, error) {
	var saldo int64
	err := r.db.Model(&models.Usuario{}).Select("saldo_centavos").Where("id = ?", userID).Scan(&saldo).Error
	return saldo, err
}

// SomaLedger soma o ledger efetivo do usuário: linhas 'received'/'concluido' e saques
// 'pending' (já debitados). Depósitos pending/refunded e saques failed (reembolsados) ficam de
// fora. Deve ser igual ao saldo; usado em testes de consistência.
func (r *CarteiraRepository) SomaLedger(userID uint) (int64, error) {
	var soma int64
	err := r.db.Model(&models.Transacao{}).
		Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_usuario = ? AND (status IN ? OR (tipo = ? AND status = ?))", userID,
			[]string{models.TransacaoReceived, models.TransacaoConcluido}, models.TipoSaque, models.TransacaoPending).
		Scan(&soma).Error
	return soma, err
}

// ─── Extrato e saques ────────────────────────────────────────────────────────

// Extrato devolve uma página do ledger do usuário (mais recentes primeiro) e o total.
func (r *CarteiraRepository) Extrato(userID uint, pagina, limite int) ([]models.Transacao, int64, error) {
	q := r.db.Model(&models.Transacao{}).Where("id_usuario = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var itens []models.Transacao
	err := q.Order("created_at DESC, id DESC").Offset((pagina - 1) * limite).Limit(limite).Find(&itens).Error
	return itens, total, err
}

// IniciarSaque debita o valor (atômico, exige saldo) e grava o saque como 'pending'.
func (r *CarteiraRepository) IniciarSaque(userID uint, valor int64, descricao string) (*models.Transacao, error) {
	var t *models.Transacao
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var err error
		t, err = r.Debitar(tx, Lancamento{
			IDUsuario: userID, Tipo: models.TipoSaque, ValorCentavos: valor,
			Status: models.TransacaoPending, Descricao: descricao,
		})
		return err
	})
	return t, err
}

// BuscarSaque devolve o saque se for do usuário (nil caso contrário).
func (r *CarteiraRepository) BuscarSaque(id, userID uint) (*models.Transacao, error) {
	var t models.Transacao
	err := r.db.Where("id = ? AND id_usuario = ? AND tipo = ?", id, userID, models.TipoSaque).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}

// MarcarTransferencia grava o ID da transferência do Asaas no saque pendente.
func (r *CarteiraRepository) MarcarTransferencia(id uint, transferID string) error {
	return r.db.Model(&models.Transacao{}).Where("id = ? AND status = ?", id, models.TransacaoPending).
		Update("asaas_transfer_id", transferID).Error
}

// ConcluirSaque muda pending -> concluido (idempotente). Retorna true se esta chamada concluiu.
func (r *CarteiraRepository) ConcluirSaque(id uint) (bool, error) {
	res := r.db.Model(&models.Transacao{}).
		Where("id = ? AND tipo = ? AND status = ?", id, models.TipoSaque, models.TransacaoPending).
		Update("status", models.TransacaoConcluido)
	return res.RowsAffected > 0, res.Error
}

// FalharSaque muda pending -> failed e devolve o valor ao saldo, na mesma transação.
// Idempotente: só quem vencer o UPDATE condicional reembolsa. Retorna true se reembolsou.
func (r *CarteiraRepository) FalharSaque(id uint) (bool, error) {
	reembolsou := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var t models.Transacao
		if err := tx.Where("id = ? AND tipo = ?", id, models.TipoSaque).First(&t).Error; err != nil {
			return err
		}
		res := tx.Model(&models.Transacao{}).
			Where("id = ? AND status = ?", id, models.TransacaoPending).
			Update("status", models.TransacaoFailed)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := tx.Exec("UPDATE usuarios SET saldo_centavos = saldo_centavos + ? WHERE id = ?",
			-t.ValorCentavos, t.IDUsuario).Error; err != nil {
			return err
		}
		reembolsou = true
		return nil
	})
	return reembolsou, err
}
