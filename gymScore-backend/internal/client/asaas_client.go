package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type AsaasClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewAsaasClient(cfg *config.Config) *AsaasClient {
	return &AsaasClient{
		baseURL: cfg.AsaasBaseURL,
		apiKey:  cfg.AsaasAPIKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *AsaasClient) do(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		log.Printf("[ASAAS] %s %s → %d: %s", method, path, resp.StatusCode, string(data))
		return nil, &ErroAsaas{Status: resp.StatusCode, Corpo: string(data)}
	}
	return data, nil
}

// ErroAsaas é a resposta HTTP de erro (>= 400) da API do Asaas. Erros de rede/timeout NÃO viram
// ErroAsaas: chegam como o erro do http.Client (ver ErroDefinitivo).
type ErroAsaas struct {
	Status int
	Corpo  string
}

func (e *ErroAsaas) Error() string { return fmt.Sprintf("asaas error %d: %s", e.Status, e.Corpo) }

// ErroDefinitivo diz se o erro garante que a operação NÃO foi criada no Asaas: só uma resposta 4xx
// de validação/autorização (400, 401, 403, 404, 422...). Todo o resto é AMBÍGUO — timeout, conexão
// caída, 5xx de gateway, 408/409/425/429, resposta 2xx ilegível ou sem ID — e a operação pode ter
// sido criada: quem chama não pode desfazer o efeito local (ex.: reembolsar um saque) com base nele.
func ErroDefinitivo(err error) bool {
	var e *ErroAsaas
	if !errors.As(err, &e) {
		return false
	}
	switch e.Status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	}
	return e.Status >= 400 && e.Status < 500
}

// ValorReais converte centavos no número decimal que o Asaas espera ("1234.05"), sem passar por
// float64 (que não representa todo valor em centavos exatamente).
func ValorReais(centavos int64) json.Number {
	sinal := ""
	if centavos < 0 {
		sinal, centavos = "-", -centavos
	}
	return json.Number(fmt.Sprintf("%s%d.%02d", sinal, centavos/100, centavos%100))
}

// buscarOuCriarCustomer retorna o customerID do Asaas pelo CPF, criando se necessário
func (c *AsaasClient) buscarOuCriarCustomer(name, cpfCnpj string) (string, error) {
	// Asaas exige CPF/CNPJ sem máscara (somente dígitos)
	cpfCnpj = strings.NewReplacer(".", "", "-", "", "/", "", " ", "").Replace(cpfCnpj)

	data, err := c.do("GET", fmt.Sprintf("/v3/customers?cpfCnpj=%s", cpfCnpj), nil)
	if err != nil {
		return "", fmt.Errorf("falha ao buscar customer: %w", err)
	}

	var listResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return "", err
	}
	if len(listResp.Data) > 0 {
		return listResp.Data[0].ID, nil
	}

	payload := map[string]string{
		"name":    name,
		"cpfCnpj": cpfCnpj,
	}
	data, err = c.do("POST", "/v3/customers", payload)
	if err != nil {
		return "", fmt.Errorf("falha ao criar customer: %w", err)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("customer criado sem ID na resposta")
	}
	return created.ID, nil
}

// CriarCobrancaPix cria uma cobrança PIX no Asaas e retorna a resposta
// externalReference (ex.: "deposito-<id>") liga a cobrança à linha local do ledger.
func (c *AsaasClient) CriarCobrancaPix(customerName, cpf string, valorCentavos int64, description, externalReference string) (*models.AsaasPaymentResponse, error) {
	customerID, err := c.buscarOuCriarCustomer(customerName, cpf)
	if err != nil {
		return nil, err
	}

	dueDate := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	reqBody := models.AsaasCreatePaymentRequest{
		Customer:          customerID,
		BillingType:       "PIX",
		Value:             ValorReais(valorCentavos),
		DueDate:           dueDate,
		Description:       description,
		ExternalReference: externalReference,
	}

	data, err := c.do("POST", "/v3/payments", reqBody)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar cobrança PIX: %w", err)
	}

	var resp models.AsaasPaymentResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BuscarPixQrCode retorna o QR Code de uma cobrança PIX
func (c *AsaasClient) BuscarPixQrCode(paymentID string) (*models.AsaasPixQrCodeResponse, error) {
	data, err := c.do("GET", fmt.Sprintf("/v3/payments/%s/pixQrCode", paymentID), nil)
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar QR Code PIX: %w", err)
	}

	var resp models.AsaasPixQrCodeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SimularRecebimento marca uma cobrança como recebida em dinheiro (receiveInCash).
// Usado APENAS no sandbox para simular a confirmação de um pagamento PIX.
func (c *AsaasClient) SimularRecebimento(paymentID string, valorCentavos int64) error {
	body := map[string]any{
		"paymentDate":    time.Now().Format("2006-01-02"),
		"value":          ValorReais(valorCentavos),
		"notifyCustomer": false,
	}
	_, err := c.do("POST", fmt.Sprintf("/v3/payments/%s/receiveInCash", paymentID), body)
	if err != nil {
		return fmt.Errorf("falha ao simular recebimento: %w", err)
	}
	return nil
}

// BuscarStatusPagamento consulta o status atual de uma cobrança no Asaas.
// Usado para confirmação por polling (sem depender de webhook).
// Status possíveis: PENDING, RECEIVED, CONFIRMED, OVERDUE, REFUNDED, etc.
func (c *AsaasClient) BuscarStatusPagamento(paymentID string) (string, error) {
	data, err := c.do("GET", fmt.Sprintf("/v3/payments/%s", paymentID), nil)
	if err != nil {
		return "", fmt.Errorf("falha ao consultar pagamento: %w", err)
	}

	var resp models.AsaasPaymentResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Status, nil
}

// CriarTransferenciaPix cria uma transferência PIX (saque) via POST /v3/transfers.
// tipoChave: CPF, CNPJ, EMAIL, PHONE ou EVP. externalReference ("saque-<id>") identifica o saque local
// no painel do Asaas (reconciliação manual). Use ErroDefinitivo para decidir se é seguro reembolsar.
func (c *AsaasClient) CriarTransferenciaPix(valorCentavos int64, chave, tipoChave, descricao, externalReference string) (*models.AsaasTransferResponse, error) {
	body := map[string]any{
		"value":             ValorReais(valorCentavos),
		"operationType":     "PIX",
		"pixAddressKey":     chave,
		"pixAddressKeyType": tipoChave,
		"description":       descricao,
		"externalReference": externalReference,
	}
	data, err := c.do("POST", "/v3/transfers", body)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar transferência PIX: %w", err)
	}
	var resp models.AsaasTransferResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		// 2xx ilegível: a transferência pode existir (erro ambíguo, não é ErroAsaas)
		return nil, fmt.Errorf("resposta ilegível ao criar transferência: %w", err)
	}
	if resp.ID == "" {
		return nil, fmt.Errorf("transferência criada sem ID na resposta")
	}
	return &resp, nil
}

// BuscarTransferencia consulta uma transferência (GET /v3/transfers/{id}) para o polling de status.
func (c *AsaasClient) BuscarTransferencia(id string) (*models.AsaasTransferResponse, error) {
	data, err := c.do("GET", fmt.Sprintf("/v3/transfers/%s", id), nil)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar transferência: %w", err)
	}
	var resp models.AsaasTransferResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
