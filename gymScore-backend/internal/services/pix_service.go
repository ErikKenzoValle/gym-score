package services

import (
	"fmt"
	"gynScore-backend/internal/client"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/pkg/utils"
)

type PIXService interface {
	GerarPagamento(req models.PIXRequest) (*models.PIXResponse, error)
	ConsultarPagamento(asaasID string) (*models.Transacao, error)
	SimularPagamento(asaasID string) error
}

type pixService struct {
	asaasClient   *client.AsaasClient
	usuarioRepo   repositories.UsuarioRepository
	transacaoRepo repositories.TransacaoRepository
}

func NovoPIXService(
	asaasClient *client.AsaasClient,
	usuarioRepo repositories.UsuarioRepository,
	transacaoRepo repositories.TransacaoRepository,
) PIXService {
	return &pixService{asaasClient, usuarioRepo, transacaoRepo}
}

// ConsultarPagamento confirma o pagamento por polling ativo: consulta o status
// direto na API do Asaas (sem depender de webhook). Quando o pagamento é confirmado,
// credita o saldo do usuário uma única vez de forma idempotente.
func (s *pixService) ConsultarPagamento(asaasID string) (*models.Transacao, error) {
	transacao, err := s.transacaoRepo.BuscarPorAsaasID(asaasID)
	if err != nil {
		return nil, err
	}
	if transacao == nil {
		return nil, fmt.Errorf("transação não encontrada")
	}

	// Já confirmada anteriormente — nada a fazer
	if transacao.Status == "received" {
		return transacao, nil
	}

	// Consulta o status real no Asaas
	status, err := s.asaasClient.BuscarStatusPagamento(asaasID)
	if err != nil {
		// Não falha o polling: devolve o estado atual e tenta de novo no próximo ciclo
		return transacao, nil
	}

	if status == "RECEIVED" || status == "CONFIRMED" || status == "RECEIVED_IN_CASH" {
		// Muda pending->received atomicamente; só credita quem efetivou a mudança
		mudou, err := s.transacaoRepo.MarcarRecebidoSePendente(asaasID)
		if err != nil {
			return transacao, nil
		}
		if mudou {
			if usuario, err := s.usuarioRepo.BuscarPorID(transacao.IDUsuario); err == nil && usuario != nil {
				usuario.Saldo += transacao.Valor
				_ = s.usuarioRepo.Atualizar(usuario)
			}
		}
		transacao.Status = "received"
	}

	return transacao, nil
}

// SimularPagamento marca a cobrança como recebida no Asaas (sandbox) e processa o
// crédito imediatamente, reaproveitando a lógica idempotente de ConsultarPagamento.
func (s *pixService) SimularPagamento(asaasID string) error {
	transacao, err := s.transacaoRepo.BuscarPorAsaasID(asaasID)
	if err != nil {
		return err
	}
	if transacao == nil {
		return fmt.Errorf("transação não encontrada")
	}
	if transacao.Status == "received" {
		return nil
	}
	if err := s.asaasClient.SimularRecebimento(asaasID, transacao.Valor); err != nil {
		return err
	}
	_, err = s.ConsultarPagamento(asaasID)
	return err
}

func (s *pixService) GerarPagamento(req models.PIXRequest) (*models.PIXResponse, error) {
	if req.Valor <= 0 {
		return nil, fmt.Errorf("valor do depósito deve ser maior que zero")
	}
	if req.Valor < 5 {
		return nil, fmt.Errorf("valor mínimo para depósito PIX é R$ 5,00")
	}
	if !utils.ValidarCPF(req.CPF) {
		return nil, fmt.Errorf("CPF informado é inválido")
	}

	usuario, err := s.usuarioRepo.BuscarPorID(req.IDUsuario)
	if err != nil {
		return nil, fmt.Errorf("usuário não encontrado: %w", err)
	}
	if usuario == nil {
		return nil, fmt.Errorf("usuário não encontrado")
	}

	descricao := fmt.Sprintf("Depósito GymScore - usuário %d", usuario.ID)

	payment, err := s.asaasClient.CriarCobrancaPix(
		fmt.Sprintf("%s %s", usuario.Nome, usuario.Sobrenome),
		req.CPF,
		req.Valor,
		descricao,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar cobrança no Asaas: %w", err)
	}

	qrCode, err := s.asaasClient.BuscarPixQrCode(payment.ID)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter QR Code PIX: %w", err)
	}

	transacao := &models.Transacao{
		IDUsuario:      usuario.ID,
		AsaasPaymentID: payment.ID,
		Valor:          req.Valor,
		Status:         "pending",
	}
	if err := s.transacaoRepo.Criar(transacao); err != nil {
		return nil, fmt.Errorf("falha ao salvar transação: %w", err)
	}

	return &models.PIXResponse{
		QRCodeBase64:   qrCode.EncodedImage,
		Payload:        qrCode.Payload,
		ExpirationDate: qrCode.ExpirationDate,
		AsaasPaymentID: payment.ID,
	}, nil
}
