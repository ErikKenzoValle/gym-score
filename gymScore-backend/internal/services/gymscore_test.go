package services_test

import (
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
)

func TestNivelGymScore_PontuacaoEFaixas(t *testing.T) {
	c := repositories.ComponentesGymScore{Primeiros: 1, Segundos: 1, Terceiros: 1, OutrosPremiados: 2, Presencas: 5, DiasTreino: 7}
	if got, want := services.PontuacaoGymScore(c), 50+35+25+30+50+7; got != want {
		t.Fatalf("pontuação %d, esperava %d", got, want)
	}
	casos := []struct{ pontos, nivel int }{
		{0, 1}, {19, 1}, {20, 2}, {49, 2}, {50, 3}, {99, 3}, {100, 4}, {174, 4}, {175, 5},
		{274, 5}, {275, 6}, {399, 6}, {400, 7}, {574, 7}, {575, 8}, {799, 8}, {800, 9}, {1099, 9}, {1100, 10}, {99999, 10},
	}
	for _, cs := range casos {
		if got := services.NivelPorPontuacao(cs.pontos); got != cs.nivel {
			t.Errorf("%d pontos: nível %d, esperava %d", cs.pontos, got, cs.nivel)
		}
	}
	inicio, prox := services.FaixaNivel(3)
	if inicio != 50 || prox == nil || *prox != 100 {
		t.Errorf("faixa do nível 3: %d..%v", inicio, prox)
	}
	if _, prox := services.FaixaNivel(10); prox != nil {
		t.Error("nível 10 não tem próximo")
	}
}

func TestCalcularEixos_SemDadosZeraTudo(t *testing.T) {
	atr, cat, dados := services.CalcularEixos(nil, nil, time.Now())
	if len(atr) != 6 || len(cat) != 8 {
		t.Fatalf("eixos: %d atributos, %d categorias", len(atr), len(cat))
	}
	for k, v := range atr {
		if v != 0 {
			t.Errorf("atributo %s = %d sem dados", k, v)
		}
	}
	for k, v := range cat {
		if v != 0 {
			t.Errorf("categoria %s = %d sem dados", k, v)
		}
	}
	if dados.Exercicios != 0 || dados.Desafios != 0 || dados.JanelaDias != 180 {
		t.Errorf("dados: %+v", dados)
	}
}

func TestCalcularEixos_PesosRecenciaETeto(t *testing.T) {
	agora := time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)
	um := 1
	// Corredora: muitas corridas recentes e treino de pernas; nenhuma musculação.
	var treinos []repositories.TreinoNaJanela
	for i := 0; i < 60; i++ {
		treinos = append(treinos, repositories.TreinoNaJanela{GrupoMuscular: "pernas", Dia: agora.AddDate(0, 0, -i*2), N: 3})
	}
	var desafios []repositories.DesafioNaJanela
	for i := 0; i < 20; i++ {
		desafios = append(desafios, repositories.DesafioNaJanela{IDDesafio: uint(i + 1), Categoria: "corrida", Quando: agora.AddDate(0, 0, -i*5), Posicao: &um})
	}
	atr, cat, dados := services.CalcularEixos(treinos, desafios, agora)
	for k, v := range atr {
		if v < 0 || v > 100 {
			t.Errorf("atributo %s fora de 0..100: %d", k, v)
		}
	}
	if cat["corrida"] < 95 || cat["musculacao"] != 0 || cat["luta"] != 0 {
		t.Errorf("categorias: %v", cat)
	}
	if atr["resistencia"] <= atr["mobilidade"] || atr["velocidade"] <= atr["mobilidade"] {
		t.Errorf("corredora deveria ter resistência/velocidade acima de mobilidade: %v", atr)
	}
	if dados.Desafios != 20 || dados.Podios != 20 || dados.Exercicios != 180 || dados.DiasTreino != 60 {
		t.Errorf("dados: %+v", dados)
	}
	// Recente pesa mais: o mesmo treino há 170 dias vale menos que hoje.
	velho, _, _ := services.CalcularEixos([]repositories.TreinoNaJanela{{GrupoMuscular: "peito", Dia: agora.AddDate(0, 0, -170), N: 10}}, nil, agora)
	novo, _, _ := services.CalcularEixos([]repositories.TreinoNaJanela{{GrupoMuscular: "peito", Dia: agora, N: 10}}, nil, agora)
	if velho["forca"] >= novo["forca"] || novo["forca"] == 0 {
		t.Errorf("recência: velho %d, novo %d", velho["forca"], novo["forca"])
	}
}

func ptr(v int) *int { return &v }

func TestMotivoFiltroEntrada_NivelEIdade(t *testing.T) {
	evento := time.Date(2026, 12, 1, 9, 0, 0, 0, time.Local)
	nasc := time.Date(2008, 12, 2, 0, 0, 0, 0, time.Local) // faz 18 no dia seguinte ao evento
	d := &models.Desafio{DataEvento: &evento, NivelMin: ptr(3), NivelMax: ptr(6)}
	if m := services.MotivoFiltroEntrada(d, services.PerfilElegibilidade{Nivel: 2}, evento); m != "Este desafio é para nível 3 a 6; você está no nível 2" {
		t.Errorf("nível baixo: %q", m)
	}
	if m := services.MotivoFiltroEntrada(d, services.PerfilElegibilidade{Nivel: 7}, evento); !strings.Contains(m, "nível 7") {
		t.Errorf("nível alto: %q", m)
	}
	if m := services.MotivoFiltroEntrada(d, services.PerfilElegibilidade{Nivel: 4}, evento); m != "" {
		t.Errorf("nível ok: %q", m)
	}
	// Nível 0 (desconhecido) conta como 1.
	if m := services.MotivoFiltroEntrada(&models.Desafio{NivelMin: ptr(1)}, services.PerfilElegibilidade{}, evento); m != "" {
		t.Errorf("nível 0 = 1: %q", m)
	}
	idade := &models.Desafio{DataEvento: &evento, IdadeMin: ptr(18)}
	if m := services.MotivoFiltroEntrada(idade, services.PerfilElegibilidade{Nivel: 1, DataNascimento: &nasc}, evento); !strings.Contains(m, "idade mínima de 18 anos") || !strings.Contains(m, "17 anos") {
		t.Errorf("idade na data do evento: %q", m)
	}
	if m := services.MotivoFiltroEntrada(idade, services.PerfilElegibilidade{Nivel: 1}, evento); m != services.MsgSemDataNascimento {
		t.Errorf("sem data de nascimento: %q", m)
	}
	nasc2 := time.Date(2008, 12, 1, 0, 0, 0, 0, time.Local) // 18 exatamente no dia do evento
	if m := services.MotivoFiltroEntrada(idade, services.PerfilElegibilidade{Nivel: 1, DataNascimento: &nasc2}, evento); m != "" {
		t.Errorf("aniversário no dia do evento: %q", m)
	}
	max := &models.Desafio{DataEvento: &evento, IdadeMax: ptr(40)}
	velho := time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local)
	if m := services.MotivoFiltroEntrada(max, services.PerfilElegibilidade{Nivel: 1, DataNascimento: &velho}, evento); !strings.Contains(m, "idade máxima de 40 anos") {
		t.Errorf("idade máxima: %q", m)
	}
	// Sem filtro de idade, data de nascimento ausente não bloqueia.
	if m := services.MotivoFiltroEntrada(&models.Desafio{}, services.PerfilElegibilidade{}, evento); m != "" {
		t.Errorf("sem filtros: %q", m)
	}
}
