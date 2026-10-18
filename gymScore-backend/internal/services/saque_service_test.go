package services_test

import (
	"testing"
	"time"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
)

// ─── Mocks de Saque ──────────────────────────────────────────────────────────

type mockSaqueRepo struct {
	saques []models.Saque
}

func (m *mockSaqueRepo) Criar(s *models.Saque) error {
	s.ID = uint(len(m.saques) + 1)
	s.CriadoEm = time.Now()
	m.saques = append(m.saques, *s)
	return nil
}

func (m *mockSaqueRepo) ListarPorUsuario(idUsuario uint) ([]models.Saque, error) {
	var lista []models.Saque
	for _, s := range m.saques {
		if s.IDUsuario == idUsuario {
			lista = append(lista, s)
		}
	}
	return lista, nil
}

func (m *mockSaqueRepo) BuscarPorID(id uint) (*models.Saque, error) {
	for _, s := range m.saques {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, nil
}

type mockTransacaoRepo struct {
	transacoes []models.Transacao
}

func (m *mockTransacaoRepo) Criar(t *models.Transacao) error {
	t.ID = uint(len(m.transacoes) + 1)
	m.transacoes = append(m.transacoes, *t)
	return nil
}

func (m *mockTransacaoRepo) BuscarPorAsaasID(asaasID string) (*models.Transacao, error) {
	for _, t := range m.transacoes {
		if t.AsaasPaymentID == asaasID {
			return &t, nil
		}
	}
	return nil, nil
}

func (m *mockTransacaoRepo) AtualizarStatus(asaasID, status string) error { return nil }
func (m *mockTransacaoRepo) MarcarRecebidoSePendente(asaasID string) (bool, error) {
	return false, nil
}
func (m *mockTransacaoRepo) ListarPorUsuario(idUsuario uint) ([]models.Transacao, error) {
	return nil, nil
}

// ─── Testes ──────────────────────────────────────────────────────────────────

func TestSolicitar_SaldoInsuficiente(t *testing.T) {
	usuarioRepo := novoMockUsuarioRepo()
	usuario := &models.Usuario{Saldo: 10.00}
	_ = usuarioRepo.Criar(usuario)

	svc := services.NovoSaqueService(&mockSaqueRepo{}, usuarioRepo, &mockTransacaoRepo{})

	_, err := svc.Solicitar(usuario.ID, &models.SolicitarSaqueRequest{
		Valor:    50.00,
		ChavePix: "teste@pix.com",
	})

	if err == nil {
		t.Fatal("esperava erro de saldo insuficiente, obteve nil")
	}
}

func TestSolicitar_ValorZero(t *testing.T) {
	usuarioRepo := novoMockUsuarioRepo()
	usuario := &models.Usuario{Saldo: 100.00}
	_ = usuarioRepo.Criar(usuario)

	svc := services.NovoSaqueService(&mockSaqueRepo{}, usuarioRepo, &mockTransacaoRepo{})

	_, err := svc.Solicitar(usuario.ID, &models.SolicitarSaqueRequest{
		Valor:    0,
		ChavePix: "teste@pix.com",
	})

	if err == nil {
		t.Fatal("esperava erro de valor inválido, obteve nil")
	}
}

func TestSolicitar_Sucesso(t *testing.T) {
	usuarioRepo := novoMockUsuarioRepo()
	usuario := &models.Usuario{Saldo: 100.00}
	_ = usuarioRepo.Criar(usuario)

	saqueRepo := &mockSaqueRepo{}
	transacaoRepo := &mockTransacaoRepo{}
	svc := services.NovoSaqueService(saqueRepo, usuarioRepo, transacaoRepo)

	resp, err := svc.Solicitar(usuario.ID, &models.SolicitarSaqueRequest{
		Valor:    50.00,
		ChavePix: "teste@pix.com",
	})

	if err != nil {
		t.Fatalf("esperava sucesso, obteve erro: %v", err)
	}
	if resp.Valor != 50.00 {
		t.Errorf("esperava valor 50.00, obteve %.2f", resp.Valor)
	}
	if resp.Status != "pendente" {
		t.Errorf("esperava status 'pendente', obteve '%s'", resp.Status)
	}

	// Verificar que o saldo foi debitado
	usuarioAtualizado, _ := usuarioRepo.BuscarPorID(usuario.ID)
	if usuarioAtualizado.Saldo != 50.00 {
		t.Errorf("esperava saldo 50.00 após saque, obteve %.2f", usuarioAtualizado.Saldo)
	}

	// Verificar que uma transação de saída foi registrada
	if len(transacaoRepo.transacoes) == 0 {
		t.Error("esperava ao menos uma transação registrada")
	} else if transacaoRepo.transacoes[0].Tipo != "saida" {
		t.Errorf("esperava Tipo='saida', obteve '%s'", transacaoRepo.transacoes[0].Tipo)
	}
}
