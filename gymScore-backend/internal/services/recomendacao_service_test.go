package services_test

import (
	"testing"
	"time"

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

// ─── Testes ──────────────────────────────────────────────────────────────────

func TestRecomendar_ListaVaziaQuandoNaoHaDesafiosAbertos(t *testing.T) {
	desafioRepo := &mockDesafioRepo{desafios: []models.Desafio{}}
	usuarioRepo := novoMockUsuarioRepo()

	svc := services.NovoRecomendacaoService(desafioRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(1)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("esperava lista vazia, obteve %d itens", len(resp))
	}
}

func TestRecomendar_ExcluiDesafiosEmQueOUsuarioJaParticipaOuLotados(t *testing.T) {
	const userID uint = 42
	desafioRepo := &mockDesafioRepo{
		desafios: []models.Desafio{
			{ID: 1, IDCriador: 99, Status: models.StatusAberto, Titulo: "Já inscrito", ValorEntradaCentavos: 5000, Vagas: 10,
				MinhaParticipacao: &models.DesafioParticipante{IDDesafio: 1, IDUsuario: userID}},
			{ID: 2, IDCriador: 99, Status: models.StatusAberto, Titulo: "Desafio livre", ValorEntradaCentavos: 5000, Vagas: 10},
			{ID: 3, IDCriador: 99, Status: models.StatusAberto, Titulo: "Lotado", ValorEntradaCentavos: 5000, Vagas: 2, QtdParticipantes: 2},
		},
	}
	usuarioRepo := novoMockUsuarioRepo()

	svc := services.NovoRecomendacaoService(desafioRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(userID)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if len(resp) != 1 || resp[0].Desafio.ID != 2 {
		t.Errorf("esperava só o desafio 2 (livre), obteve %+v", resp)
	}
}

func TestRecomendar_RazaoNaoVazia(t *testing.T) {
	desafioRepo := &mockDesafioRepo{
		desafios: []models.Desafio{
			{ID: 10, IDCriador: 99, Status: models.StatusAberto, Titulo: "Corrida 5km", ValorEntradaCentavos: 10000, Vagas: 10},
		},
	}
	usuarioRepo := novoMockUsuarioRepo()

	svc := services.NovoRecomendacaoService(desafioRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(1)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("esperava 1 recomendação, obteve %d", len(resp))
	}
	for _, r := range resp {
		if r.Razao == "" || r.RazaoFonte != services.RazaoFonteRegras || r.Base == nil {
			t.Error("campo 'razao' não deve ser vazio em nenhuma recomendação")
		}
	}
}

func TestRecomendar_LimitadoACinco(t *testing.T) {
	var desafios []models.Desafio
	for i := uint(1); i <= 10; i++ {
		desafios = append(desafios, models.Desafio{
			ID: i, IDCriador: 99, Status: models.StatusAberto,
			Titulo: "Desafio", ValorEntradaCentavos: 5000, Vagas: 10,
		})
	}
	desafioRepo := &mockDesafioRepo{desafios: desafios}
	usuarioRepo := novoMockUsuarioRepo()

	svc := services.NovoRecomendacaoService(desafioRepo, usuarioRepo, nil)

	resp, err := svc.Recomendar(1)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if len(resp) != 5 {
		t.Errorf("esperava no máximo 5 recomendações, obteve %d", len(resp))
	}
}

// U4 (rodada 6): a recomendação nunca sugere desafio cujos requisitos de nível/idade o atleta não cumpre.
func TestRecomendar_NaoSugereDesafioInelegivel(t *testing.T) {
	usuarioRepo := novoMockUsuarioRepo()
	nasc := time.Now().AddDate(-16, 0, 0) // 16 anos
	u := &models.Usuario{Nome: "A", Perfil: models.PerfilAtleta, DataNascimento: &nasc}
	_ = usuarioRepo.Criar(u)
	nivel := func(v int) *int { return &v }
	desafioRepo := &mockDesafioRepo{desafios: []models.Desafio{
		{ID: 1, Status: models.StatusAberto, Titulo: "Nível 3+", Vagas: 10, NivelMin: nivel(3)},
		{ID: 2, Status: models.StatusAberto, Titulo: "Maiores de 18", Vagas: 10, IdadeMin: nivel(18)},
		{ID: 3, Status: models.StatusAberto, Titulo: "Livre", Vagas: 10},
		{ID: 4, Status: models.StatusAberto, Titulo: "Nível 1 a 2 e até 20 anos", Vagas: 10, NivelMax: nivel(2), IdadeMax: nivel(20)},
	}}
	resp, err := services.NovoRecomendacaoService(desafioRepo, usuarioRepo, nil).Recomendar(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[uint]bool{}
	for _, r := range resp {
		ids[r.Desafio.ID] = true
		if !r.Desafio.PodeEntrar || r.Desafio.MotivoBloqueioEntrada != "" {
			t.Errorf("desafio %d recomendado sem poder entrar: %+v", r.Desafio.ID, r.Desafio.MotivoBloqueioEntrada)
		}
	}
	if len(ids) != 2 || !ids[3] || !ids[4] {
		t.Errorf("esperava só os desafios 3 e 4, obteve %v", ids)
	}
}
