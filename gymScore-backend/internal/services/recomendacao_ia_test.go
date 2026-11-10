package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/client"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"

	"gorm.io/gorm"
)

// ─── Mocks (A6-1) ────────────────────────────────────────────────────────────

// mockIA simula o cliente Anthropic: nunca chama a API real.
type mockIA struct {
	mu         sync.Mutex
	chamadas   int
	perfil     client.PerfilAtletaIA
	candidatos []client.CandidatoIA
	fn         func(ctx context.Context, c []client.CandidatoIA) ([]client.RazaoIA, error)
}

func (m *mockIA) GerarRazoesRecomendacao(ctx context.Context, p client.PerfilAtletaIA, c []client.CandidatoIA) ([]client.RazaoIA, error) {
	m.mu.Lock()
	m.chamadas++
	m.perfil, m.candidatos = p, c
	m.mu.Unlock()
	return m.fn(ctx, c)
}

// mockGymScore devolve estatísticas fixas.
type mockGymScore struct {
	e   *services.EstatisticasAtleta
	err error
}

func (m *mockGymScore) Estatisticas(uint) (*models.EstatisticasResponse, error) { return nil, nil }
func (m *mockGymScore) EstatisticasDoAtleta(uint) (*services.EstatisticasAtleta, error) {
	return m.e, m.err
}
func (m *mockGymScore) ComRecalculo(uint, func(tx *gorm.DB) error) error  { return nil }
func (m *mockGymScore) ReconciliarNiveis(int, time.Duration) (int, error) { return 0, nil }

// cenarioRecomendacao: atleta forte em corrida/resistência, nível 4; quatro desafios de categorias diferentes.
func cenarioRecomendacao(t *testing.T) (*mockUsuarioRepo, *mockDesafioRepo, *mockGymScore, uint) {
	t.Helper()
	usuarios := novoMockUsuarioRepo()
	nasc := time.Date(1990, 5, 17, 0, 0, 0, 0, time.Local)
	email := "atleta.secreta@example.com"
	u := &models.Usuario{Nome: "Fulana Secreta da Silva", Email: email, Perfil: models.PerfilAtleta, DataNascimento: &nasc}
	_ = usuarios.Criar(u)
	resumo := "Prova de 10 km em percurso plano com hidratação e chegada no parque"
	desafios := &mockDesafioRepo{desafios: []models.Desafio{
		{ID: 1, Status: models.StatusAberto, Titulo: "Supino máximo", Categoria: "musculacao", Vagas: 10, ValorEntradaCentavos: 2000},
		{ID: 2, Status: models.StatusAberto, Titulo: "Corrida 10 km", Categoria: "corrida", Vagas: 10, ValorEntradaCentavos: 2000, Resumo: &resumo},
		{ID: 3, Status: models.StatusAberto, Titulo: "Natação 1 km", Categoria: "natacao", Vagas: 10, ValorEntradaCentavos: 2000},
		{ID: 4, Status: models.StatusAberto, Titulo: "Desafio geral", Categoria: "geral", Vagas: 10, ValorEntradaCentavos: 2000},
	}}
	gs := &mockGymScore{e: &services.EstatisticasAtleta{
		IDUsuario: u.ID, Nivel: 4, Pontuacao: 120,
		Atributos: map[string]int{"forca": 10, "resistencia": 70, "velocidade": 60, "agilidade": 15, "mobilidade": 12, "constancia": 50},
		Categorias: map[string]int{"corrida": 65, "musculacao": 0, "crossfit": 0, "funcional": 0, "ciclismo": 20,
			"natacao": 0, "calistenia": 0, "luta": 0},
		Dados: models.DadosJanela{JanelaDias: 180, Exercicios: 40, Desafios: 3}, DataNascimento: &nasc,
	}}
	return usuarios, desafios, gs, u.ID
}

func idsDe(resp []models.DesafioRecomendado) []uint {
	out := make([]uint, len(resp))
	for i, r := range resp {
		out[i] = r.Desafio.ID
	}
	return out
}

// ─── Score determinístico ────────────────────────────────────────────────────

