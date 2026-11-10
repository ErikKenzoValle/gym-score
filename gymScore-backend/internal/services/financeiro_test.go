package services_test

import (
	"reflect"
	"strings"
	"testing"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
)

// fecha confere a invariante da conta: arrecadação + reserva = prêmios + instrutor + academia + taxa.
func fecha(t *testing.T, p services.ParamsFinanceiros, r services.ResultadoFinanceiro) {
	t.Helper()
	if esq, dir := r.Arrecadacao+p.ReservaCentavos, r.PremioTotal+r.InstrutorTotal+r.Academia+r.Taxa; esq != dir {
		t.Errorf("conta não fecha: arrecadação+reserva=%d, prêmios+instrutor+academia+taxa=%d (%+v)", esq, dir, r)
	}
}

func TestCalcularFinanceiro_Cenarios(t *testing.T) {
	casos := []struct {
		nome string
		p    services.ParamsFinanceiros
		want services.ResultadoFinanceiro
	}{
		{
			// 10 inscritos x R$ 10,00; prêmio fixo R$ 50; pote 50%; taxa 10%; instrutor R$ 20 + 5%.
			// reserva = 5000 + 2000 = 7000.
			nome: "completo: fixo + pote + taxa + instrutor fixo_percentual",
			p: services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 10, PremioFixoCentavos: 5000, DividirPote: true,
				PercentualPote: 50, TaxaPct: 10, InstrutorModo: models.InstrutorFixoPercentual, InstrutorFixo: 2000, InstrutorPct: 5,
				ReservaCentavos: 7000},
			want: services.ResultadoFinanceiro{Arrecadacao: 10000, PotePremio: 5000, PremioTotal: 10000, Taxa: 1000,
				InstrutorFixo: 2000, InstrutorPct: 500, InstrutorTotal: 2500, Academia: 3500},
		},
		{
			// 3 x R$ 3,33 = 999. pote 33% = 329 (329,67), taxa 10% = 99 (99,9), instrutor 7% = 69 (69,93).
			// A academia fica com 999 - 329 - 99 - 69 = 502: os centavos que faltam em cada conta voltam para ela.
			nome: "arredondamento: o resto vai para a academia",
			p: services.ParamsFinanceiros{EntradaCentavos: 333, Inscritos: 3, DividirPote: true, PercentualPote: 33,
				TaxaPct: 10, InstrutorModo: models.InstrutorPercentual, InstrutorPct: 7},
			want: services.ResultadoFinanceiro{Arrecadacao: 999, PotePremio: 329, PremioTotal: 329, Taxa: 99,
				InstrutorPct: 69, InstrutorTotal: 69, Academia: 502},
		},
		{
			nome: "sem dividir o pote: só o prêmio fixo vai para os atletas",
			p: services.ParamsFinanceiros{EntradaCentavos: 2000, Inscritos: 5, PremioFixoCentavos: 3000, DividirPote: false,
				PercentualPote: 0, TaxaPct: 10, InstrutorModo: models.InstrutorFixo, InstrutorFixo: 1500, ReservaCentavos: 4500},
			// T=10000; taxa 1000; academia = 10000 + 4500 - 3000 - 1500 - 1000 = 9000
			want: services.ResultadoFinanceiro{Arrecadacao: 10000, PremioTotal: 3000, Taxa: 1000,
				InstrutorFixo: 1500, InstrutorTotal: 1500, Academia: 9000},
		},
		{
			nome: "comportamento legado (pote 100%, sem taxa, sem instrutor pago): academia não recebe nada",
			p:    services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 5, DividirPote: true, PercentualPote: 100, InstrutorModo: models.InstrutorFixo},
			want: services.ResultadoFinanceiro{Arrecadacao: 5000, PotePremio: 5000, PremioTotal: 5000},
		},
		{
			nome: "desafio gratuito: só o prêmio fixo, a reserva inteira é consumida",
			p: services.ParamsFinanceiros{EntradaCentavos: 0, Inscritos: 4, PremioFixoCentavos: 8000, DividirPote: true, PercentualPote: 90,
				TaxaPct: 10, InstrutorModo: models.InstrutorFixo, InstrutorFixo: 2000, ReservaCentavos: 10000},
			want: services.ResultadoFinanceiro{PremioTotal: 8000, InstrutorFixo: 2000, InstrutorTotal: 2000},
		},
		{
			nome: "sem inscritos: a reserva sobra para a academia",
			p: services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 0, PremioFixoCentavos: 5000, DividirPote: true, PercentualPote: 90,
				TaxaPct: 10, InstrutorModo: models.InstrutorFixo, InstrutorFixo: 1000, ReservaCentavos: 6000},
			// premio total = 5000 (fixo), instrutor 1000, academia = 0 + 6000 - 5000 - 1000 = 0
			want: services.ResultadoFinanceiro{PremioTotal: 5000, InstrutorFixo: 1000, InstrutorTotal: 1000},
		},
		{
			// % do instrutor só conta nos modos com %: no modo fixo o pct gravado é ignorado.
			nome: "modo fixo ignora instrutor_pct",
			p: services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 10, DividirPote: true, PercentualPote: 90,
				TaxaPct: 10, InstrutorModo: models.InstrutorFixo, InstrutorPct: 50},
			want: services.ResultadoFinanceiro{Arrecadacao: 10000, PotePremio: 9000, PremioTotal: 9000, Taxa: 1000},
		},
		{
			nome: "taxa não reduz o pote: pote 90% + taxa 10% usa 100% das inscrições",
			p: services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 7, DividirPote: true, PercentualPote: 90, TaxaPct: 10,
				InstrutorModo: models.InstrutorFixo},
			want: services.ResultadoFinanceiro{Arrecadacao: 7000, PotePremio: 6300, PremioTotal: 6300, Taxa: 700},
		},
	}
	for _, c := range casos {
		// Cenários de 1 instrutor (antes da rodada 5): Instrutores=1 e o que cada membro recebe = o total.
		c.p.Instrutores = 1
		c.want.Instrutores = 1
		c.want.InstrutorPorMembro = c.want.InstrutorFixo + c.want.InstrutorPct
		got, err := services.CalcularFinanceiro(c.p)
		if err != nil {
			t.Errorf("%s: erro inesperado: %v", c.nome, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n  obteve   %+v\n  esperava %+v", c.nome, got, c.want)
		}
		fecha(t, c.p, got)
	}
}

