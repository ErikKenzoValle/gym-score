package models

import "time"

// Modos de pagamento do instrutor (por desafio).
const (
	InstrutorFixo           = "fixo"
	InstrutorPercentual     = "percentual"
	InstrutorFixoPercentual = "fixo_percentual"
)

// Como o instrutor do desafio era escolhido — LEGADO desde a 00021 (a equipe substituiu o leilão). Desafios
// novos gravam sempre "designado"; "vaga_aberta" só existe em linhas antigas.
const (
	SelecaoDesignado  = "designado"
	SelecaoVagaAberta = "vaga_aberta"
)

// Limites financeiros da criação de desafio (centavos).
const (
	PremioFixoMaxCentavos    int64 = 10_000_000 // R$ 100.000,00
	InstrutorFixoMaxCentavos int64 = 10_000_000
	MensagemCandidaturaMax         = 500
)

// Status da candidatura de um instrutor à equipe do desafio.
const (
	CandidaturaPendente  = "pendente"
	CandidaturaEscolhida = "escolhida" // aceita: o instrutor entrou na equipe
	CandidaturaRecusada  = "recusada"
	CandidaturaRetirada  = "retirada"
	CandidaturaRemovida  = "removida" // 00021: membro aceito e depois removido da equipe pela academia
)

// Origem da receita da plataforma.
const (
	ReceitaTaxaInscricao = "taxa_inscricao"
	ReceitaTarifaSaque   = "tarifa_saque"
)

// ModoTemFixo / ModoTemPercentual dizem quais partes o modo de pagamento do instrutor inclui.
func ModoTemFixo(modo string) bool { return modo == InstrutorFixo || modo == InstrutorFixoPercentual }
func ModoTemPercentual(modo string) bool {
	return modo == InstrutorPercentual || modo == InstrutorFixoPercentual
}

// ModoInstrutorValido indica se modo é um dos três aceitos.
func ModoInstrutorValido(modo string) bool {
	return modo == InstrutorFixo || modo == InstrutorPercentual || modo == InstrutorFixoPercentual
}

// CandidaturaInstrutor é a candidatura de um instrutor a uma vaga da equipe do desafio (tabela
// desafio_candidaturas_instrutor): o instrutor aceita as condições definidas pela academia (sem lance).
// Re-candidatar após retirar/ser recusado reaproveita a mesma linha (UK desafio+instrutor).
type CandidaturaInstrutor struct {
	ID          uint `gorm:"primaryKey;autoIncrement"`
	IDDesafio   uint `gorm:"column:id_desafio;not null"`
	IDInstrutor uint `gorm:"column:id_instrutor;not null"`
	// LanceCentavos é LEGADO (leilão, até a 00020): NULL nas candidaturas novas; nunca lido pelas regras.
	LanceCentavos *int64    `gorm:"column:lance_centavos"`
	Mensagem      *string   `gorm:"column:mensagem;size:500"`
	Status        string    `gorm:"column:status;not null;default:pendente"`
	CriadoEm      time.Time `gorm:"column:criado_em;autoCreateTime"`
	AtualizadoEm  time.Time `gorm:"column:atualizado_em;autoUpdateTime"`

	Instrutor *Instrutor `gorm:"foreignKey:IDInstrutor"`
}

func (CandidaturaInstrutor) TableName() string { return "desafio_candidaturas_instrutor" }

// PlataformaReceita é uma linha da receita da plataforma (não é carteira de usuário).
type PlataformaReceita struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	Origem        string    `gorm:"column:origem;not null"`
	ValorCentavos int64     `gorm:"column:valor_centavos;not null"`
	IDDesafio     *uint     `gorm:"column:id_desafio"`
	IDTransacao   *uint     `gorm:"column:id_transacao"`
	CriadoEm      time.Time `gorm:"column:criado_em;autoCreateTime"`
}

func (PlataformaReceita) TableName() string { return "plataforma_receitas" }

// ─── DTOs de entrada ─────────────────────────────────────────────────────────

// CandidatarRequest é o corpo de POST /desafios/:id/candidaturas (instrutor do token): candidatar-se é
// aceitar as condições da academia; mensagem opcional. Repetir a chamada atualiza a mensagem enquanto a
// candidatura estiver pendente. Um `lance_centavos` enviado (front antigo) é ignorado.
type CandidatarRequest struct {
	Mensagem string `json:"mensagem" validate:"max=500"`
}

// ─── DTOs de saída ───────────────────────────────────────────────────────────

// PremiacaoResponse é o que qualquer pessoa que enxerga o desafio vê da premiação: o prêmio total
// previsto e a lista por posição (posicoes_podio itens, sobre o pote atual).
type PremiacaoResponse struct {
	PremioTotalCentavos int64   `json:"premio_total_centavos"`
	PremioTotal         float64 `json:"premio_total"`
	PorPosicaoCentavos  []int64 `json:"por_posicao_centavos"`
	// DivisaoBP: a fatia de cada posição em pontos-base (10000 = 100%), mesma ordem de por_posicao_centavos.
	DivisaoBP []int `json:"divisao_bp"`
}

