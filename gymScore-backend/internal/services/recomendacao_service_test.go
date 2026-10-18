package services_test

import (
	"testing"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
)

// ─── Mocks para Recomendação ─────────────────────────────────────────────────
// mockDesafioRepo implementa apenas o subconjunto de métodos exigido pela
// recomendação (interface desafioLister, não exportada em services).

type mockDesafioRepo struct {
	desafios []models.Desafio
}

func (m *mockDesafioRepo) Listar(f repositories.FiltroDesafios) ([]models.Desafio, int64, error) {
	var filtrados []models.Desafio
	for _, d := range m.desafios {
		if f.Status != "" && d.Status != f.Status {
			continue
		}
		filtrados = append(filtrados, d)
	}
	return filtrados, int64(len(filtrados)), nil
}
func (m *mockDesafioRepo) ListarHistoricoUsuario(id uint) ([]models.Desafio, error) {
	return nil, nil
}

type mockTreinoRepo struct{}

func (m *mockTreinoRepo) Criar(t *models.Treino) error                          { return nil }
func (m *mockTreinoRepo) BuscarPorID(id uint) (*models.Treino, error)           { return nil, nil }
func (m *mockTreinoRepo) ListarPorUsuario(id uint) ([]models.Treino, error)     { return nil, nil }
func (m *mockTreinoRepo) Atualizar(t *models.Treino) error                      { return nil }
func (m *mockTreinoRepo) Deletar(id uint) error                                 { return nil }
func (m *mockTreinoRepo) BuscarPorCodigo(codigo string) (*models.Treino, error) { return nil, nil }
func (m *mockTreinoRepo) CodigoExiste(codigo string) (bool, error)              { return false, nil }
func (m *mockTreinoRepo) RegistrarConclusao(c *models.TreinoConclusao) error    { return nil }
func (m *mockTreinoRepo) RadarPorUsuario(id uint) ([]models.RadarGrupo, error)  { return nil, nil }

// ─── Testes ──────────────────────────────────────────────────────────────────

func TestRecomendar_ListaVaziaQuandoNaoHaDesafiosAbertos(t *testing.T) {
	desafioRepo := &mockDesafioRepo{desafios: []models.Desafio{}}
	usuarioRepo := novoMockUsuarioRepo()
	treinoRepo := &mockTreinoRepo{}

	svc := services.NovoRecomendacaoService(desafioRepo, treinoRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(1)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("esperava lista vazia, obteve %d itens", len(resp))
	}
}

func TestRecomendar_ExcluiDesafiosDoProprioUsuario(t *testing.T) {
	const userID uint = 42
	desafioRepo := &mockDesafioRepo{
		desafios: []models.Desafio{
			{ID: 1, IDCriador: userID, Status: models.StatusAberto, Titulo: "Meu desafio", ValorEntradaCentavos: 5000},
			{ID: 2, IDCriador: 99, Status: models.StatusAberto, Titulo: "Desafio alheio", ValorEntradaCentavos: 5000},
		},
	}
	usuarioRepo := novoMockUsuarioRepo()
	treinoRepo := &mockTreinoRepo{}

	svc := services.NovoRecomendacaoService(desafioRepo, treinoRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(userID)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	for _, r := range resp {
		if r.Desafio.IDCriador == userID {
			t.Error("recomendação não deve incluir desafios criados pelo próprio usuário")
		}
	}
}

func TestRecomendar_RazaoNaoVazia(t *testing.T) {
	desafioRepo := &mockDesafioRepo{
		desafios: []models.Desafio{
			{ID: 10, IDCriador: 99, Status: models.StatusAberto, Titulo: "Corrida 5km", ValorEntradaCentavos: 10000},
		},
	}
	usuarioRepo := novoMockUsuarioRepo()
	treinoRepo := &mockTreinoRepo{}

	svc := services.NovoRecomendacaoService(desafioRepo, treinoRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(1)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	for _, r := range resp {
		if r.Razao == "" {
			t.Error("campo 'razao' não deve ser vazio em nenhuma recomendação")
		}
	}
}

func TestRecomendar_LimitadoACinco(t *testing.T) {
	var desafios []models.Desafio
	for i := uint(1); i <= 10; i++ {
		desafios = append(desafios, models.Desafio{
			ID: i, IDCriador: 99, Status: models.StatusAberto,
			Titulo: "Desafio", ValorEntradaCentavos: 5000,
		})
	}
	desafioRepo := &mockDesafioRepo{desafios: desafios}
	usuarioRepo := novoMockUsuarioRepo()
	treinoRepo := &mockTreinoRepo{}

	svc := services.NovoRecomendacaoService(desafioRepo, treinoRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(1)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if len(resp) > 5 {
		t.Errorf("esperava no máximo 5 recomendações, obteve %d", len(resp))
	}
}