// Rodada 5 (T1): fixo e % são POR instrutor; cada membro recebe fixo + floor(T x pct / 100) e a parte
// fixa das vagas não preenchidas (reservada) volta na parte da academia.
func TestCalcularFinanceiro_EquipeComVagaNaoPreenchida(t *testing.T) {
	// 10 x R$ 10 = 10000; pote 50%, taxa 10%, 3 vagas x (R$ 20 + 5%); só 2 membros no fechamento.
	// reserva = 5000 + 3 x 2000 = 11000. Cada membro: 2000 + 500 = 2500; total 5000.
	// academia = 10000 + 11000 - 10000 - 5000 - 1000 = 5000 (inclui os 2000 da vaga vazia).
	p := services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 10, PremioFixoCentavos: 5000, DividirPote: true,
		PercentualPote: 50, TaxaPct: 10, InstrutorModo: models.InstrutorFixoPercentual, InstrutorFixo: 2000, InstrutorPct: 5,
		Instrutores: 2, ReservaCentavos: services.ReservaNecessaria(5000, 2000, 3)}
	got, err := services.CalcularFinanceiro(p)
	if err != nil {
		t.Fatal(err)
	}
	want := services.ResultadoFinanceiro{Arrecadacao: 10000, PotePremio: 5000, PremioTotal: 10000, Taxa: 1000,
		InstrutorFixo: 2000, InstrutorPct: 500, InstrutorPorMembro: 2500, Instrutores: 2, InstrutorTotal: 5000, Academia: 5000}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("obteve %+v\nesperava %+v", got, want)
	}
	fecha(t, p, got)
	// Equipe completa (3): academia = 21000 - 10000 - 7500 - 1000 = 2500.
	p.Instrutores = 3
	if got, err = services.CalcularFinanceiro(p); err != nil || got.Academia != 2500 || got.InstrutorTotal != 7500 {
		t.Errorf("equipe completa: %+v err=%v", got, err)
	}
	fecha(t, p, got)
	// % por instrutor x membros entra na soma: 50 + 10 + 5 x 9 > 100.
	p.Instrutores = 9
	if _, err := services.CalcularFinanceiro(p); err == nil {
		t.Error("50 + 10 + 5x9 deveria falhar")
	}
}

