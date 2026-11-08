package services_test

import (
	"reflect"
	"strings"
	"testing"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
)

func ptr64(v int64) *int64 { return &v }

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

func TestCalcularFinanceiro_Erros(t *testing.T) {
	// soma dos percentuais acima de 100
	if _, err := services.CalcularFinanceiro(services.ParamsFinanceiros{EntradaCentavos: 1000, Inscritos: 1, DividirPote: true,
		PercentualPote: 90, TaxaPct: 10, InstrutorModo: models.InstrutorPercentual, InstrutorPct: 1}); err == nil {
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
	if got, want := services.DividirPremio(10000, 3, 3), []int64{6000, 3000, 1000}; !reflect.DeepEqual(got, want) {
		t.Errorf("3 presentes: %v, esperava %v", got, want)
	}
	// Só 2 presentes: renormaliza 60/30 -> 10000*60/90 = 6666, 10000*30/90 = 3333, resto 1 para o 1º.
	if got, want := services.DividirPremio(10000, 3, 2), []int64{6667, 3333}; !reflect.DeepEqual(got, want) {
		t.Errorf("2 presentes: %v, esperava %v", got, want)
	}
	// 329 centavos (cenário de arredondamento) em pódio de 2: 70/30 -> 230 + 98 = 328, resto 1 -> 231/98.
	if got, want := services.DividirPremio(329, 2, 2), []int64{231, 98}; !reflect.DeepEqual(got, want) {
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
	if got := services.ReservaNecessaria(5000, 2000, nil); got != 7000 {
		t.Errorf("designado: %d", got)
	}
	// vaga aberta sem escolhido: prêmio fixo + teto
	if got := services.ReservaNecessaria(5000, 0, ptr64(3000)); got != 8000 {
		t.Errorf("vaga aberta: %d", got)
	}
}

func TestConfigFinanceira_Validar(t *testing.T) {
	base := func() services.ConfigFinanceira {
		return services.ConfigFinanceira{DividirPote: true, Taxa: 10, Modo: models.InstrutorFixo, Selecao: models.SelecaoDesignado}
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
	c = base()
	c.Selecao = models.SelecaoVagaAberta // sem teto
	if err := c.Validar(true); err == nil {
		t.Error("vaga aberta sem teto deveria falhar")
	}
	c = base()
	c.Selecao, c.Teto = models.SelecaoVagaAberta, ptr64(3000)
	if err := c.Validar(true); err != nil || c.Reserva() != 3000 {
		t.Errorf("vaga aberta com teto: err=%v reserva=%d", err, c.Reserva())
	}
	c = base()
	c.Teto = ptr64(100) // teto só vale em vaga aberta
	if err := c.Validar(true); err == nil {
		t.Error("teto em designado deveria falhar")
	}
	c = base()
	c.Selecao, c.Teto, c.Fixo = models.SelecaoVagaAberta, ptr64(3000), 500
	if err := c.Validar(true); err == nil {
		t.Error("fixo na vaga aberta (antes da escolha) deveria falhar")
	}
}
