package controllers

import (
	"gynScore-backend/internal/config"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"log"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type WebhookController struct {
	db            *gorm.DB
	transacaoRepo repositories.TransacaoRepository
	usuarioRepo   repositories.UsuarioRepository
	webhookToken  string
}

func NovoWebhookController(
	db *gorm.DB,
	transacaoRepo repositories.TransacaoRepository,
	usuarioRepo repositories.UsuarioRepository,
	cfg *config.Config,
) *WebhookController {
	return &WebhookController{db, transacaoRepo, usuarioRepo, cfg.AsaasWebhookToken}
}

// ReceberWebhookAsaas processa eventos de pagamento enviados pelo Asaas
func (ctrl *WebhookController) ReceberWebhookAsaas(c *fiber.Ctx) error {
	// Validar token do Asaas (header asaas-access-token)
	if ctrl.webhookToken != "" {
		token := c.Get("asaas-access-token")
		if token != ctrl.webhookToken {
			log.Printf("[WEBHOOK] Token inválido — requisição rejeitada (IP: %s)", c.IP())
			return c.SendStatus(fiber.StatusUnauthorized)
		}
	}

	var payload models.AsaasWebhookPayload
	if err := c.BodyParser(&payload); err != nil {
		log.Printf("[WEBHOOK] Erro ao parsear payload: %v", err)
		return c.SendStatus(fiber.StatusOK)
	}

	switch payload.Event {
	case "PAYMENT_RECEIVED":
		if err := ctrl.handlePaymentReceived(payload.Payment.ID, payload.Payment.Value); err != nil {
			log.Printf("[WEBHOOK] Erro ao processar PAYMENT_RECEIVED %s: %v", payload.Payment.ID, err)
		}
	case "PAYMENT_REFUNDED":
		if err := ctrl.transacaoRepo.AtualizarStatus(payload.Payment.ID, "refunded"); err != nil {
			log.Printf("[WEBHOOK] Erro ao atualizar status refunded %s: %v", payload.Payment.ID, err)
		}
	default:
		log.Printf("[WEBHOOK] Evento ignorado: %s", payload.Event)
	}

	// Asaas requer 200 para parar retries
	return c.SendStatus(fiber.StatusOK)
}

func (ctrl *WebhookController) handlePaymentReceived(asaasPaymentID string, valor float64) error {
	transacao, err := ctrl.transacaoRepo.BuscarPorAsaasID(asaasPaymentID)
	if err != nil {
		return err
	}

	// idempotência: ignorar se já processado
	if transacao.Status == "received" {
		return nil
	}

	return ctrl.db.Transaction(func(tx *gorm.DB) error {
		usuario, err := ctrl.usuarioRepo.BuscarPorID(transacao.IDUsuario)
		if err != nil {
			return err
		}

		usuario.Saldo += transacao.Valor
		if err := tx.Save(usuario).Error; err != nil {
			return err
		}

		return tx.Model(&models.Transacao{}).
			Where("asaas_payment_id = ?", asaasPaymentID).
			Update("status", "received").Error
	})
}
