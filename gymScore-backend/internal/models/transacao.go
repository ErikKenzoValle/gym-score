package models

import "time"

type Transacao struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario      uint      `gorm:"column:id_usuario;not null" json:"id_usuario"`
	AsaasPaymentID string    `gorm:"column:asaas_payment_id;size:100;uniqueIndex;not null" json:"asaas_payment_id"`
	Valor          float64   `gorm:"column:valor;type:decimal(10,2);not null" json:"valor"`
	Status         string    `gorm:"column:status;type:enum('pending','received','refunded');default:'pending'" json:"status"`
	// Tipo classifica a transação como entrada (depósito) ou saída (pagamento de desafio, saque)
	Tipo           string    `gorm:"column:tipo;type:enum('entrada','saida');default:'entrada'" json:"tipo"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Transacao) TableName() string { return "transacoes" }

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
