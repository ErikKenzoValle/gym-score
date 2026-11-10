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
	// InstrutorFixo e InstrutorPct são POR instrutor (rodada 5); Instrutores é quantos recebem (os membros
	// da equipe no encerramento; na previsão do desafio aberto, a equipe completa).
	InstrutorFixo int64
	InstrutorPct  int
	Instrutores   int
	// ReservaCentavos é a garantia vigente debitada da academia.
	ReservaCentavos int64
}

// ResultadoFinanceiro é a repartição, em centavos, do dinheiro de um desafio:
//
//	Arrecadacao + Reserva = PremioTotal + InstrutorTotal + Academia + Taxa   (ao centavo)
type ResultadoFinanceiro struct {
	Arrecadacao   int64 // T: soma das entradas dos inscritos que restaram
	PotePremio    int64 // floor(T x percentual_pote / 100) se dividir_pote, senão 0
	PremioTotal   int64 // premio_fixo + PotePremio (repartido entre as posições por DividirPremio)
	Taxa          int64 // floor(T x taxa / 100): sai da parte da academia, nunca do pote dos atletas
	InstrutorFixo int64 // por instrutor
	InstrutorPct  int64 // por instrutor: floor(T x instrutor_pct / 100) (0 se o modo não inclui %)
	// InstrutorPorMembro = InstrutorFixo + InstrutorPct (o que CADA membro recebe);
	// InstrutorTotal = Instrutores x InstrutorPorMembro.
	InstrutorPorMembro int64
	Instrutores        int
	InstrutorTotal     int64
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
	if p.PercentualPote < 0 || p.TaxaPct < 0 || p.InstrutorPct < 0 || p.Instrutores < 0 {
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
	if soma := pote + p.TaxaPct + instrPct*p.Instrutores; soma > 100 {
		return r, fmt.Errorf("a soma dos percentuais (%d%%) passa de 100%%", soma)
	}
	r.Arrecadacao = p.EntradaCentavos * int64(p.Inscritos)
	r.PotePremio = pctDe(r.Arrecadacao, pote)
	r.PremioTotal = p.PremioFixoCentavos + r.PotePremio
	r.Taxa = pctDe(r.Arrecadacao, p.TaxaPct)
	r.InstrutorFixo = p.InstrutorFixo
	r.InstrutorPct = pctDe(r.Arrecadacao, instrPct)
	r.InstrutorPorMembro = r.InstrutorFixo + r.InstrutorPct
	r.Instrutores = p.Instrutores
	r.InstrutorTotal = int64(p.Instrutores) * r.InstrutorPorMembro
	r.Academia = r.Arrecadacao + p.ReservaCentavos - r.PremioTotal - r.InstrutorTotal - r.Taxa
	if r.Academia < 0 {
		return r, fmt.Errorf("a reserva (%d) não cobre o prêmio fixo e o instrutor", p.ReservaCentavos)
	}
	return r, nil
}

// MaxPotePct é o maior percentual do pote permitido com a taxa e o % TOTAL dos instrutores dados
// (instrutor_pct x instrutores_vagas).
func MaxPotePct(taxaPct, instrutoresPct int) int {
	m := 100 - taxaPct - instrutoresPct
	if m < 0 {
		return 0
	}
	return m
}

// ReservaNecessaria é a garantia que a academia precisa ter debitada (T1, rodada 5): prêmio fixo + a
// parte fixa de CADA vaga da equipe (instrutores_vagas x instrutor_fixo). Vagas que não forem preenchidas
// voltam para a academia no encerramento (dentro da parte da academia) ou no cancelamento (reserva inteira).
func ReservaNecessaria(premioFixo, instrutorFixo int64, vagas int) int64 {
	return premioFixo + int64(vagas)*instrutorFixo
}

// paramsDoDesafio extrai os parâmetros da conta do model; instrutores = quantos membros recebem.
func paramsDoDesafio(d *models.Desafio, inscritos, instrutores int) ParamsFinanceiros {
	return ParamsFinanceiros{
		EntradaCentavos: d.ValorEntradaCentavos, Inscritos: inscritos,
		PremioFixoCentavos: d.PremioFixoCentavos, DividirPote: d.DividirPote, PercentualPote: d.PercentualPote,
		TaxaPct: d.TaxaPlataformaPct, InstrutorModo: d.InstrutorModoPagamento,
		InstrutorFixo: d.InstrutorFixoCentavos, InstrutorPct: d.InstrutorPct, Instrutores: instrutores,
		ReservaCentavos: d.ReservaAcademiaCentavos,
	}
}

// msgSomaPct é a mensagem da API quando a soma dos percentuais passa de 100, dizendo o máximo do pote.
// instrutor é o % POR instrutor e vagas o tamanho da equipe.
func msgSomaPct(pote, taxa, instrutor, vagas int) string {
	total := instrutor * vagas
	return fmt.Sprintf(
		"A soma do percentual do pote (%d%%), da taxa da plataforma (%d%%) e do percentual dos instrutores (%d%% x %d = %d%%) não pode passar de 100%%. "+
			"Com a taxa de %d%% e os instrutores em %d%%, o máximo para o pote é %d%%",
		pote, taxa, instrutor, vagas, total, taxa, total, MaxPotePct(taxa, total))
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
	Fixo           int64 // por instrutor
	Pct            int   // por instrutor
	Vagas          int   // instrutores_vagas (tamanho da equipe)
}

// Validar normaliza e valida a configuração. poteOmitido: o cliente não informou percentual_pote, então
// vale o máximo permitido (100 - taxa - % do instrutor x vagas). Com dividir_pote=false, percentual_pote vira 0.
func (c *ConfigFinanceira) Validar(poteOmitido bool) error {
	if c.PremioFixo < 0 || c.PremioFixo > models.PremioFixoMaxCentavos {
		return erroRequisicao("premio_fixo_centavos fora do permitido (R$ 0,00 a R$ 100.000,00)")
	}
	if !models.ModoInstrutorValido(c.Modo) {
		return erroRequisicao("instrutor_modo_pagamento inválido (use fixo, percentual ou fixo_percentual)")
	}
	if c.Vagas < models.InstrutoresVagasMin || c.Vagas > models.InstrutoresVagasMax {
		return erroRequisicao(fmt.Sprintf("instrutores_vagas deve estar entre %d e %d", models.InstrutoresVagasMin, models.InstrutoresVagasMax))
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
	if c.Fixo > 0 && !models.ModoTemFixo(c.Modo) {
		return erroRequisicao("instrutor_fixo_centavos só vale nos modos fixo e fixo_percentual")
	}
	instrPct := 0
	if models.ModoTemPercentual(c.Modo) {
		instrPct = c.Pct
	}
	switch {
	case !c.DividirPote:
		c.PercentualPote = 0
	case poteOmitido:
		c.PercentualPote = MaxPotePct(c.Taxa, instrPct*c.Vagas)
	}
	if c.PercentualPote < 0 || c.PercentualPote > 100 {
		return erroRequisicao("percentual_pote deve estar entre 0 e 100")
	}
	if c.PercentualPote+c.Taxa+instrPct*c.Vagas > 100 {
		return erroRequisicao(msgSomaPct(c.PercentualPote, c.Taxa, instrPct, c.Vagas))
	}
	return nil
}

// Reserva é a garantia que a academia precisa ter debitada nesta configuração.
func (c ConfigFinanceira) Reserva() int64 {
	return ReservaNecessaria(c.PremioFixo, c.Fixo, c.Vagas)
}

func configDoDesafio(d *models.Desafio) ConfigFinanceira {
	return ConfigFinanceira{
		PremioFixo: d.PremioFixoCentavos, DividirPote: d.DividirPote, PercentualPote: d.PercentualPote,
		Taxa: d.TaxaPlataformaPct, Modo: d.InstrutorModoPagamento, Fixo: d.InstrutorFixoCentavos,
		Pct: d.InstrutorPct, Vagas: d.InstrutoresVagas,
	}
}
