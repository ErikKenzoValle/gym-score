package models

import "time"

// Tipos de chave PIX aceitos pelo Asaas.
const (
	ChaveCPF   = "CPF"
	ChaveCNPJ  = "CNPJ"
	ChaveEmail = "EMAIL"
	ChaveFone  = "PHONE"
	ChaveEVP   = "EVP"
)

// SaqueMinCentavos / SaqueMaxCentavos limitam o saque PIX (R$ 5,00 a R$ 10.000,00).
const (
	SaqueMinCentavos int64 = 500
	SaqueMaxCentavos int64 = 1_000_000
)

// SaqueRequest é o DTO do saque PIX. O usuário é sempre o do JWT.
type SaqueRequest struct {
	ValorCentavos int64  `json:"valor_centavos" validate:"required,gte=500,lte=1000000"`
	ChavePix      string `json:"chave_pix" validate:"required,min=3,max=140"`
	TipoChave     string `json:"tipo_chave" validate:"required,oneof=CPF CNPJ EMAIL PHONE EVP"`
}

// SaldoResponse é o saldo da carteira.
type SaldoResponse struct {
	SaldoCentavos int64   `json:"saldo_centavos"`
	Saldo         float64 `json:"saldo"` // reais, só exibição
}

// SaqueResponse descreve um saque.
type SaqueResponse struct {
	ID              uint      `json:"id"`
	ValorCentavos   int64     `json:"valor_centavos"` // positivo (valor sacado)
	Valor           float64   `json:"valor"`
	Status          string    `json:"status"` // pending | concluido | failed
	AsaasTransferID *string   `json:"asaas_transfer_id,omitempty"`
	Descricao       string    `json:"descricao,omitempty"`
	CriadoEm        time.Time `json:"criado_em"`
}

// ExtratoResponse é uma página do ledger do usuário.
type ExtratoResponse struct {
	Itens  []TransacaoResponse `json:"itens"`
	Total  int64               `json:"total"`
	Pagina int                 `json:"pagina"`
	Limite int                 `json:"limite"`
}

// AsaasTransferResponse é a resposta de POST/GET /v3/transfers.
type AsaasTransferResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"` // PENDING, BANK_PROCESSING, DONE, CANCELLED, FAILED
	FailReason string `json:"failReason"`
}

// ─── Check-ins e ranking ─────────────────────────────────────────────────────

// DesafioCheckin é a prova de treino de um participante (1 por dia).
type DesafioCheckin struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	IDDesafio uint      `gorm:"column:id_desafio;not null"`
	IDUsuario uint      `gorm:"column:id_usuario;not null"`
	Texto     *string   `gorm:"column:texto;size:500"`
	FotoPath  *string   `gorm:"column:foto_path;size:255"`
	Latitude  *float64  `gorm:"column:latitude"`
	Longitude *float64  `gorm:"column:longitude"`
	Data      string    `gorm:"column:data;type:date;not null"`
	CriadoEm  time.Time `gorm:"column:criado_em;autoCreateTime"`

	Usuario *Usuario `gorm:"foreignKey:IDUsuario"`
}

func (DesafioCheckin) TableName() string { return "desafio_checkins" }

// CheckinResponse é o DTO de um check-in.
type CheckinResponse struct {
	ID        uint           `json:"id"`
	Usuario   UsuarioPublico `json:"usuario"`
	Texto     string         `json:"texto,omitempty"`
	FotoURL   string         `json:"foto_url,omitempty"`
	Latitude  *float64       `json:"latitude,omitempty"`
	Longitude *float64       `json:"longitude,omitempty"`
	Data      string         `json:"data"`
	CriadoEm  time.Time      `json:"criado_em"`
}

// RankingItem é uma linha do ranking entre amigos.
type RankingItem struct {
	Posicao    int    `json:"posicao"`
	ID         uint   `json:"id"`
	Nome       string `json:"nome"`
	Sobrenome  string `json:"sobrenome"`
	Exercicios int64  `json:"exercicios"`
	Checkins   int64  `json:"checkins"`
	Pontos     int64  `json:"pontos"`
	Voce       bool   `json:"voce"`
}
