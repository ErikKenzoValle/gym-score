package services

import (
	"errors"
	"fmt"
	"log"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"gorm.io/gorm"
)

// AsaasGateway é o subconjunto da API do Asaas usado pelo PIXService
// (implementado por *client.AsaasClient; substituível por um fake nos testes).
type AsaasGateway interface {
	CriarCobrancaPix(customerName, cpf string, valor float64, description string) (*models.AsaasPaymentResponse, error)
	BuscarPixQrCode(paymentID string) (*models.AsaasPixQrCodeResponse, error)
	BuscarStatusPagamento(paymentID string) (string, error)
	SimularRecebimento(paymentID string, valor float64) error
	CriarTransferenciaPix(valor float64, chave, tipoChave, descricao string) (*models.AsaasTransferResponse, error)
	BuscarTransferencia(id string) (*models.AsaasTransferResponse, error)
}

// PIXService trata depósitos PIX.
type PIXService interface {
	GerarPagamento(userID uint, req models.PIXRequest) (*models.PIXResponse, error)
	// ConsultarPagamento confirma por polling no Asaas; só o dono do depósito enxerga.
	ConsultarPagamento(userID uint, asaasID string) (*models.TransacaoResponse, error)
	SimularPagamento(userID uint, asaasID string) error
	// ConfirmarRecebimento / EstornarRecebimento são o caminho do webhook (token já validado).
	ConfirmarRecebimento(asaasID string) error
	EstornarRecebimento(asaasID string) error
}

type pixService struct {
	asaas         AsaasGateway
	usuarioRepo   repositories.UsuarioRepository
	transacaoRepo repositories.TransacaoRepository
	carteira      *repositories.CarteiraRepository
	notif         Notificador
}

func NovoPIXService(
	asaas AsaasGateway,
	usuarioRepo repositories.UsuarioRepository,
	transacaoRepo repositories.TransacaoRepository,
	carteira *repositories.CarteiraRepository,
	notif Notificador,
) PIXService {
	return &pixService{asaas, usuarioRepo, transacaoRepo, carteira, notif}
}

func paraResponse(t *models.Transacao) *models.TransacaoResponse {
	r := &models.TransacaoResponse{
		ID: t.ID, Tipo: t.Tipo, ValorCentavos: t.ValorCentavos,
		Valor: utils.CentavosParaReais(t.ValorCentavos), Status: t.Status,
		IDDesafio: t.IDDesafio, AsaasPaymentID: t.AsaasPaymentID, CriadoEm: t.CreatedAt,
	}
	if t.Descricao != nil {
		r.Descricao = *t.Descricao
	}
	return r
}

func statusRecebido(status string) bool {
	return status == "RECEIVED" || status == "CONFIRMED" || status == "RECEIVED_IN_CASH"
}

// ConfirmarRecebimento credita o depósito (idempotente) — compartilhado por polling e webhook.
func (s *pixService) ConfirmarRecebimento(asaasID string) error {
	creditou, err := s.carteira.ConfirmarDeposito(asaasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return erroNaoEncontrado("transação não encontrada")
	}
	if err == nil && creditou { // só quem de fato creditou notifica (idempotente)
		if t, errB := s.transacaoRepo.BuscarDeposito(asaasID); errB == nil && t != nil {
			s.notif.Notificar(t.IDUsuario, models.NotifDeposito, "Depósito confirmado",
				fmt.Sprintf("Seu depósito de %s foi creditado na carteira.", reaisBR(t.ValorCentavos)), "/carteira")
		}
	}
	return err
}

// EstornarRecebimento trata PAYMENT_REFUNDED: debita sem deixar o saldo negativo.
func (s *pixService) EstornarRecebimento(asaasID string) error {
	_, err := s.carteira.EstornarDeposito(asaasID)
	if errors.Is(err, repositories.ErrSaldoInsuficiente) {
		log.Printf("[PIX] estorno do pagamento %s NÃO aplicado: saldo do usuário insuficiente (revisar manualmente)", asaasID)
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("[PIX] estorno de pagamento desconhecido: %s", asaasID)
		return nil
	}
	return err
}

