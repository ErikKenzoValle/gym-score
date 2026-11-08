package services

import (
	"fmt"

	"gymScore-backend/internal/models"
)

// ─── Conta do desafio (funções puras, sem banco) ─────────────────────────────

// ParamsFinanceiros são os dados do desafio que entram na conta do encerramento.
type ParamsFinanceiros struct {
	EntradaCentavos    int64
	Inscritos          int
	PremioFixoCentavos int64
	DividirPote        bool
	PercentualPote     int
	TaxaPct            int
	InstrutorModo      string
	InstrutorFixo      int64
	InstrutorPct       int
	// ReservaCentavos é a garantia vigente debitada da academia.
	ReservaCentavos int64
}

// ResultadoFinanceiro é a repartição, em centavos, do dinheiro de um desafio:
//
//	Arrecadacao + Reserva = PremioTotal + InstrutorTotal + Academia + Taxa   (ao centavo)
type ResultadoFinanceiro struct {
	Arrecadacao    int64 // T: soma das entradas dos inscritos que restaram
	PotePremio     int64 // floor(T x percentual_pote / 100) se dividir_pote, senão 0
	PremioTotal    int64 // premio_fixo + PotePremio (repartido entre as posições por DividirPremio)
	Taxa           int64 // floor(T x taxa / 100): sai da parte da academia, nunca do pote dos atletas
	InstrutorFixo  int64
	InstrutorPct   int64 // floor(T x instrutor_pct / 100) (0 se o modo não inclui %)
	InstrutorTotal int64
	// Academia é o que volta para a academia: T + reserva - prêmios - instrutor - taxa. Inclui o resto
	// de arredondamento dos percentuais (os centavos que faltam em cada conta ficam com a academia).
	Academia int64
}

// pctDe devolve floor(valor x pct / 100) sem estourar int64 nos valores do domínio.
func pctDe(valor int64, pct int) int64 {
	if valor <= 0 || pct <= 0 {
		return 0
	}
	return valor * int64(pct) / 100
}

// CalcularFinanceiro faz a conta do desafio. Erra se os parâmetros violam as regras (soma de % acima
// de 100 ou resultado da academia negativo): nesses casos nada deve ser pago.
func CalcularFinanceiro(p ParamsFinanceiros) (ResultadoFinanceiro, error) {
	var r ResultadoFinanceiro
	if p.PercentualPote < 0 || p.TaxaPct < 0 || p.InstrutorPct < 0 {
		return r, fmt.Errorf("percentuais negativos")
	}
	pote := 0
	if p.DividirPote {
		pote = p.PercentualPote
	}
	instrPct := 0
	if models.ModoTemPercentual(p.InstrutorModo) {
		instrPct = p.InstrutorPct
	}
	if pote+p.TaxaPct+instrPct > 100 {
		return r, fmt.Errorf("a soma dos percentuais (%d%%) passa de 100%%", pote+p.TaxaPct+instrPct)
	}
	r.Arrecadacao = p.EntradaCentavos * int64(p.Inscritos)
	r.PotePremio = pctDe(r.Arrecadacao, pote)
	r.PremioTotal = p.PremioFixoCentavos + r.PotePremio
	r.Taxa = pctDe(r.Arrecadacao, p.TaxaPct)
	r.InstrutorFixo = p.InstrutorFixo
	r.InstrutorPct = pctDe(r.Arrecadacao, instrPct)
	r.InstrutorTotal = r.InstrutorFixo + r.InstrutorPct
	r.Academia = r.Arrecadacao + p.ReservaCentavos - r.PremioTotal - r.InstrutorTotal - r.Taxa
	if r.Academia < 0 {
		return r, fmt.Errorf("a reserva (%d) não cobre o prêmio fixo e o instrutor", p.ReservaCentavos)
	}
	return r, nil
}

// MaxPotePct é o maior percentual do pote permitido com a taxa e o % do instrutor dados.
func MaxPotePct(taxaPct, instrutorPct int) int {
	m := 100 - taxaPct - instrutorPct
	if m < 0 {
		return 0
	}
	return m
}

// ReservaNecessaria é a garantia que a academia precisa ter debitada: prêmio fixo + a parte fixa do
// instrutor (o teto, enquanto a vaga está aberta e ninguém foi escolhido).
func ReservaNecessaria(premioFixo, instrutorFixo int64, tetoVaga *int64) int64 {
	if tetoVaga != nil {
		return premioFixo + *tetoVaga
	}
	return premioFixo + instrutorFixo
}

// paramsDoDesafio extrai os parâmetros da conta do model.
func paramsDoDesafio(d *models.Desafio, inscritos int) ParamsFinanceiros {
	return ParamsFinanceiros{
		EntradaCentavos: d.ValorEntradaCentavos, Inscritos: inscritos,
		PremioFixoCentavos: d.PremioFixoCentavos, DividirPote: d.DividirPote, PercentualPote: d.PercentualPote,
		TaxaPct: d.TaxaPlataformaPct, InstrutorModo: d.InstrutorModoPagamento,
		InstrutorFixo: d.InstrutorFixoCentavos, InstrutorPct: d.InstrutorPct,
		ReservaCentavos: d.ReservaAcademiaCentavos,
	}
}