func TestRecomendar_ScoreUsaNivelAtributosECategorias(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, nil, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 4 {
		t.Fatalf("esperava 4 recomendações, obteve %v", idsDe(resp))
	}
	if resp[0].Desafio.ID != 2 {
		t.Errorf("corrida (ponto forte do atleta) deveria vir primeiro, ordem %v", idsDe(resp))
	}
	if resp[len(resp)-1].Desafio.ID != 1 {
		t.Errorf("musculação (força 10, categoria 0) deveria vir por último, ordem %v", idsDe(resp))
	}
	b := resp[0].Base
	if b == nil || b.Nivel != 4 || b.Categoria != "corrida" || b.ValorCategoria == nil || *b.ValorCategoria != 65 || len(b.Atributos) == 0 {
		t.Fatalf("base da recomendação incompleta: %+v", b)
	}
	if b.Atributos[0].Chave != "resistencia" || b.Atributos[0].Valor != 70 || b.Atributos[0].PesoPct != 50 {
		t.Errorf("atributo mais relevante deveria ser resistência 70 (peso 50%%): %+v", b.Atributos)
	}
	for _, r := range resp {
		if r.RazaoFonte != services.RazaoFonteRegras || r.Razao == "" {
			t.Errorf("sem IA a razão deve ser determinística e não vazia: %+v", r)
		}
		if r.Base.Score <= 0 {
			t.Errorf("score não calculado: %+v", r.Base)
		}
	}
	if !strings.Contains(resp[0].Razao, "Corrida") || !strings.Contains(resp[0].Razao, "65/100") {
		t.Errorf("razão determinística deveria citar o eixo: %q", resp[0].Razao)
	}
	// geral não tem eixo próprio: valor_categoria nulo
	for _, r := range resp {
		if r.Desafio.ID == 4 && r.Base.ValorCategoria != nil {
			t.Errorf("categoria geral não tem eixo: %+v", r.Base)
		}
	}
}

func TestRecomendar_FaixaDeNivelPontuaENaoSugereForaDaFaixa(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	n := func(v int) *int { return &v }
	desafios.desafios = []models.Desafio{
		{ID: 10, Status: models.StatusAberto, Titulo: "Livre", Categoria: "musculacao", Vagas: 10},
		{ID: 11, Status: models.StatusAberto, Titulo: "Nível 3 a 6", Categoria: "musculacao", Vagas: 10, NivelMin: n(3), NivelMax: n(6)},
		{ID: 12, Status: models.StatusAberto, Titulo: "Nível 5+", Categoria: "corrida", Vagas: 10, NivelMin: n(5)},
	}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, nil, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil {
		t.Fatal(err)
	}
	ids := idsDe(resp)
	if len(ids) != 2 || ids[0] != 11 || ids[1] != 10 {
		t.Fatalf("esperava [11 10] (12 exige nível 5, atleta no 4 vindo das estatísticas), obteve %v", ids)
	}
	if !strings.Contains(resp[0].Razao, "nível 3 a 6") || !strings.Contains(resp[0].Razao, "nível 4") {
		t.Errorf("razão deveria citar a faixa e o nível: %q", resp[0].Razao)
	}
}

func TestRecomendar_EstatisticasComErroNaoQuebra(t *testing.T) {
	usuarios, desafios, _, id := cenarioRecomendacao(t)
	gs := &mockGymScore{err: errors.New("banco fora")}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, nil, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil || len(resp) != 4 {
		t.Fatalf("estatísticas indisponíveis não podem quebrar a recomendação: %v %d", err, len(resp))
	}
	for _, r := range resp {
		if r.Razao == "" || r.Base == nil || !r.Base.SemDados {
			t.Errorf("sem estatísticas: razão determinística e base sem dados: %+v", r)
		}
	}
}

// ─── IA (cliente mockado) ────────────────────────────────────────────────────

func TestRecomendar_IASucessoUsaFraseDoModeloSemPII(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	ia := &mockIA{fn: func(_ context.Context, c []client.CandidatoIA) ([]client.RazaoIA, error) {
		out := []client.RazaoIA{}
		for _, x := range c {
			out = append(out, client.RazaoIA{ID: x.ID, PorQue: "  Sua Resistência em 70/100\ncombina com este desafio.  "})
		}
		return out, nil
	}}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, ia, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil {
		t.Fatal(err)
	}
	if ia.chamadas != 1 {
		t.Fatalf("esperava 1 chamada à IA, obteve %d", ia.chamadas)
	}
	for _, r := range resp {
		if r.RazaoFonte != services.RazaoFonteIA || r.Razao != "Sua Resistência em 70/100 combina com este desafio." {
			t.Errorf("razão da IA não aplicada/normalizada: %+v", r)
		}
	}
	// o que vai ao modelo: nível + eixos; candidatos = exatamente os recomendados, na mesma ordem
	if ia.perfil.Nivel != 4 || ia.perfil.Atributos["resistencia"] != 70 || ia.perfil.Categorias["corrida"] != 65 {
		t.Errorf("perfil enviado ao modelo sem nível/eixos: %+v", ia.perfil)
	}
	if len(ia.candidatos) != len(resp) {
		t.Fatalf("candidatos enviados %d != recomendados %d", len(ia.candidatos), len(resp))
	}
	for i := range resp {
		if ia.candidatos[i].ID != resp[i].Desafio.ID || len(ia.candidatos[i].Sinais) == 0 {
			t.Errorf("candidato %d enviado fora de ordem ou sem sinais: %+v", i, ia.candidatos[i])
		}
	}
	payload, _ := json.Marshal(map[string]any{"perfil": ia.perfil, "candidatos": ia.candidatos})
	for _, pii := range []string{"Fulana", "Secreta", "atleta.secreta", "example.com", "1990", "nascimento", `"idade`, `"email`, `"nome`, " anos"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(pii)) {
			t.Errorf("payload enviado ao modelo contém PII/termo proibido %q: %s", pii, payload)
		}
	}
}

