package models

import (
	"encoding/json"
	"time"
)

// ─── DTOs de entrada do painel admin ─────────────────────────────────────────

// AdminLoginRequest é o corpo de POST /api/admin/auth/login.
type AdminLoginRequest struct {
	Email string `json:"email" validate:"required,email,max=150"`
	Senha string `json:"senha" validate:"required,max=200"`
}

// AdminTrocarSenhaRequest é o corpo de POST /api/admin/auth/senha (o próprio admin).
type AdminTrocarSenhaRequest struct {
	SenhaAtual string `json:"senha_atual" validate:"required,max=200"`
	NovaSenha  string `json:"nova_senha" validate:"required,min=12,max=72"`
}

// MotivoRequest é o corpo de toda ação de escrita do admin: o motivo vai para admin_auditoria.
type MotivoRequest struct {
	Motivo string `json:"motivo" validate:"required,min=3,max=500"`
}

// CriarAdminRequest é o corpo de POST /api/admin/administradores (superadmin).
type CriarAdminRequest struct {
	Nome   string `json:"nome" validate:"required,min=2,max=100"`
	Email  string `json:"email" validate:"required,email,max=150"`
	Papel  string `json:"papel" validate:"required,oneof=superadmin admin"`
	Motivo string `json:"motivo" validate:"required,min=3,max=500"`
}

// TrocarPapelAdminRequest é o corpo de POST /api/admin/administradores/:id/papel (superadmin).
type TrocarPapelAdminRequest struct {
	Papel  string `json:"papel" validate:"required,oneof=superadmin admin"`
	Motivo string `json:"motivo" validate:"required,min=3,max=500"`
}

// ─── DTOs de saída ───────────────────────────────────────────────────────────

// PaginaAdmin é uma página das listagens do painel (pagina >= 1, limite 1..100, padrão 20).
type PaginaAdmin[T any] struct {
	Itens  []T   `json:"itens"`
	Total  int64 `json:"total"`
	Pagina int   `json:"pagina"`
	Limite int   `json:"limite"`
}

// AdminResponse é um administrador (sem senha_hash nem token_versao).
type AdminResponse struct {
	ID            uint       `json:"id"`
	Nome          string     `json:"nome"`
	Email         string     `json:"email"`
	Papel         string     `json:"papel"`
	Ativo         bool       `json:"ativo"`
	UltimoLoginEm *time.Time `json:"ultimo_login_em,omitempty"`
	CriadoPor     *uint      `json:"criado_por,omitempty"`
	CriadoEm      time.Time  `json:"criado_em"`
}

// NovoAdminResponse converte o model no DTO.
func NovoAdminResponse(a *Administrador) AdminResponse {
	return AdminResponse{ID: a.ID, Nome: a.Nome, Email: a.Email, Papel: a.Papel, Ativo: a.Ativo,
		UltimoLoginEm: a.UltimoLoginEm, CriadoPor: a.CriadoPor, CriadoEm: a.CriadoEm}
}

// AdminSenhaTemporariaResponse devolve o admin e a senha temporária (exibida UMA vez; criar e resetar).
type AdminSenhaTemporariaResponse struct {
	Admin           AdminResponse `json:"admin"`
	SenhaTemporaria string        `json:"senha_temporaria"`
}

// AcaoAdminResponse é a resposta das ações idempotentes: Alterado=false quando nada mudou
// (já estava no estado pedido); nesse caso não há linha de auditoria.
type AcaoAdminResponse struct {
	Alterado bool `json:"alterado"`
	Dados    any  `json:"dados,omitempty"`
}

// UsuarioResumoAdmin identifica um usuário nas listagens do admin (sem CPF/saldo).
type UsuarioResumoAdmin struct {
	ID        uint   `json:"id"`
	Nome      string `json:"nome"`
	Sobrenome string `json:"sobrenome"`
	Email     string `json:"email"`
	Perfil    string `json:"perfil"`
	Bloqueado bool   `json:"bloqueado"`
}

// NovoUsuarioResumoAdmin converte o model no resumo.
func NovoUsuarioResumoAdmin(u *Usuario) *UsuarioResumoAdmin {
	if u == nil {
		return nil
	}
	return &UsuarioResumoAdmin{ID: u.ID, Nome: u.Nome, Sobrenome: u.Sobrenome, Email: u.Email, Perfil: u.Perfil, Bloqueado: u.Banido()}
}

