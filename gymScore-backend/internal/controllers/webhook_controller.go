package controllers

import (
	"crypto/subtle"
	"errors"
	"log"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"

	"github.com/gofiber/fiber/v2"
)

type WebhookController struct {
	pixService   services.PIXService
	webhookToken string
}

func NovoWebhookController(pixService services.PIXService, cfg *config.Config) *WebhookController {
	if cfg.AsaasWebhookToken == "" {
		log.Println("[WEBHOOK] ASAAS_WEBHOOK_TOKEN vazio: webhook DESATIVADO (a rota responde 404; pagamentos são confirmados por polling)")
	}
	return &WebhookController{pixService, cfg.AsaasWebhookToken}
}

// ReceberWebhookAsaas godoc
// @Summary     Webhook do Asaas
// @Description Desativado (404) sem ASAAS_WEBHOOK_TOKEN; exige o header asaas-access-token.
// @Tags        pagamento
// @Accept      json
// @Param       body body models.AsaasWebhookPayload true "Evento"
// @Success     200
// @Failure     500 "falha ao gravar: o Asaas reenvia"
// @Router      /api/webhooks/asaas [post]
//
// ReceberWebhookAsaas processa eventos de pagamento enviados pelo Asaas.
// Desativado (404) quando não há token configurado.
func (ctrl *WebhookController) ReceberWebhookAsaas(c *fiber.Ctx) error {
	if ctrl.webhookToken == "" {
		return c.SendStatus(fiber.StatusNotFound)
	}
	token := c.Get("asaas-access-token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(ctrl.webhookToken)) != 1 {
		log.Printf("[WEBHOOK] Token inválido — requisição rejeitada (IP: %s)", c.IP())
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var payload models.AsaasWebhookPayload
	if err := c.BodyParser(&payload); err != nil {
		log.Printf("[WEBHOOK] Erro ao parsear payload: %v", err)
		return c.SendStatus(fiber.StatusOK)
	}

	var err error
	switch payload.Event {
	case "PAYMENT_RECEIVED", "PAYMENT_CONFIRMED":
		// Mesmo caminho idempotente do polling
		err = ctrl.pixService.ConfirmarRecebimento(payload.Payment.ID)
	case "PAYMENT_REFUNDED":
		err = ctrl.pixService.EstornarRecebimento(payload.Payment.ID)
	default:
		log.Printf("[WEBHOOK] Evento ignorado: %s", payload.Event)
	}
	if err != nil {
		var neg *services.ErroNegocio
		if errors.As(err, &neg) && neg.Status < 500 {
			// Evento de cobrança desconhecida (ou equivalente): reenviar não muda nada
			log.Printf("[WEBHOOK] %s %s ignorado: %v", payload.Event, payload.Payment.ID, err)
			return c.SendStatus(fiber.StatusOK)
		}
		// Falha de infraestrutura (banco): 5xx faz o Asaas reenviar; o processamento é idempotente.
		log.Printf("[WEBHOOK] Erro ao processar %s %s: %v", payload.Event, payload.Payment.ID, err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// 200 = processado, já processado ou ignorado (o Asaas para de reenviar)
	return c.SendStatus(fiber.StatusOK)
}