func TestRecomendar_IATimeoutCaiNaRazaoDeterministica(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	ia := &mockIA{fn: func(ctx context.Context, _ []client.CandidatoIA) ([]client.RazaoIA, error) {
		<-ctx.Done() // simula a API que não responde a tempo
		return nil, ctx.Err()
	}}
	inicio := time.Now()
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, ia, services.ComEstatisticas(gs),
		services.ComTimeoutIA(50*time.Millisecond)).Recomendar(id)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(inicio); d > 2*time.Second {
		t.Errorf("timeout da IA não respeitado: %v", d)
	}
	for _, r := range resp {
		if r.RazaoFonte != services.RazaoFonteRegras || r.Razao == "" {
			t.Errorf("timeout deveria cair na razão determinística: %+v", r)
		}
	}
}

func TestRecomendar_IAJSONInvalidoCaiNaRazaoDeterministica(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	ia := &mockIA{fn: func(context.Context, []client.CandidatoIA) ([]client.RazaoIA, error) {
		return nil, client.ErrRespostaFormato
	}}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, ia, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil || len(resp) != 4 {
		t.Fatalf("JSON inválido não pode quebrar: %v %d", err, len(resp))
	}
	for _, r := range resp {
		if r.RazaoFonte != services.RazaoFonteRegras || r.Razao == "" {
			t.Errorf("JSON inválido deveria cair na razão determinística: %+v", r)
		}
	}
}

func TestRecomendar_IAIDInventadoEDuplicadoSaoDescartados(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	ia := &mockIA{fn: func(context.Context, []client.CandidatoIA) ([]client.RazaoIA, error) {
		return []client.RazaoIA{
			{ID: 999, PorQue: "Desafio que nem existe, mas parece ótimo para você."},
			{ID: 2, PorQue: "Sua Corrida em 65/100 faz deste desafio uma boa aposta."},
			{ID: 3, PorQue: "Primeira versão da frase para a natação, nível 4."},
			{ID: 3, PorQue: "Segunda versão da frase para a natação, nível 4."},
		}, nil
	}}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, ia, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 4 {
		t.Fatalf("ID inventado não pode adicionar recomendação: %v", idsDe(resp))
	}
	for _, r := range resp {
		switch r.Desafio.ID {
		case 2:
			if r.RazaoFonte != services.RazaoFonteIA {
				t.Errorf("id válido deveria usar a frase da IA: %+v", r)
			}
		default:
			if r.RazaoFonte != services.RazaoFonteRegras {
				t.Errorf("desafio %d (sem frase válida / duplicada) deveria usar a determinística: %+v", r.Desafio.ID, r)
			}
		}
	}
}

func TestRecomendar_IATextoComMarcacaoOuLinkEDescartado(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	ruins := map[uint]string{
		1: `<img src=x onerror=alert(1)> combina com você`,
		2: "Veja mais em https://exemplo.com sobre este desafio de corrida",
		3: "curta",
		4: strings.Repeat("palavra ", 40),
	}
	ia := &mockIA{fn: func(_ context.Context, c []client.CandidatoIA) ([]client.RazaoIA, error) {
		out := []client.RazaoIA{}
		for _, x := range c {
			out = append(out, client.RazaoIA{ID: x.ID, PorQue: ruins[x.ID]})
		}
		return out, nil
	}}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, ia, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp {
		if r.RazaoFonte != services.RazaoFonteRegras || strings.ContainsAny(r.Razao, "<>") || strings.Contains(r.Razao, "http") {
			t.Errorf("texto inválido do modelo não pode chegar ao cliente: %+v", r)
		}
	}
}

func TestRecomendar_SemChaveUsaRegrasSemPanico(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	semChave := client.NovoAnthropicClient("", "") // nil: IA desabilitada
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, semChave, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil || len(resp) != 4 {
		t.Fatalf("sem chave deveria funcionar só com regras: %v %d", err, len(resp))
	}
	for _, r := range resp {
		if r.RazaoFonte != services.RazaoFonteRegras || r.Razao == "" {
			t.Errorf("sem chave: razão determinística: %+v", r)
		}
	}
}

func TestRecomendar_SemCandidatosNaoChamaIA(t *testing.T) {
	usuarios, desafios, gs, id := cenarioRecomendacao(t)
	desafios.desafios = nil
	ia := &mockIA{fn: func(context.Context, []client.CandidatoIA) ([]client.RazaoIA, error) { return nil, nil }}
	resp, err := services.NovoRecomendacaoService(desafios, usuarios, ia, services.ComEstatisticas(gs)).Recomendar(id)
	if err != nil || len(resp) != 0 || ia.chamadas != 0 {
		t.Fatalf("sem candidatos: lista vazia e nenhuma chamada (obteve %d itens, %d chamadas, %v)", len(resp), ia.chamadas, err)
	}
}