// TransacaoAdminResponse é uma linha do ledger vista pelo admin.
type TransacaoAdminResponse struct {
	ID                uint          `json:"id"`
	IDUsuario         uint          `json:"id_usuario"`
	Tipo              TipoTransacao `json:"tipo"`
	ValorCentavos     int64         `json:"valor_centavos"`
	Status            string        `json:"status"`
	IDDesafio         *uint         `json:"id_desafio,omitempty"`
	IDTransacaoOrigem *uint         `json:"id_transacao_origem,omitempty"`
	AsaasPaymentID    *string       `json:"asaas_payment_id,omitempty"`
	AsaasTransferID   *string       `json:"asaas_transfer_id,omitempty"`
	Descricao         *string       `json:"descricao,omitempty"`
	EmRevisao         bool          `json:"em_revisao"`
	RevisaoMotivo     *string       `json:"revisao_motivo,omitempty"`
	CriadoEm          time.Time     `json:"criado_em"`
	AtualizadoEm      time.Time     `json:"atualizado_em"`
}

// NovaTransacaoAdmin converte o model no DTO.
func NovaTransacaoAdmin(t *Transacao) TransacaoAdminResponse {
	return TransacaoAdminResponse{ID: t.ID, IDUsuario: t.IDUsuario, Tipo: t.Tipo, ValorCentavos: t.ValorCentavos,
		Status: t.Status, IDDesafio: t.IDDesafio, IDTransacaoOrigem: t.IDTransacaoOrigem, AsaasPaymentID: t.AsaasPaymentID,
		AsaasTransferID: t.AsaasTransferID, Descricao: t.Descricao, EmRevisao: t.EmRevisao, RevisaoMotivo: t.RevisaoMotivo,
		CriadoEm: t.CreatedAt, AtualizadoEm: t.UpdatedAt}
}

// TransacaoDetalheAdminResponse é GET /api/admin/transacoes/:id.
type TransacaoDetalheAdminResponse struct {
	Transacao TransacaoAdminResponse `json:"transacao"`
	Usuario   *UsuarioResumoAdmin    `json:"usuario,omitempty"`
	// Origem é a linha que originou esta (ex.: o saque de uma tarifa_saque).
	Origem *TransacaoAdminResponse `json:"origem,omitempty"`
	// Derivadas são as linhas originadas por esta (ex.: a tarifa de um saque).
	Derivadas []TransacaoAdminResponse `json:"derivadas"`
}

// SaqueAdminResponse é um saque com a tarifa cobrada, o usuário e a referência enviada ao Asaas.
type SaqueAdminResponse struct {
	TransacaoAdminResponse
	// ValorSaqueCentavos é o valor transferido (positivo); TarifaCentavos a tarifa ainda cobrada (0 se devolvida).
	ValorSaqueCentavos int64               `json:"valor_saque_centavos"`
	TarifaCentavos     int64               `json:"tarifa_centavos"`
	ReferenciaAsaas    string              `json:"referencia_asaas"` // externalReference "saque-<id>"
	Usuario            *UsuarioResumoAdmin `json:"usuario,omitempty"`
}

// SaqueAcaoResponse é a resposta de reconciliar/reembolsar/marcar-pago.
type SaqueAcaoResponse struct {
	// Alterado=false: o saque já estava no estado pedido (idempotente; sem auditoria).
	Alterado bool `json:"alterado"`
	// StatusAsaas é o status consultado no Asaas (vazio quando não houve consulta).
	StatusAsaas string             `json:"status_asaas,omitempty"`
	Saque       SaqueAdminResponse `json:"saque"`
}

// UsuarioAdminResponse é um usuário visto pelo admin (CPF mascarado, nunca senha).
type UsuarioAdminResponse struct {
	ID              uint       `json:"id"`
	Nome            string     `json:"nome"`
	Sobrenome       string     `json:"sobrenome"`
	Email           string     `json:"email"`
	CPFMascarado    string     `json:"cpf_mascarado,omitempty"`
	Perfil          string     `json:"perfil"`
	Username        string     `json:"username,omitempty"`
	SaldoCentavos   int64      `json:"saldo_centavos"`
	Bloqueado       bool       `json:"bloqueado"`
	BloqueadoMotivo *string    `json:"bloqueado_motivo,omitempty"`
	BloqueadoEm     *time.Time `json:"bloqueado_em,omitempty"`
	// Estado efetivo da moderação (bloqueado* acima = banimento, mantidos por compatibilidade do painel).
	BanidoEm       *time.Time `json:"banido_em,omitempty"`
	SuspensoAte    *time.Time `json:"suspenso_ate,omitempty"`
	Suspenso       bool       `json:"suspenso"`
	MotivoBloqueio *string    `json:"motivo_bloqueio,omitempty"`
	UltimaVez      *time.Time `json:"ultima_vez,omitempty"`
	CriadoEm       time.Time  `json:"criado_em"`
}

