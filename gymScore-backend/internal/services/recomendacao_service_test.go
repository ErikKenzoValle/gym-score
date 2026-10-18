package services_test

import (
	"testing"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
)

// ─── Mocks para Recomendação ─────────────────────────────────────────────────

type mockDesafioRepo struct {
	desafios []models.Desafio
}

func (m *mockDesafioRepo) Criar(d *models.Desafio) error                          { return nil }
func (m *mockDesafioRepo) BuscarPorID(id uint) (*models.Desafio, error)            { return nil, nil }
func (m *mockDesafioRepo) Atualizar(d *models.Desafio) error                      { return nil }
func (m *mockDesafioRepo) Deletar(id uint) error                                  { return nil }
func (m *mockDesafioRepo) AdicionarParticipante(p *models.DesafioParticipante) error { return nil }
func (m *mockDesafioRepo) ContarParticipantes(id uint) (int64, error)             { return 0, nil }
func (m *mockDesafioRepo) UsuarioParticipa(idDesafio, idUsuario uint) (bool, error) { return false, nil }
func (m *mockDesafioRepo) ListarParticipantes(id uint) ([]models.DesafioParticipante, error) {
	return nil, nil
}
func (m *mockDesafioRepo) ListarPorUsuario(id uint) ([]models.Desafio, error) { return nil, nil }
func (m *mockDesafioRepo) Listar() ([]models.Desafio, error)                  { return m.desafios, nil }
func (m *mockDesafioRepo) ListarHistoricoUsuario(id uint) ([]models.Desafio, error) {
	return nil, nil
}
func (m *mockDesafioRepo) MarcarPresente(idDesafio, idUsuario uint) error { return nil }
func (m *mockDesafioRepo) ListarPresentes(id uint) ([]models.DesafioParticipante, error) {
	return nil, nil
}
func (m *mockDesafioRepo) ListarParticipacoesUsuario(idUsuario uint) ([]models.Desafio, error) {
	return nil, nil
}

type mockTreinoRepo struct{}

func (m *mockTreinoRepo) Criar(t *models.Treino) error                              { return nil }
func (m *mockTreinoRepo) BuscarPorID(id uint) (*models.Treino, error)               { return nil, nil }
func (m *mockTreinoRepo) ListarPorUsuario(id uint) ([]models.Treino, error)         { return nil, nil }
func (m *mockTreinoRepo) Atualizar(t *models.Treino) error                          { return nil }
func (m *mockTreinoRepo) Deletar(id uint) error                                     { return nil }
func (m *mockTreinoRepo) BuscarPorCodigo(codigo string) (*models.Treino, error)     { return nil, nil }
func (m *mockTreinoRepo) CodigoExiste(codigo string) (bool, error)                  { return false, nil }
func (m *mockTreinoRepo) RegistrarConclusao(c *models.TreinoConclusao) error        { return nil }
func (m *mockTreinoRepo) RadarPorUsuario(id uint) ([]models.RadarGrupo, error)      { return nil, nil }

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
			{ID: 1, IDCriador: userID, Status: models.StatusAberto, Titulo: "Meu desafio", Valor: 50},
			{ID: 2, IDCriador: 99, Status: models.StatusAberto, Titulo: "Desafio alheio", Valor: 50},
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
			{ID: 10, IDCriador: 99, Status: models.StatusAberto, Titulo: "Corrida 5km", Valor: 100},
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
			Titulo: "Desafio", Valor: 50,
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
