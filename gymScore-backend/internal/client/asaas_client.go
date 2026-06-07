package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gynScore-backend/internal/config"
	"gynScore-backend/internal/models"
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
		return nil, fmt.Errorf("asaas error %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
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
func (c *AsaasClient) CriarCobrancaPix(customerName, cpf string, valor float64, description string) (*models.AsaasPaymentResponse, error) {
	customerID, err := c.buscarOuCriarCustomer(customerName, cpf)
	if err != nil {
		return nil, err
	}

	dueDate := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	reqBody := models.AsaasCreatePaymentRequest{
		Customer:    customerID,
		BillingType: "PIX",
		Value:       valor,
		DueDate:     dueDate,
		Description: description,
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
func (c *AsaasClient) SimularRecebimento(paymentID string, valor float64) error {
	body := map[string]any{
		"paymentDate":    time.Now().Format("2006-01-02"),
		"value":          valor,
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