// UsuarioDetalheAdminResponse é GET /api/admin/usuarios/:id.
type UsuarioDetalheAdminResponse struct {
	Usuario UsuarioAdminResponse `json:"usuario"`
	// SomaLedgerCentavos é a soma do ledger efetivo; LedgerConfere = (soma == saldo).
	SomaLedgerCentavos int64                               `json:"soma_ledger_centavos"`
	LedgerConfere      bool                                `json:"ledger_confere"`
	Ledger             PaginaAdmin[TransacaoAdminResponse] `json:"ledger"`
	// Desafios em que o usuário está inscrito (até 100, mais recentes primeiro).
	Desafios  []ParticipacaoAdminResponse `json:"desafios"`
	Academia  *CadastroAdminResponse      `json:"academia,omitempty"`
	Instrutor *CadastroAdminResponse      `json:"instrutor,omitempty"`
}

// ParticipacaoAdminResponse é a inscrição do usuário num desafio.
type ParticipacaoAdminResponse struct {
	IDDesafio      uint          `json:"id_desafio"`
	Titulo         string        `json:"titulo"`
	Status         StatusDesafio `json:"status"`
	DataEvento     *time.Time    `json:"data_evento,omitempty"`
	Presente       bool          `json:"presente"`
	PremioCentavos int64         `json:"premio_centavos"`
	Vencedor       bool          `json:"vencedor"`
}

// DesafioResumoAdmin é a linha da listagem de desafios do admin.
type DesafioResumoAdmin struct {
	ID                   uint          `json:"id"`
	Titulo               string        `json:"titulo"`
	Status               StatusDesafio `json:"status"`
	Categoria            string        `json:"categoria"`
	DataEvento           *time.Time    `json:"data_evento,omitempty"`
	DataFimEvento        *time.Time    `json:"data_fim_evento,omitempty"` // rodada 5 (null em legados)
	ValorEntradaCentavos int64         `json:"valor_entrada_centavos"`
	Vagas                int           `json:"vagas"`
	IDCriador            uint          `json:"id_criador"`
	IDAcademia           *uint         `json:"id_academia,omitempty"`
	NomeAcademia         string        `json:"nome_academia,omitempty"`
	// IDInstrutor é o RESPONSÁVEL da equipe (espelho legado desafios.id_instrutor).
	IDInstrutor             *uint `json:"id_instrutor,omitempty"`
	InstrutoresVagas        int   `json:"instrutores_vagas"`
	ReservaAcademiaCentavos int64 `json:"reserva_academia_centavos"`
	// Vencido: aberto/em andamento com data_evento + carência no passado (o job vai cancelar).
	Vencido  bool      `json:"vencido"`
	CriadoEm time.Time `json:"criado_em"`
	// Janela de contestação / anulação: podio_lancado_em nulo em desafio encerrado = encerrado antes da
	// janela (a anulação não se aplica). pode_anular_resultado: encerrado, com pódio na janela e sem anulação.
	PodioLancadoEm        *time.Time `json:"podio_lancado_em,omitempty"`
	ResultadoAnuladoEm    *time.Time `json:"resultado_anulado_em,omitempty"`
	ResultadoAnulacaoModo *string    `json:"resultado_anulacao_modo,omitempty"`
	PodeAnularResultado   bool       `json:"pode_anular_resultado"`
}

// FinanceiroDesafioAdmin é a configuração financeira do desafio e os totais do ledger.
type FinanceiroDesafioAdmin struct {
	PremioFixoCentavos     int64  `json:"premio_fixo_centavos"`
	DividirPote            bool   `json:"dividir_pote"`
	PercentualPote         int    `json:"percentual_pote"`
	TaxaPlataformaPct      int    `json:"taxa_plataforma_pct"`
	InstrutorModoPagamento string `json:"instrutor_modo_pagamento"`
	InstrutorFixoCentavos  int64  `json:"instrutor_fixo_centavos"`
	InstrutorPct           int    `json:"instrutor_pct"`
	// InstrutoresVagas: tamanho da equipe; fixo e % acima são POR instrutor (rodada 5).
	InstrutoresVagas int `json:"instrutores_vagas"`
	// LEGADO (leilão até a 00020): só desafios antigos têm vaga_aberta/teto.
	InstrutorSelecao      string `json:"instrutor_selecao,omitempty"`
	InstrutorTetoCentavos *int64 `json:"instrutor_teto_centavos,omitempty"`
	// TotaisPorTipo soma o ledger efetivo do desafio por tipo (centavos assinados).
	TotaisPorTipo map[string]int64 `json:"totais_por_tipo"`
	// ReceitaPlataformaCentavos é a soma de plataforma_receitas do desafio.
	ReceitaPlataformaCentavos int64 `json:"receita_plataforma_centavos"`
}