func TestCalcularFinanceiro_Erros(t *testing.T) {
	// soma dos percentuais acima de 100
	if _, err := services.CalcularFinanceiro(services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 1, DividirPote: true,
		PercentualPote: 90, TaxaPct: 10, InstrutorModo: models.InstrutorPercentual, InstrutorPct: 1, Instrutores: 1}); err == nil {
		t.Error("90 + 10 + 1 deveria falhar")
	}
	// reserva não cobre o prêmio fixo: academia ficaria negativa
	if _, err := services.CalcularFinanceiro(services.ParamsFinanceiros{EntradaCentavos: 0, Inscritos: 0, PremioFixoCentavos: 1000,
		InstrutorModo: models.InstrutorFixo, ReservaCentavos: 500}); err == nil {
		t.Error("reserva menor que o prêmio fixo deveria falhar")
	}
	// com dividir_pote=false o percentual do pote não entra na soma
	if _, err := services.CalcularFinanceiro(services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 1, DividirPote: false,
		PercentualPote: 100, TaxaPct: 10, InstrutorModo: models.InstrutorFixo}); err != nil {
		t.Errorf("dividir_pote=false ignora percentual_pote: %v", err)
	}
}

func TestPremiosDaConta_DividirPremioSobreOTotalComPote(t *testing.T) {
	// Cenário completo acima: prêmio total 10000 em pódio de 3.
	if got, want := services.DividirPremio(10000, services.DivisaoPadraoBP(3), 3), []int64{6000, 3000, 1000}; !reflect.DeepEqual(got, want) {
		t.Errorf("3 presentes: %v, esperava %v", got, want)
	}
	// Só 2 presentes: renormaliza 60/30 -> 10000*60/90 = 6666, 10000*30/90 = 3333, resto 1 para o 1º.
	if got, want := services.DividirPremio(10000, services.DivisaoPadraoBP(3), 2), []int64{6667, 3333}; !reflect.DeepEqual(got, want) {
		t.Errorf("2 presentes: %v, esperava %v", got, want)
	}
	// 329 centavos (cenário de arredondamento) em pódio de 2: 70/30 -> 230 + 98 = 328, resto 1 -> 231/98.
	if got, want := services.DividirPremio(329, services.DivisaoPadraoBP(2), 2), []int64{231, 98}; !reflect.DeepEqual(got, want) {
		t.Errorf("329 em 2: %v, esperava %v", got, want)
	}
}

func TestMaxPotePct(t *testing.T) {
	for _, c := range []struct{ taxa, instr, want int }{{10, 0, 90}, {10, 15, 75}, {0, 0, 100}, {60, 50, 0}} {
		if got := services.MaxPotePct(c.taxa, c.instr); got != c.want {
			t.Errorf("MaxPotePct(%d,%d)=%d, esperava %d", c.taxa, c.instr, got, c.want)
		}
	}
}