func (s *pixService) depositoDoUsuario(userID uint, asaasID string) (*models.Transacao, error) {
	t, err := s.transacaoRepo.BuscarDepositoDoUsuario(asaasID, userID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		// Mesma resposta para "não existe" e "é de outro usuário": não revela existência
		return nil, erroNaoEncontrado("Pagamento não encontrado")
	}
	return t, nil
}

// ConsultarPagamento confirma o pagamento por polling ativo: consulta o status
// direto na API do Asaas (sem depender de webhook) e credita uma única vez.
func (s *pixService) ConsultarPagamento(userID uint, asaasID string) (*models.TransacaoResponse, error) {
	t, err := s.depositoDoUsuario(userID, asaasID)
	if err != nil {
		return nil, err
	}
	if t.Status == models.TransacaoPending {
		status, err := s.asaas.BuscarStatusPagamento(asaasID)
		if err != nil {
			// Não falha o polling: devolve o estado atual e tenta de novo no próximo ciclo
			log.Printf("[PIX] falha ao consultar pagamento %s no Asaas: %v", asaasID, err)
			return paraResponse(t), nil
		}
		if statusRecebido(status) {
			if err := s.ConfirmarRecebimento(asaasID); err != nil {
				return nil, err
			}
			if t, err = s.depositoDoUsuario(userID, asaasID); err != nil {
				return nil, err
			}
		}
	}
	return paraResponse(t), nil
}

// SimularPagamento (sandbox) marca a cobrança como recebida no Asaas e confirma o crédito.
func (s *pixService) SimularPagamento(userID uint, asaasID string) error {
	t, err := s.depositoDoUsuario(userID, asaasID)
	if err != nil {
		return err
	}
	if t.Status != models.TransacaoPending {
		return nil
	}
	if err := s.asaas.SimularRecebimento(asaasID, utils.CentavosParaReais(t.ValorCentavos)); err != nil {
		return err
	}
	_, err = s.ConsultarPagamento(userID, asaasID)
	return err
}

func (s *pixService) GerarPagamento(userID uint, req models.PIXRequest) (*models.PIXResponse, error) {
	if req.ValorCentavos < 500 {
		return nil, erroRequisicao("valor mínimo para depósito PIX é R$ 5,00")
	}
	if !utils.ValidarCPF(req.CPF) {
		return nil, erroRequisicao("CPF informado é inválido")
	}
	usuario, err := s.usuarioRepo.BuscarPorID(userID)
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		return nil, erroNaoEncontrado("usuário não encontrado")
	}

	descricao := fmt.Sprintf("Depósito GymScore - usuário %d", usuario.ID)
	payment, err := s.asaas.CriarCobrancaPix(
		fmt.Sprintf("%s %s", usuario.Nome, usuario.Sobrenome),
		req.CPF, utils.CentavosParaReais(req.ValorCentavos), descricao,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar cobrança no Asaas: %w", err)
	}
	qrCode, err := s.asaas.BuscarPixQrCode(payment.ID)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter QR Code PIX: %w", err)
	}

	id := payment.ID
	desc := "Depósito via PIX"
	if err := s.transacaoRepo.CriarDepositoPendente(&models.Transacao{
		IDUsuario:      usuario.ID,
		ValorCentavos:  req.ValorCentavos,
		AsaasPaymentID: &id,
		Descricao:      &desc,
	}); err != nil {
		return nil, fmt.Errorf("falha ao salvar transação: %w", err)
	}

	return &models.PIXResponse{
		QRCodeBase64:   qrCode.EncodedImage,
		Payload:        qrCode.Payload,
		ExpirationDate: qrCode.ExpirationDate,
		AsaasPaymentID: payment.ID,
	}, nil
}
