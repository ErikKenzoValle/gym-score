package models

// PIXRequest representa os dados necessários para gerar um pagamento PIX de depósito.
// O usuário é sempre o do JWT; o valor é enviado em centavos.
type PIXRequest struct {
	ValorCentavos int64  `json:"valor_centavos" validate:"required,gte=500,lte=100000000" example:"5000"`
	CPF           string `json:"cpf" validate:"required" example:"123.456.789-00"`
}

// PIXResponse contém o QR Code e dados da cobrança PIX gerada via Asaas
type PIXResponse struct {
	QRCodeBase64   string `json:"qrcode_base64" description:"QR Code em Base64 (PNG)"`
	Payload        string `json:"payload" description:"Código PIX Copia e Cola"`
	ExpirationDate string `json:"expiration_date" description:"Data de expiração da cobrança"`
	AsaasPaymentID string `json:"asaas_payment_id" description:"ID da cobrança no Asaas"`
}