func TestReservaNecessaria(t *testing.T) {
	if got := services.ReservaNecessaria(5000, 2000, 1); got != 7000 {
		t.Errorf("1 vaga: %d", got)
	}
	// T1: prêmio fixo + vagas x fixo por instrutor
	if got := services.ReservaNecessaria(5000, 2000, 3); got != 11000 {
		t.Errorf("3 vagas: %d", got)
	}
	if got := services.ReservaNecessaria(5000, 0, 4); got != 5000 {
		t.Errorf("só percentual: %d", got)
	}
}

func TestConfigFinanceira_Validar(t *testing.T) {
	base := func() services.ConfigFinanceira {
		return services.ConfigFinanceira{DividirPote: true, Taxa: 10, Modo: models.InstrutorFixo, Vagas: 1}
	}

	// percentual_pote omitido = máximo permitido (100 - taxa - instrutor)
	c := base()
	if err := c.Validar(true); err != nil || c.PercentualPote != 90 {
		t.Errorf("pote omitido: err=%v pote=%d (esperava 90)", err, c.PercentualPote)
	}
	c = base()
	c.Modo, c.Pct = models.InstrutorFixoPercentual, 15
	if err := c.Validar(true); err != nil || c.PercentualPote != 75 {
		t.Errorf("pote omitido com instrutor 15%%: err=%v pote=%d (esperava 75)", err, c.PercentualPote)
	}
	// dividir_pote=false grava 0 mesmo que o cliente mande 100
	c = base()
	c.DividirPote, c.PercentualPote = false, 100
	if err := c.Validar(false); err != nil || c.PercentualPote != 0 {
		t.Errorf("sem dividir: err=%v pote=%d (esperava 0)", err, c.PercentualPote)
	}
	// soma acima de 100: mensagem em português com o máximo permitido para o pote
	c = base()
	c.PercentualPote = 95
	err := c.Validar(false)
	if err == nil {
		t.Fatal("95 + 10 deveria falhar")
	}
	for _, trecho := range []string{"não pode passar de 100%", "o máximo para o pote é 90%", "taxa de 10%"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("mensagem %q sem %q", err.Error(), trecho)
		}
	}
	// regras do instrutor
	c = base()
	c.Pct = 5 // modo fixo não aceita %
	if err := c.Validar(true); err == nil {
		t.Error("instrutor_pct no modo fixo deveria falhar")
	}
	c = base()
	c.Modo, c.Fixo = models.InstrutorPercentual, 100
	if err := c.Validar(true); err == nil {
		t.Error("instrutor_fixo no modo percentual deveria falhar")
	}
	// equipe (T1): % por instrutor x vagas na soma; pote omitido = 100 - taxa - pct x vagas
	c = base()
	c.Modo, c.Pct, c.Vagas = models.InstrutorPercentual, 10, 3
	if err := c.Validar(true); err != nil || c.PercentualPote != 60 {
		t.Errorf("pote omitido com 3 x 10%%: err=%v pote=%d (esperava 60)", err, c.PercentualPote)
	}
	c = base()
	c.Modo, c.Pct, c.Vagas, c.PercentualPote = models.InstrutorPercentual, 10, 3, 61
	if err := c.Validar(false); err == nil || !strings.Contains(err.Error(), "10% x 3 = 30%") {
		t.Errorf("61 + 10 + 3x10 deveria falhar com a conta por instrutor: %v", err)
	}
	c = base()
	c.Fixo, c.Vagas, c.PremioFixo = 2000, 4, 1000
	if err := c.Validar(true); err != nil || c.Reserva() != 9000 {
		t.Errorf("reserva 1000 + 4 x 2000: err=%v reserva=%d", err, c.Reserva())
	}
	for _, v := range []int{0, 21} {
		c = base()
		c.Vagas = v
		if err := c.Validar(true); err == nil {
			t.Errorf("instrutores_vagas=%d deveria falhar", v)
		}
	}
}
