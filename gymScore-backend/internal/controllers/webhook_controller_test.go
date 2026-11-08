package controllers_test

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/controllers"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"

	"github.com/gofiber/fiber/v2"
)

type pixFake struct {
	confirmados, estornados []string
	erro                    error // devolvido pelo processamento (simula falha)
}

func (p *pixFake) ConfirmarRecebimento(id string) error {
	p.confirmados = append(p.confirmados, id)
	return p.erro
}
func (p *pixFake) EstornarRecebimento(id string) error {
	p.estornados = append(p.estornados, id)
	return p.erro
}

func chamar(t *testing.T, token, corpo, tokenCfg string) (int, *pixFake) {
	return chamarComErro(t, token, corpo, tokenCfg, nil)
}

func chamarComErro(t *testing.T, token, corpo, tokenCfg string, falha error) (int, *pixFake) {
	t.Helper()
	fake := &pixFakeCompleto{pixFake{erro: falha}}
	app := fiber.New()
	ctrl := controllers.NovoWebhookController(fake, &config.Config{AsaasWebhookToken: tokenCfg})
	app.Post("/w", ctrl.ReceberWebhookAsaas)
	req := httptest.NewRequest("POST", "/w", bytes.NewBufferString(corpo))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("asaas-access-token", token)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, &fake.pixFake
}

func TestWebhook_DesativadoSemTokenConfigurado(t *testing.T) {
	status, fake := chamar(t, "qualquer", `{"event":"PAYMENT_RECEIVED","payment":{"id":"p1"}}`, "")
	if status != 404 || len(fake.confirmados) != 0 {
		t.Errorf("sem token configurado deve responder 404 sem processar (status %d)", status)
	}
}

func TestWebhook_TokenIncorretoOuAusente(t *testing.T) {
	for _, tk := range []string{"", "errado", "segredo-"} {
		status, fake := chamar(t, tk, `{"event":"PAYMENT_RECEIVED","payment":{"id":"p1"}}`, "segredo")
		if status != 401 || len(fake.confirmados) != 0 {
			t.Errorf("token %q: esperava 401 sem processar, obteve %d", tk, status)
		}
	}
}

func TestWebhook_EventosRoteados(t *testing.T) {
	casos := map[string]struct{ conf, est int }{
		"PAYMENT_RECEIVED":  {1, 0},
		"PAYMENT_CONFIRMED": {1, 0},
		"PAYMENT_REFUNDED":  {0, 1},
		"PAYMENT_OVERDUE":   {0, 0},
	}
	for evento, esperado := range casos {
		status, fake := chamar(t, "segredo", `{"event":"`+evento+`","payment":{"id":"p1"}}`, "segredo")
		if status != 200 || len(fake.confirmados) != esperado.conf || len(fake.estornados) != esperado.est {
			t.Errorf("%s: status=%d confirmados=%d estornados=%d", evento, status, len(fake.confirmados), len(fake.estornados))
		}
	}
	// corpo ilegível: 200 para o Asaas parar de reenviar
	if status, _ := chamar(t, "segredo", `não é json`, "segredo"); status != 200 {
		t.Errorf("payload inválido deveria responder 200, obteve %d", status)
	}
}

// pixFakeCompleto implementa services.PIXService (apenas os métodos do webhook importam).
type pixFakeCompleto struct{ pixFake }

func (p *pixFakeCompleto) GerarPagamento(uint, models.PIXRequest) (*models.PIXResponse, error) {
	return nil, nil
}
func (p *pixFakeCompleto) ConsultarPagamento(uint, string) (*models.TransacaoResponse, error) {
	return nil, nil
}
func (p *pixFakeCompleto) SimularPagamento(uint, string) error   { return nil }
func (p *pixFakeCompleto) ReconciliarDepositos(int) (int, error) { return 0, nil }

// Falha de banco ao gravar => 5xx (o Asaas reenvia); erro de negócio (cobrança desconhecida) => 200.
func TestWebhook_FalhaDeGravacaoResponde5xx(t *testing.T) {
	for _, ev := range []string{"PAYMENT_RECEIVED", "PAYMENT_REFUNDED"} {
		status, _ := chamarComErro(t, "segredo", `{"event":"`+ev+`","payment":{"id":"p1"}}`, "segredo", errors.New("deadlock"))
		if status != 500 {
			t.Errorf("%s com falha de banco: esperava 500, obteve %d", ev, status)
		}
	}
	naoAchou := &services.ErroNegocio{Status: 404, Mensagem: "transação não encontrada"}
	if status, _ := chamarComErro(t, "segredo", `{"event":"PAYMENT_RECEIVED","payment":{"id":"x"}}`, "segredo", naoAchou); status != 200 {
		t.Errorf("cobrança desconhecida: esperava 200, obteve %d", status)
	}
}