// ParticipanteAdminResponse é um inscrito com a presença (quem marcou e como).
type ParticipanteAdminResponse struct {
	IDUsuario          uint          `json:"id_usuario"`
	Nome               string        `json:"nome"`
	Sobrenome          string        `json:"sobrenome"`
	Email              string        `json:"email"`
	Perfil             string        `json:"perfil"`
	Presente           bool          `json:"presente"`
	PresenteEm         *time.Time    `json:"presente_em,omitempty"`
	PresencaModo       *ModoPresenca `json:"presenca_modo,omitempty"` // qr | manual (null em presenças antigas)
	PresencaMarcadaPor *uint         `json:"presenca_marcada_por,omitempty"`
	PremioCentavos     int64         `json:"premio_centavos"`
	Vencedor           bool          `json:"vencedor"`
	InscritoEm         time.Time     `json:"inscrito_em"`
}

// CandidaturaAdminResponse é uma candidatura à vaga de instrutor do desafio.
type CandidaturaAdminResponse struct {
	ID            uint      `json:"id"`
	IDInstrutor   uint      `json:"id_instrutor"`
	NomeInstrutor string    `json:"nome_instrutor"`
	LanceCentavos *int64    `json:"lance_centavos,omitempty"` // LEGADO: só candidaturas do antigo leilão
	Status        string    `json:"status"`
	Mensagem      *string   `json:"mensagem,omitempty"`
	CriadoEm      time.Time `json:"criado_em"`
}

// DesafioDetalheAdminResponse é GET /api/admin/desafios/:id.
type DesafioDetalheAdminResponse struct {
	Desafio DesafioResumoAdmin `json:"desafio"`
	// Rodada 5: local (texto), endereço estruturado (null em antigos) e a equipe de instrutores.
	Local    string                 `json:"local"`
	Endereco *EnderecoResponse      `json:"endereco"`
	Equipe   []MembroEquipeResponse `json:"equipe"`
	// Rodada 6: resumo/regras, nº de premiados + divisão do prêmio (pontos-base) e requisitos de entrada.
	Resumo          string                      `json:"resumo"`
	Regras          string                      `json:"regras"`
	PosicoesPodio   int                         `json:"posicoes_podio"`
	DivisaoPremioBP []int                       `json:"divisao_premio_bp"`
	NivelMin        *int                        `json:"nivel_min"`
	NivelMax        *int                        `json:"nivel_max"`
	IdadeMin        *int                        `json:"idade_min"`
	IdadeMax        *int                        `json:"idade_max"`
	Financeiro      FinanceiroDesafioAdmin      `json:"financeiro"`
	Participantes   []ParticipanteAdminResponse `json:"participantes"`
	Candidaturas    []CandidaturaAdminResponse  `json:"candidaturas"`
	Podio           []PodioResponse             `json:"podio"`
	// Ledger do desafio (até 100 linhas mais recentes; LedgerTotal = total de linhas).
	Ledger      []TransacaoAdminResponse `json:"ledger"`
	LedgerTotal int64                    `json:"ledger_total"`
}

// CadastroAdminResponse é uma academia ou um instrutor na fila de aprovação.
type CadastroAdminResponse struct {
	ID             uint                `json:"id"`
	Tipo           string              `json:"tipo"` // academia | instrutor
	Status         string              `json:"status"`
	MotivoRejeicao *string             `json:"motivo_rejeicao,omitempty"`
	AprovadoEm     *time.Time          `json:"aprovado_em,omitempty"`
	CriadoEm       time.Time           `json:"criado_em"`
	Usuario        *UsuarioResumoAdmin `json:"usuario,omitempty"`
	// Academia
	NomeFantasia string `json:"nome_fantasia,omitempty"`
	RazaoSocial  string `json:"razao_social,omitempty"`
	CNPJ         string `json:"cnpj,omitempty"`
	Telefone     string `json:"telefone,omitempty"`
	// Endereço da academia (rodada 4, B4-8): `endereco` é o texto formatado (academias antigas têm só ele);
	// os campos estruturados são omitidos (NULL) até a academia cadastrar/editar o endereço novo.
	Endereco    string  `json:"endereco,omitempty"`
	CEP         *string `json:"cep,omitempty"`
	Logradouro  *string `json:"logradouro,omitempty"`
	Numero      *string `json:"numero,omitempty"`
	Complemento *string `json:"complemento,omitempty"`
	Bairro      *string `json:"bairro,omitempty"`
	Cidade      *string `json:"cidade,omitempty"`
	UF          *string `json:"uf,omitempty"`
	// Instrutor
	CREF          string `json:"cref,omitempty"`
	Especialidade string `json:"especialidade,omitempty"`
	IDAcademia    *uint  `json:"id_academia,omitempty"`
}

