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
	// TarifaSaqueCentavos é a tarifa debitada junto de cada saque (0 = sem tarifa).
	TarifaSaqueCentavos int64 `json:"tarifa_saque_centavos"`
	// Moderação: créditos do pódio ainda fora do saldo (janela de contestação / retidos por denúncia) e
	// débitos abertos. Com débito aberto o saque é recusado (409 debito_aberto).
	ALiberarCentavos     int64 `json:"a_liberar_centavos"`
	RetidoCentavos       int64 `json:"retido_centavos"`
	DebitoAbertoCentavos int64 `json:"debito_aberto_centavos"`
	SaqueBloqueado       bool  `json:"saque_bloqueado"`
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
	// TarifaCentavos é a tarifa cobrada além do valor (0 se não há ou se foi devolvida com a falha do saque).
	// O débito total no saldo é ValorCentavos + TarifaCentavos.
	TarifaCentavos int64 `json:"tarifa_centavos"`
	// EmRevisao: o saque segue pending e aguarda conferência manual (resposta ambígua do Asaas ou
	// transferência não registrada). O valor NÃO volta ao saldo enquanto isso.
	EmRevisao bool `json:"em_revisao"`
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

// ─── Ranking ─────────────────────────────────────────────────────────────────

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
