package services

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"gorm.io/gorm"
)

// AsaasGateway é o subconjunto da API do Asaas usado pelo PIXService
// (implementado por *client.AsaasClient; substituível por um fake nos testes).
type AsaasGateway interface {
	// Valores sempre em centavos (o cliente monta o decimal exato, sem float).
	CriarCobrancaPix(customerName, cpf string, valorCentavos int64, description, externalReference string) (*models.AsaasPaymentResponse, error)
	BuscarPixQrCode(paymentID string) (*models.AsaasPixQrCodeResponse, error)
	BuscarStatusPagamento(paymentID string) (string, error)
	SimularRecebimento(paymentID string, valorCentavos int64) error
	// CriarTransferenciaPix: o erro deve permitir client.ErroDefinitivo (4xx) x ambíguo (resto).
	CriarTransferenciaPix(valorCentavos int64, chave, tipoChave, descricao, externalReference string) (*models.AsaasTransferResponse, error)
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
	// ReconciliarDepositos (job) consulta no Asaas os depósitos pending antigos e credita os pagos;
	// depósitos sem cobrança (criação falhou) viram failed. Devolve quantos foram consultados.
	ReconciliarDepositos(limite int) (int, error)
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
	if err := s.asaas.SimularRecebimento(asaasID, t.ValorCentavos); err != nil {
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

	// A linha local (pending, sem cobrança) nasce ANTES da cobrança: se o processo cair depois de
	// criar a cobrança, a externalReference "deposito-<id>" a liga a esta linha, e a reconciliação
	// marca como failed as linhas que ficaram sem cobrança.
	desc := "Depósito via PIX"
	t := &models.Transacao{IDUsuario: usuario.ID, ValorCentavos: req.ValorCentavos, Descricao: &desc}
	if err := s.transacaoRepo.CriarDepositoPendente(t); err != nil {
		return nil, fmt.Errorf("falha ao salvar transação: %w", err)
	}
	descricao := fmt.Sprintf("Depósito GymScore - usuário %d", usuario.ID)
	payment, err := s.asaas.CriarCobrancaPix(
		fmt.Sprintf("%s %s", usuario.Nome, usuario.Sobrenome),
		req.CPF, req.ValorCentavos, descricao, fmt.Sprintf("deposito-%d", t.ID),
	)
	if err != nil {
		// Depósito nunca credita sem pagamento confirmado: marcar failed não arrisca dinheiro.
		if errF := s.transacaoRepo.FalharDeposito(t.ID); errF != nil {
			log.Printf("[PIX] ERRO ao marcar depósito %d como failed: %v", t.ID, errF)
		}
		return nil, fmt.Errorf("falha ao criar cobrança no Asaas: %w", err)
	}
	if err := s.transacaoRepo.VincularCobranca(t.ID, payment.ID); err != nil {
		return nil, fmt.Errorf("falha ao salvar transação: %w", err)
	}
	qrCode, err := s.asaas.BuscarPixQrCode(payment.ID)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter QR Code PIX: %w", err)
	}

	return &models.PIXResponse{
		QRCodeBase64:   qrCode.EncodedImage,
		Payload:        qrCode.Payload,
		ExpirationDate: qrCode.ExpirationDate,
		AsaasPaymentID: payment.ID,
	}, nil
}

// prazoReconciliarDeposito: depósitos pending mais novos que isso ficam com o polling/webhook.
const prazoReconciliarDeposito = 10 * time.Minute

// ReconciliarDepositos é o passo do job de reconciliação para depósitos.
func (s *pixService) ReconciliarDepositos(limite int) (int, error) {
	antes := time.Now().Add(-prazoReconciliarDeposito)
	if n, err := s.carteira.FalharDepositosSemCobranca(antes); err != nil {
		log.Printf("[RECONCILIACAO] depósitos sem cobrança: %v", err)
	} else if n > 0 {
		log.Printf("[RECONCILIACAO] %d depósito(s) sem cobrança marcados como failed", n)
	}
	ts, err := s.carteira.ListarDepositosPendentes(antes, limite)
	if err != nil {
		return 0, err
	}
	for _, t := range ts {
		status, err := s.asaas.BuscarStatusPagamento(*t.AsaasPaymentID)
		if err != nil {
			log.Printf("[RECONCILIACAO] depósito %d (%s): %v", t.ID, *t.AsaasPaymentID, err)
			continue
		}
		if statusRecebido(status) {
			if err := s.ConfirmarRecebimento(*t.AsaasPaymentID); err != nil {
				log.Printf("[RECONCILIACAO] confirmar depósito %d: %v", t.ID, err)
			}
		}
	}
	return len(ts), nil
}