// NovoCadastroAcademia / NovoCadastroInstrutor convertem os models no DTO de aprovação.
func NovoCadastroAcademia(a *Academia) CadastroAdminResponse {
	return CadastroAdminResponse{ID: a.ID, Tipo: "academia", Status: a.Status, MotivoRejeicao: a.MotivoRejeicao,
		AprovadoEm: a.AprovadoEm, CriadoEm: a.CriadoEm, Usuario: NovoUsuarioResumoAdmin(a.Usuario),
		NomeFantasia: a.NomeFantasia, RazaoSocial: a.RazaoSocial, CNPJ: a.CNPJ, Telefone: a.Telefone,
		Endereco: a.Endereco, CEP: a.CEP, Logradouro: a.Logradouro, Numero: a.Numero, Complemento: a.Complemento,
		Bairro: a.Bairro, Cidade: a.Cidade, UF: a.UF}
}

// NovoCadastroInstrutor converte o instrutor no DTO de aprovação.
func NovoCadastroInstrutor(i *Instrutor) CadastroAdminResponse {
	return CadastroAdminResponse{ID: i.ID, Tipo: "instrutor", Status: i.Status, MotivoRejeicao: i.MotivoRejeicao,
		AprovadoEm: i.AprovadoEm, CriadoEm: i.CriadoEm, Usuario: NovoUsuarioResumoAdmin(i.Usuario),
		CREF: i.CREF, Especialidade: i.Especialidade, IDAcademia: i.IDAcademia}
}

// ReceitaAdminResponse é uma linha de plataforma_receitas.
type ReceitaAdminResponse struct {
	ID            uint      `json:"id"`
	Origem        string    `json:"origem"` // taxa_inscricao | tarifa_saque
	ValorCentavos int64     `json:"valor_centavos"`
	IDDesafio     *uint     `json:"id_desafio,omitempty"`
	IDTransacao   *uint     `json:"id_transacao,omitempty"`
	CriadoEm      time.Time `json:"criado_em"`
}

// ReceitasAdminResponse é GET /api/admin/receitas: a página e os totais por origem do período.
type ReceitasAdminResponse struct {
	PaginaAdmin[ReceitaAdminResponse]
	TotaisPorOrigem map[string]int64 `json:"totais_por_origem"`
	TotalCentavos   int64            `json:"total_centavos"`
}

// ReservaAdminResponse é a reserva (garantia da academia) de um desafio.
type ReservaAdminResponse struct {
	IDDesafio       uint          `json:"id_desafio"`
	Titulo          string        `json:"titulo"`
	Status          StatusDesafio `json:"status"`
	IDAcademia      *uint         `json:"id_academia,omitempty"`
	IDCriador       uint          `json:"id_criador"`
	ReservaCentavos int64         `json:"reserva_centavos"` // valor vigente na coluna do desafio
	RetidoCentavos  int64         `json:"retido_centavos"`  // líquido ainda retido no ledger
}

// AuditoriaAdminResponse é uma linha de admin_auditoria.
type AuditoriaAdminResponse struct {
	ID         uint            `json:"id"`
	IDAdmin    uint            `json:"id_admin"`
	AdminNome  string          `json:"admin_nome,omitempty"`
	AdminEmail string          `json:"admin_email,omitempty"`
	Acao       string          `json:"acao"`
	AlvoTipo   string          `json:"alvo_tipo"`
	AlvoID     *uint           `json:"alvo_id,omitempty"`
	Motivo     string          `json:"motivo"`
	Detalhes   json.RawMessage `json:"detalhes,omitempty" swaggertype:"object"`
	IP         string          `json:"ip"`
	CriadoEm   time.Time       `json:"criado_em"`
}