// msgSomaPct é a mensagem da API quando a soma dos percentuais passa de 100, dizendo o máximo do pote.
func msgSomaPct(pote, taxa, instrutor int) string {
	return fmt.Sprintf(
		"A soma do percentual do pote (%d%%), da taxa da plataforma (%d%%) e do percentual do instrutor (%d%%) não pode passar de 100%%. "+
			"Com a taxa de %d%% e o instrutor em %d%%, o máximo para o pote é %d%%",
		pote, taxa, instrutor, taxa, instrutor, MaxPotePct(taxa, instrutor))
}

// msgSaldoReserva é a mensagem de saldo insuficiente para a reserva, com o link para depositar.
func msgSaldoReserva(valor int64) string {
	return fmt.Sprintf("Saldo insuficiente para a reserva de %s (prêmio fixo + instrutor). Deposite em /depositar e tente novamente", reaisBR(valor))
}

// ConfigFinanceira é a configuração financeira de um desafio (criação e edição validam a MESMA regra).
type ConfigFinanceira struct {
	PremioFixo     int64
	DividirPote    bool
	PercentualPote int
	Taxa           int // snapshot da taxa da plataforma no desafio
	Modo           string
	Fixo           int64
	Pct            int
	Selecao        string
	Teto           *int64
	// Escolhido: vaga aberta com instrutor já escolhido (Fixo passou a ser o lance).
	Escolhido bool
}

// Validar normaliza e valida a configuração. poteOmitido: o cliente não informou percentual_pote, então
// vale o máximo permitido (100 - taxa - % do instrutor). Com dividir_pote=false, percentual_pote vira 0.
func (c *ConfigFinanceira) Validar(poteOmitido bool) error {
	if c.PremioFixo < 0 || c.PremioFixo > models.PremioFixoMaxCentavos {
		return erroRequisicao("premio_fixo_centavos fora do permitido (R$ 0,00 a R$ 100.000,00)")
	}
	if !models.ModoInstrutorValido(c.Modo) {
		return erroRequisicao("instrutor_modo_pagamento inválido (use fixo, percentual ou fixo_percentual)")
	}
	if c.Selecao != models.SelecaoDesignado && c.Selecao != models.SelecaoVagaAberta {
		return erroRequisicao("instrutor_selecao inválido (use designado ou vaga_aberta)")
	}
	if c.Fixo < 0 || c.Fixo > models.InstrutorFixoMaxCentavos {
		return erroRequisicao("instrutor_fixo_centavos fora do permitido (R$ 0,00 a R$ 100.000,00)")
	}
	if c.Pct < 0 || c.Pct > 100 {
		return erroRequisicao("instrutor_pct deve estar entre 0 e 100")
	}
	if c.Pct > 0 && !models.ModoTemPercentual(c.Modo) {
		return erroRequisicao("instrutor_pct só vale nos modos percentual e fixo_percentual")
	}
	if c.Fixo > 0 && !models.ModoTemFixo(c.Modo) && !c.Escolhido {
		return erroRequisicao("instrutor_fixo_centavos só vale nos modos fixo e fixo_percentual")
	}
	switch c.Selecao {
	case models.SelecaoDesignado:
		if c.Teto != nil {
			return erroRequisicao("instrutor_teto_centavos só vale com instrutor_selecao=vaga_aberta")
		}
	case models.SelecaoVagaAberta:
		if c.Teto == nil {
			return erroRequisicao("Informe instrutor_teto_centavos (o teto do lance) para abrir a vaga de instrutor")
		}
		if *c.Teto < 0 || *c.Teto > models.InstrutorFixoMaxCentavos {
			return erroRequisicao("instrutor_teto_centavos fora do permitido (R$ 0,00 a R$ 100.000,00)")
		}
		if !c.Escolhido && c.Fixo > 0 {
			return erroRequisicao("Na vaga aberta o valor do instrutor é o lance escolhido: não informe instrutor_fixo_centavos")
		}
	}
	instrPct := 0
	if models.ModoTemPercentual(c.Modo) {
		instrPct = c.Pct
	}
	switch {
	case !c.DividirPote:
		c.PercentualPote = 0
	case poteOmitido:
		c.PercentualPote = MaxPotePct(c.Taxa, instrPct)
	}
	if c.PercentualPote < 0 || c.PercentualPote > 100 {
		return erroRequisicao("percentual_pote deve estar entre 0 e 100")
	}
	if c.PercentualPote+c.Taxa+instrPct > 100 {
		return erroRequisicao(msgSomaPct(c.PercentualPote, c.Taxa, instrPct))
	}
	return nil
}

// Reserva é a garantia que a academia precisa ter debitada nesta configuração.
func (c ConfigFinanceira) Reserva() int64 {
	if c.Selecao == models.SelecaoVagaAberta && !c.Escolhido {
		return ReservaNecessaria(c.PremioFixo, 0, c.Teto)
	}
	return ReservaNecessaria(c.PremioFixo, c.Fixo, nil)
}

func configDoDesafio(d *models.Desafio) ConfigFinanceira {
	return ConfigFinanceira{
		PremioFixo: d.PremioFixoCentavos, DividirPote: d.DividirPote, PercentualPote: d.PercentualPote,
		Taxa: d.TaxaPlataformaPct, Modo: d.InstrutorModoPagamento, Fixo: d.InstrutorFixoCentavos,
		Pct: d.InstrutorPct, Selecao: d.InstrutorSelecao, Teto: d.InstrutorTetoCentavos,
		Escolhido: d.InstrutorSelecao == models.SelecaoVagaAberta && d.IDInstrutor != nil,
	}
}
