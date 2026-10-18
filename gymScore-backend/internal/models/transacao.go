package models

import "time"

// TipoTransacao classifica a movimentação no ledger.
type TipoTransacao string

const (
	TipoDeposito       TipoTransacao = "deposito"
	TipoSaque          TipoTransacao = "saque"
	TipoEntradaDesafio TipoTransacao = "entrada_desafio"
	TipoEstornoDesafio TipoTransacao = "estorno_desafio"
	TipoPremioDesafio  TipoTransacao = "premio_desafio"
	// TipoAjuste é a linha de abertura criada pela migração de saldos legados (status received).
	TipoAjuste TipoTransacao = "ajuste"
)

// Status da movimentação no ledger.
const (
	TransacaoPending   = "pending"
	TransacaoReceived  = "received"
	TransacaoRefunded  = "refunded"
	TransacaoFailed    = "failed"
	TransacaoConcluido = "concluido"
)

// Transacao é uma linha do ledger. ValorCentavos é assinado (+crédito / -débito).
// created_at/updated_at não têm default no banco: o GORM os preenche.
type Transacao struct {
	ID              uint          `gorm:"primaryKey;autoIncrement"`
	IDUsuario       uint          `gorm:"column:id_usuario;not null"`
	Tipo            TipoTransacao `gorm:"column:tipo;not null"`
	ValorCentavos   int64         `gorm:"column:valor_centavos;not null"`
	IDDesafio       *uint         `gorm:"column:id_desafio"`
	AsaasPaymentID  *string       `gorm:"column:asaas_payment_id;size:100"`
	AsaasTransferID *string       `gorm:"column:asaas_transfer_id;size:100"`
	Status          string        `gorm:"column:status;not null;default:pending"`
	CreatedAt       time.Time     `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time     `gorm:"column:updated_at;autoUpdateTime"`
	Descricao       *string       `gorm:"column:descricao;size:255"`
}

func (Transacao) TableName() string { return "transacoes" }

// TransacaoResponse é o DTO de uma movimentação (ledger) devolvido ao dono.
type TransacaoResponse struct {
	ID             uint          `json:"id"`
	Tipo           TipoTransacao `json:"tipo"`
	ValorCentavos  int64         `json:"valor_centavos"`
	Valor          float64       `json:"valor"` // reais, apenas exibição
	Status         string        `json:"status"`
	IDDesafio      *uint         `json:"id_desafio,omitempty"`
	AsaasPaymentID *string       `json:"asaas_payment_id,omitempty"`
	Descricao      string        `json:"descricao,omitempty"`
	CriadoEm       time.Time     `json:"criado_em"`
}

// AsaasCreatePaymentRequest é o body para POST /v3/payments
type AsaasCreatePaymentRequest struct {
	Customer    string  `json:"customer"`
	BillingType string  `json:"billingType"`
	Value       float64 `json:"value"`
	DueDate     string  `json:"dueDate"`
	Description string  `json:"description"`
}

// AsaasPaymentResponse é a resposta da criação de cobrança
type AsaasPaymentResponse struct {
	ID          string  `json:"id"`
	Customer    string  `json:"customer"`
	Value       float64 `json:"value"`
	Status      string  `json:"status"`
	BillingType string  `json:"billingType"`
	DueDate     string  `json:"dueDate"`
}

// AsaasPixQrCodeResponse é a resposta de GET /v3/payments/{id}/pixQrCode
type AsaasPixQrCodeResponse struct {
	EncodedImage   string `json:"encodedImage"`
	Payload        string `json:"payload"`
	ExpirationDate string `json:"expirationDate"`
}

// AsaasWebhookPayload é o payload recebido nos webhooks do Asaas
type AsaasWebhookPayload struct {
	Event   string               `json:"event"`
	Payment AsaasPaymentResponse `json:"payment"`
}