// DashboardAdminResponse é GET /api/admin/dashboard (valores em centavos).
type DashboardAdminResponse struct {
	Financeiro  FinanceiroDashboardAdmin  `json:"financeiro"`
	Conferencia ConferenciaDashboardAdmin `json:"conferencia"`
	Receita     ReceitaDashboardAdmin     `json:"receita"`
	Pendencias  PendenciasDashboardAdmin  `json:"pendencias"`
	// DesafiosPorStatus conta todos os desafios por status.
	DesafiosPorStatus map[string]int64 `json:"desafios_por_status"`
}

// FinanceiroDashboardAdmin são os totais da plataforma.
type FinanceiroDashboardAdmin struct {
	SaldoCarteirasCentavos   int64 `json:"saldo_carteiras_centavos"`   // Σ usuarios.saldo_centavos
	SomaLedgerCentavos       int64 `json:"soma_ledger_centavos"`       // ledger efetivo (deve = saldo)
	DepositosCentavos        int64 `json:"depositos_centavos"`         // depósitos recebidos
	SaquesCentavos           int64 `json:"saques_centavos"`            // saques pending + concluido
	AjustesCentavos          int64 `json:"ajustes_centavos"`           // linhas de abertura legadas
	CustodiaEntradasCentavos int64 `json:"custodia_entradas_centavos"` // entradas em desafios ativos
	CustodiaReservasCentavos int64 `json:"custodia_reservas_centavos"` // reservas em desafios ativos
	CustodiaTotalCentavos    int64 `json:"custodia_total_centavos"`
	ReceitasCentavos         int64 `json:"receitas_centavos"` // Σ plataforma_receitas (todo o período, líquida)
	// Moderação: créditos do pódio ainda na janela (fora dos saldos) e débitos em aberto ("a receber").
	PendentesUsuariosCentavos   int64 `json:"pendentes_usuarios_centavos"`
	PendentesPlataformaCentavos int64 `json:"pendentes_plataforma_centavos"`
	DebitosAbertosCentavos      int64 `json:"debitos_abertos_centavos"`
}

// ConferenciaDashboardAdmin: Σ saldos + custódia + receitas + pendentes − débitos abertos =
// Σ depósitos − Σ saques (+ ajustes).
type ConferenciaDashboardAdmin struct {
	LadoAplicacoesCentavos int64 `json:"lado_aplicacoes_centavos"` // saldos + custódia + receitas + pendentes − débitos
	LadoOrigensCentavos    int64 `json:"lado_origens_centavos"`    // depósitos − saques + ajustes
	DivergenciaCentavos    int64 `json:"divergencia_centavos"`     // aplicações − origens (0 = confere)
	Confere                bool  `json:"confere"`
	// SaldosVsLedgerCentavos = Σ saldos − soma do ledger (0 = confere).
	SaldosVsLedgerCentavos int64 `json:"saldos_vs_ledger_centavos"`
	SaldosConferem         bool  `json:"saldos_conferem"`
}

// ReceitaDashboardAdmin é a receita da plataforma por origem no período pedido.
type ReceitaDashboardAdmin struct {
	De            *time.Time       `json:"de,omitempty"`
	Ate           *time.Time       `json:"ate,omitempty"`
	PorOrigem     map[string]int64 `json:"por_origem"`
	TotalCentavos int64            `json:"total_centavos"`
}

// PendenciasDashboardAdmin são as filas que pedem ação do admin.
type PendenciasDashboardAdmin struct {
	SaquesPendentes      int64 `json:"saques_pendentes"`
	SaquesEmRevisao      int64 `json:"saques_em_revisao"`
	DesafiosVencidos     int64 `json:"desafios_vencidos"`
	AcademiasPendentes   int64 `json:"academias_pendentes"`
	InstrutoresPendentes int64 `json:"instrutores_pendentes"`
	// Moderação: denúncias na fila do admin (das quais escalonadas pela academia), denúncias ainda com a
	// academia julgadora (acompanhamento), recursos a julgar, pagamentos do pódio retidos por denúncia e
	// débitos de usuários em aberto (quantidades).
	DenunciasAdmin       int64 `json:"denuncias_admin"`
	DenunciasEscalonadas int64 `json:"denuncias_escalonadas"`
	DenunciasAcademia    int64 `json:"denuncias_academia"`
	RecursosPendentes    int64 `json:"recursos_pendentes"`
	PagamentosRetidos    int64 `json:"pagamentos_retidos"`
	DebitosAbertos       int64 `json:"debitos_abertos"`
}
