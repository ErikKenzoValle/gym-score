package models

// PIXRequest representa os dados necessários para gerar um pagamento PIX de depósito
type PIXRequest struct {
	IDUsuario uint    `json:"id_usuario" example:"1"`
	Valor     float64 `json:"valor" example:"50.00"`
	CPF       string  `json:"cpf" example:"123.456.789-00"`
}

// PIXResponse contém o QR Code e dados da cobrança PIX gerada via Asaas
type PIXResponse struct {
	QRCodeBase64   string `json:"qrcode_base64" description:"QR Code em Base64 (PNG)"`
	Payload        string `json:"payload" description:"Código PIX Copia e Cola"`
	ExpirationDate string `json:"expiration_date" description:"Data de expiração da cobrança"`
	AsaasPaymentID string `json:"asaas_payment_id" description:"ID da cobrança no Asaas"`
}

// ExtratoResponse é o DTO de retorno do extrato financeiro do usuário
type ExtratoResponse struct {
	TotalEntradas float64     `json:"total_entradas"`
	TotalSaidas   float64     `json:"total_saidas"`
	Saldo         float64     `json:"saldo"`
	Transacoes    []Transacao `json:"transacoes"`
}