// FinanceiroResponse é a visão financeira da academia dona (ninguém mais recebe este objeto). Tudo em
// centavos e calculado sobre as inscrições ATUAIS (com o desafio encerrado, é o valor efetivamente pago).
type FinanceiroResponse struct {
	ArrecadacaoCentavos    int64  `json:"arrecadacao_centavos"` // entradas dos inscritos (T)
	PremioFixoCentavos     int64  `json:"premio_fixo_centavos"`
	DividirPote            bool   `json:"dividir_pote"`
	PercentualPote         int    `json:"percentual_pote"`
	PotePremioCentavos     int64  `json:"pote_premio_centavos"` // parcela das inscrições que vai para os atletas
	PremioTotalCentavos    int64  `json:"premio_total_centavos"`
	TaxaPlataformaPct      int    `json:"taxa_plataforma_pct"`
	TaxaPlataformaCentavos int64  `json:"taxa_plataforma_centavos"`
	InstrutorModoPagamento string `json:"instrutor_modo_pagamento"`
	// instrutor_fixo_centavos, instrutor_pct e instrutor_pct_centavos são POR instrutor;
	// instrutor_total_centavos é o total da equipe (instrutores_pagos x (fixo + parte do %)).
	InstrutorFixoCentavos  int64 `json:"instrutor_fixo_centavos"`
	InstrutorPct           int   `json:"instrutor_pct"`
	InstrutorPctCentavos   int64 `json:"instrutor_pct_centavos"`
	InstrutorTotalCentavos int64 `json:"instrutor_total_centavos"`
	// InstrutoresVagas: tamanho da equipe; EquipeCount: membros atuais; InstrutoresPagos: quantos entram na
	// previsão (aberto: a equipe completa, igual à reserva; depois de iniciar: os membros).
	InstrutoresVagas          int   `json:"instrutores_vagas"`
	EquipeCount               int   `json:"equipe_count"`
	InstrutoresPagos          int   `json:"instrutores_pagos"`
	LiquidoPrevistoCentavos   int64 `json:"liquido_previsto_centavos"`   // o que a academia recebe de volta no encerramento
	ResultadoPrevistoCentavos int64 `json:"resultado_previsto_centavos"` // liquido - reserva (negativo = custo da academia)
	ReservaCentavos           int64 `json:"reserva_centavos"`            // garantia debitada do saldo da academia
	CandidaturasPendentes     int   `json:"candidaturas_pendentes"`
}

// GanhoInstrutorResponse é o que um membro da equipe vê: quanto ELE vai ganhar neste desafio.
type GanhoInstrutorResponse struct {
	Responsavel           bool   `json:"responsavel"`
	ModoPagamento         string `json:"modo_pagamento"`
	FixoCentavos          int64  `json:"fixo_centavos"`
	Pct                   int    `json:"pct"`
	PctCentavos           int64  `json:"pct_centavos"` // previsto, sobre as inscrições atuais
	TotalPrevistoCentavos int64  `json:"total_previsto_centavos"`
}

// VagaInstrutorResponse é a oferta de trabalho vista por instrutores: as condições POR instrutor que a
// academia definiu (modo, fixo, %, previsão do % sobre as inscrições atuais), o tamanho da equipe, as
// vagas livres e a candidatura do próprio instrutor, se houver.
type VagaInstrutorResponse struct {
	ModoPagamento       string               `json:"modo_pagamento"`
	FixoCentavos        int64                `json:"fixo_centavos"`
	Pct                 int                  `json:"pct"`
	PctPrevistoCentavos int64                `json:"pct_previsto_centavos"`
	InstrutoresVagas    int                  `json:"instrutores_vagas"`
	VagasAbertas        int                  `json:"vagas_abertas"`
	MinhaCandidatura    *CandidaturaResponse `json:"minha_candidatura"`
}

// CandidaturaResponse é uma candidatura (status: pendente | escolhida (= aceita, está na equipe) |
// recusada | retirada | removida). Para a academia dona inclui o resumo do instrutor.
type CandidaturaResponse struct {
	ID           uint             `json:"id"`
	IDDesafio    uint             `json:"id_desafio"`
	Mensagem     string           `json:"mensagem"`
	Status       string           `json:"status"`
	CriadoEm     time.Time        `json:"criado_em"`
	AtualizadoEm time.Time        `json:"atualizado_em"`
	Instrutor    *CandidatoResumo `json:"instrutor,omitempty"` // só na lista da academia dona
}

// CandidatoResumo identifica o instrutor candidato (nome, CREF, especialidade e se é da academia).
type CandidatoResumo struct {
	ID            uint   `json:"id"`
	Nome          string `json:"nome"`
	CREF          string `json:"cref"`
	Especialidade string `json:"especialidade"`
	Vinculado     bool   `json:"vinculado"` // vinculado a esta academia
}

// TaxasPlataformaResponse são as taxas vigentes, para a UI mostrar o máximo do pote e a tarifa de saque.
type TaxasPlataformaResponse struct {
	Modo                string `json:"modo"`
	TaxaInscricaoPct    int    `json:"taxa_inscricao_pct"`
	TarifaSaqueCentavos int64  `json:"tarifa_saque_centavos"`
}
