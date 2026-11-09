package models

import "time"

// StatusDesafio define os possíveis estados de um desafio
type StatusDesafio string

const (
	StatusAberto      StatusDesafio = "aberto"
	StatusEmAndamento StatusDesafio = "em_andamento"
	StatusEncerrado   StatusDesafio = "encerrado"
	StatusCancelado   StatusDesafio = "cancelado"
)

// Limites da criação de desafio (valores em centavos; 0 = gratuito).
const (
	EntradaMaxCentavos int64 = 1_000_000 // R$ 10.000,00
	VagasMin                 = 2
	VagasMax                 = 500
	PosicoesPodioMin         = 1
	PosicoesPodioMax         = 5
	// MarcaMaxCaracteres é o limite da marca (tempo/carga/reps) de cada posição do pódio.
	MarcaMaxCaracteres = 50
)

// CategoriaPadrao é a categoria usada quando o corpo não informa uma.
const CategoriaPadrao = "geral"

// CategoriasDesafio são as categorias aceitas (chips do feed), na ordem de exibição.
var CategoriasDesafio = []string{
	"geral", "corrida", "musculacao", "crossfit", "funcional", "ciclismo", "natacao", "calistenia", "luta", "outros",
}

// CategoriaValida indica se c é uma categoria conhecida.
func CategoriaValida(c string) bool {
	for _, v := range CategoriasDesafio {
		if v == c {
			return true
		}
	}
	return false
}

// Desafio representa a entidade de desafio no sistema GymScore.
// Só a academia cria desafios (IDCriador = usuário da academia); IDAcademia/IDInstrutor/DataEvento
// são NULL apenas em desafios legados (anteriores ao Sprint 6).
type Desafio struct {
	ID                   uint          `gorm:"primaryKey;autoIncrement"`
	Titulo               string        `gorm:"type:varchar(200);not null"`
	Descricao            *string       `gorm:"type:text"`
	Categoria            string        `gorm:"column:categoria;type:varchar(30);not null;default:geral"`
	ValorEntradaCentavos int64         `gorm:"column:valor_entrada_centavos;not null"`
	Local                *string       `gorm:"type:varchar(200)"`
	DataEvento           *time.Time    `gorm:"column:data_evento"`
	Latitude             *float64      `gorm:"type:decimal(10,8)"`
	Longitude            *float64      `gorm:"type:decimal(11,8)"`
	Status               StatusDesafio `gorm:"type:enum('aberto','em_andamento','encerrado','cancelado');default:'aberto'"`
	Vagas                int           `gorm:"column:vagas;not null;default:2"`
	PosicoesPodio        int           `gorm:"column:posicoes_podio;not null;default:3"`
	DataEncerramento     *time.Time    `gorm:"column:data_encerramento"`
	IDCriador            uint          `gorm:"not null"`
	IDAcademia           *uint         `gorm:"column:id_academia"`
	IDInstrutor          *uint         `gorm:"column:id_instrutor"`

	// Financeiro (migration 00012). taxa_plataforma_pct é o snapshot do .env na criação.
	// ReservaAcademiaCentavos é a garantia vigente debitada da academia (premio_fixo + instrutor_fixo,
	// ou premio_fixo + teto enquanto a vaga está aberta); não é zerada no encerramento nem no cancelamento.
	PremioFixoCentavos      int64  `gorm:"column:premio_fixo_centavos;not null;default:0"`
	DividirPote             bool   `gorm:"column:dividir_pote;not null"`
	PercentualPote          int    `gorm:"column:percentual_pote;not null"`
	TaxaPlataformaPct       int    `gorm:"column:taxa_plataforma_pct;not null;default:0"`
	InstrutorModoPagamento  string `gorm:"column:instrutor_modo_pagamento;not null;default:fixo"`
	InstrutorFixoCentavos   int64  `gorm:"column:instrutor_fixo_centavos;not null;default:0"`
	InstrutorPct            int    `gorm:"column:instrutor_pct;not null;default:0"`
	InstrutorSelecao        string `gorm:"column:instrutor_selecao;not null;default:designado"`
	InstrutorTetoCentavos   *int64 `gorm:"column:instrutor_teto_centavos"`
	ReservaAcademiaCentavos int64  `gorm:"column:reserva_academia_centavos;not null;default:0"`
	// Lançamento do pódio e anulação do resultado (moderação, migration 00013). Somente leitura para o
	// GORM: gravados por UPDATE condicional (CAS do pódio / anulação) nos repositórios.
	PodioLancadoPor       *uint      `gorm:"column:podio_lancado_por;->"`
	PodioLancadoEm        *time.Time `gorm:"column:podio_lancado_em;->"`
	ResultadoAnuladoEm    *time.Time `gorm:"column:resultado_anulado_em;->"`
	ResultadoAnulacaoModo *string    `gorm:"column:resultado_anulacao_modo;->"`

	CriadoEm     time.Time `gorm:"autoCreateTime"`
	AtualizadoEm time.Time `gorm:"autoUpdateTime"`

	// Relacionamentos
	Criador       *Usuario              `gorm:"foreignKey:IDCriador"`
	Academia      *Academia             `gorm:"foreignKey:IDAcademia"`
	Instrutor     *Instrutor            `gorm:"foreignKey:IDInstrutor"`
	Participantes []DesafioParticipante `gorm:"foreignKey:IDDesafio"`

	// Preenchidos por DesafioRepository.Listar (não são colunas): contagens e a inscrição do
	// usuário que consulta. Em BuscarDetalhe o service os deriva de Participantes.
	QtdParticipantes  int                  `gorm:"-"`
	QtdPresentes      int                  `gorm:"-"`
	MinhaParticipacao *DesafioParticipante `gorm:"-"`
	// Preenchidos pelo service (vaga de instrutor): candidaturas pendentes (visão da academia dona)
	// e a candidatura do instrutor que consulta.
	QtdCandidaturas  int                   `gorm:"-"`
	MinhaCandidatura *CandidaturaInstrutor `gorm:"-"`
}

// TableName define o nome da tabela no banco de dados
func (Desafio) TableName() string {
	return "desafios"
}

// DesafioParticipante representa um usuário inscrito em um desafio (ocupando uma vaga).
// CodigoIngresso é o token do QR do ingresso: sempre gerado pela aplicação (crypto/rand) no INSERT;
// string vazia NÃO aciona o DEFAULT do banco e violaria a unicidade.
type DesafioParticipante struct {
	ID             uint   `gorm:"primaryKey;autoIncrement"`
	IDDesafio      uint   `gorm:"column:id_desafio;not null;uniqueIndex:uq_desafio_usuario"`
	IDUsuario      uint   `gorm:"column:id_usuario;not null;uniqueIndex:uq_desafio_usuario"`
	CodigoIngresso string `gorm:"column:codigo_ingresso;type:char(32);not null;uniqueIndex:uq_participante_codigo_ingresso"`
	PremioCentavos int64  `gorm:"column:premio_centavos;not null;default:0"`
	Vencedor       bool   `gorm:"column:vencedor;not null;default:false"`
	// Presente/PresenteEm: presença confirmada pelo instrutor/academia (lendo o QR ou pela lista).
	// As duas colunas são mantidas coerentes (presente=1 <=> presente_em preenchido).
	Presente   bool       `gorm:"column:presente;not null;default:false"`
	PresenteEm *time.Time `gorm:"column:presente_em"`
	// Quem (staff) e como confirmou a presença (migration 00018). NULL em presenças antigas.
	PresencaMarcadaPor *uint         `gorm:"column:presenca_marcada_por"`
	PresencaModo       *ModoPresenca `gorm:"column:presenca_modo;type:enum('qr','manual')"`
	CriadoEm           time.Time     `gorm:"autoCreateTime"`

	Usuario *Usuario `gorm:"foreignKey:IDUsuario"`
}

// ModoPresenca indica como a presença foi confirmada.
type ModoPresenca string

const (
	PresencaQR     ModoPresenca = "qr"     // leitura do QR do ingresso
	PresencaManual ModoPresenca = "manual" // marcada pela lista de inscritos
)

// TableName define o nome da tabela de participantes
func (DesafioParticipante) TableName() string {
	return "desafio_participantes"
}

// ─── DTOs de entrada ─────────────────────────────────────────────────────────

// CriarDesafioRequest é o DTO de entrada para criação de desafio (só perfil academia).
// A academia dona é sempre a do JWT; ela não vira participante nem paga entrada.
type CriarDesafioRequest struct {
	Titulo    string   `json:"titulo" validate:"required,min=3,max=200"`
	Descricao string   `json:"descricao" validate:"max=2000"`
	Categoria string   `json:"categoria" validate:"max=30"`
	Local     string   `json:"local" validate:"required,max=200"`
	Latitude  *float64 `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude *float64 `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	// DataEvento aceita RFC3339 ("2026-11-20T09:00:00-03:00") ou "YYYY-MM-DDTHH:MM" / "YYYY-MM-DD HH:MM"
	// (horário local do servidor). Precisa estar no futuro.
	DataEvento           string `json:"data_evento" validate:"required"`
	Vagas                int    `json:"vagas" validate:"required,gte=2,lte=500"`
	ValorEntradaCentavos int64  `json:"valor_entrada_centavos" validate:"gte=0,lte=1000000"`
	PosicoesPodio        int    `json:"posicoes_podio" validate:"required,gte=1,lte=5"`

	// Premiação: prêmio fixo (reservado do saldo da academia) + parcela das inscrições (percentual_pote
	// de T, se dividir_pote). dividir_pote omitido = true; percentual_pote omitido = o máximo permitido
	// (100 - taxa da plataforma - instrutor_pct). Com dividir_pote=false, percentual_pote é gravado 0.
	PremioFixoCentavos int64 `json:"premio_fixo_centavos" validate:"gte=0,lte=10000000"`
	DividirPote        *bool `json:"dividir_pote"`
	PercentualPote     *int  `json:"percentual_pote" validate:"omitempty,gte=0,lte=100"`

	// Instrutor: instrutor_selecao "designado" (padrão; exige id_instrutor vinculado) ou "vaga_aberta"
	// (sem id_instrutor; exige instrutor_teto_centavos; instrutores se candidatam com um lance <= teto).
	// instrutor_modo_pagamento: fixo (padrão) | percentual | fixo_percentual.
	IDInstrutor            uint   `json:"id_instrutor"`
	InstrutorSelecao       string `json:"instrutor_selecao" validate:"omitempty,oneof=designado vaga_aberta"`
	InstrutorModoPagamento string `json:"instrutor_modo_pagamento" validate:"omitempty,oneof=fixo percentual fixo_percentual"`
	InstrutorFixoCentavos  int64  `json:"instrutor_fixo_centavos" validate:"gte=0,lte=10000000"`
	InstrutorPct           int    `json:"instrutor_pct" validate:"gte=0,lte=100"`
	InstrutorTetoCentavos  *int64 `json:"instrutor_teto_centavos" validate:"omitempty,gte=0,lte=10000000"`
}

// EditarDesafioRequest é o DTO do PATCH /desafios/:id: todos os campos são opcionais
// (ponteiro nil = não altera). Descricao "" limpa a descrição.
type EditarDesafioRequest struct {
	Titulo               *string  `json:"titulo" validate:"omitempty,min=3,max=200"`
	Descricao            *string  `json:"descricao" validate:"omitempty,max=2000"`
	Categoria            *string  `json:"categoria" validate:"omitempty,max=30"`
	Local                *string  `json:"local" validate:"omitempty,min=1,max=200"`
	Latitude             *float64 `json:"latitude" validate:"omitempty,gte=-90,lte=90"`
	Longitude            *float64 `json:"longitude" validate:"omitempty,gte=-180,lte=180"`
	DataEvento           *string  `json:"data_evento"`
	Vagas                *int     `json:"vagas" validate:"omitempty,gte=2,lte=500"`
	ValorEntradaCentavos *int64   `json:"valor_entrada_centavos" validate:"omitempty,gte=0,lte=1000000"`
	PosicoesPodio        *int     `json:"posicoes_podio" validate:"omitempty,gte=1,lte=5"`
	IDInstrutor          *uint    `json:"id_instrutor" validate:"omitempty,gte=1"`

	// Financeiro (mesmas regras da criação; a taxa da plataforma fica a do snapshot). Com atletas
	// inscritos o prêmio só pode aumentar. Depois de escolhido o instrutor da vaga aberta, os campos
	// instrutor_* ficam travados.
	PremioFixoCentavos     *int64  `json:"premio_fixo_centavos" validate:"omitempty,gte=0,lte=10000000"`
	DividirPote            *bool   `json:"dividir_pote"`
	PercentualPote         *int    `json:"percentual_pote" validate:"omitempty,gte=0,lte=100"`
	InstrutorModoPagamento *string `json:"instrutor_modo_pagamento" validate:"omitempty,oneof=fixo percentual fixo_percentual"`
	InstrutorFixoCentavos  *int64  `json:"instrutor_fixo_centavos" validate:"omitempty,gte=0,lte=10000000"`
	InstrutorPct           *int    `json:"instrutor_pct" validate:"omitempty,gte=0,lte=100"`
	InstrutorTetoCentavos  *int64  `json:"instrutor_teto_centavos" validate:"omitempty,gte=0,lte=10000000"`
}

// PresencaRequest identifica o atleta que está presente: pelo código do QR do ingresso
// OU pelo id do usuário (marcação na lista). Exatamente um dos dois.
type PresencaRequest struct {
	CodigoIngresso string `json:"codigo_ingresso" validate:"omitempty,max=64"`
	IDUsuario      uint   `json:"id_usuario"`
}

// ConvidarRequest é o DTO do convite: id_amigo é o ALVO (amigo aceito de quem convida).
type ConvidarRequest struct {
	IDAmigo uint `json:"id_amigo" validate:"required"`
}

// ─── DTOs de saída ───────────────────────────────────────────────────────────

// AcademiaResumo identifica a academia dona do desafio.
type AcademiaResumo struct {
	ID           uint   `json:"id"`
	NomeFantasia string `json:"nome_fantasia"`
}

// InstrutorResumo identifica o instrutor designado.
type InstrutorResumo struct {
	ID        uint   `json:"id"`         // instrutores.id
	IDUsuario uint   `json:"id_usuario"` // usuarios.id do instrutor (ex.: para denunciá-lo pelo desafio)
	Nome      string `json:"nome"`
}

// ParticipanteResponse é um participante no detalhe do desafio.
type ParticipanteResponse struct {
	UsuarioPublico
	Presente   bool       `json:"presente"`
	PresenteEm *time.Time `json:"presente_em,omitempty"`
	// PresencaModo: "qr" (leitura do ingresso) ou "manual"; ausente em presenças antigas.
	PresencaModo   *ModoPresenca `json:"presenca_modo,omitempty"`
	PremioCentavos int64         `json:"premio_centavos"`
	Premio         float64       `json:"premio"`
	Vencedor       bool          `json:"vencedor"`
}

// PodioResponse é uma posição do pódio de um desafio encerrado.
type PodioResponse struct {
	Posicao        int     `json:"posicao"`
	IDParticipante uint    `json:"id_participante"`
	Nome           string  `json:"nome"`
	Sobrenome      string  `json:"sobrenome"`
	Marca          string  `json:"marca"`
	PremioCentavos int64   `json:"premio_centavos"`
	Premio         float64 `json:"premio"`
}

// PresencaResponse é a resposta da marcação de presença (o instrutor confirma o nome do atleta).
type PresencaResponse struct {
	IDUsuario          uint       `json:"id_usuario"`
	Nome               string     `json:"nome"`
	Sobrenome          string     `json:"sobrenome"`
	Presente           bool       `json:"presente"`
	PresenteEm         *time.Time `json:"presente_em"`
	JaPresente         bool       `json:"ja_presente"` // true = já estava presente (chamada idempotente)
	PresentesCount     int        `json:"presentes_count"`
	ParticipantesCount int        `json:"participantes_count"`
}

// DesafioResponse é o DTO de desafio devolvido pela API.
// Em listas (feed, meus) `participantes` e `podio` vêm vazios: use GET /desafios/:id.
type DesafioResponse struct {
	ID                   uint             `json:"id"`
	Titulo               string           `json:"titulo"`
	Descricao            string           `json:"descricao"`
	Categoria            string           `json:"categoria"`
	ValorEntradaCentavos int64            `json:"valor_entrada_centavos"`
	ValorEntrada         float64          `json:"valor_entrada"`
	PoteCentavos         int64            `json:"pote_centavos"`
	Pote                 float64          `json:"pote"`
	Local                string           `json:"local"`
	Latitude             *float64         `json:"latitude,omitempty"`
	Longitude            *float64         `json:"longitude,omitempty"`
	DataEvento           *time.Time       `json:"data_evento"` // null só em desafios legados
	Status               StatusDesafio    `json:"status"`
	Vagas                int              `json:"vagas"`
	PosicoesPodio        int              `json:"posicoes_podio"`
	Academia             *AcademiaResumo  `json:"academia"`  // null só em legados
	Instrutor            *InstrutorResumo `json:"instrutor"` // null se sem instrutor (legados)
	// PremiacaoPrevistaCentavos tem posicoes_podio itens: a divisão do pote ATUAL pela tabela da
	// quantidade de posições (se houver menos presentes que posições, os valores finais são renormalizados).
	PremiacaoPrevistaCentavos []int64 `json:"premiacao_prevista_centavos"`
	// Premiacao (todos): prêmio total previsto + lista por posição. Financeiro só vai para a academia
	// dona, InstrutorGanho para o instrutor designado e Vaga para instrutores (oferta da vaga aberta).
	Premiacao        PremiacaoResponse       `json:"premiacao"`
	Financeiro       *FinanceiroResponse     `json:"financeiro,omitempty"`
	InstrutorGanho   *GanhoInstrutorResponse `json:"instrutor_ganho,omitempty"`
	Vaga             *VagaInstrutorResponse  `json:"vaga,omitempty"`
	InstrutorSelecao string                  `json:"instrutor_selecao"`
	// VagaInstrutorAberta: a vaga de instrutor segue aberta (sem escolhido); o desafio não pode ser iniciado.
	VagaInstrutorAberta bool      `json:"vaga_instrutor_aberta"`
	ParticipantesCount  int       `json:"participantes_count"`
	PresentesCount      int       `json:"presentes_count"`
	CriadoEm            time.Time `json:"criado_em"`

	// Do usuário logado
	VoceParticipa     bool   `json:"voce_participa"`
	VocePresente      bool   `json:"voce_presente"`
	SeuCodigoIngresso string `json:"seu_codigo_ingresso,omitempty"` // só para o próprio atleta inscrito

	// Flags de ação para o usuário logado
	PodeEntrar bool `json:"pode_entrar"`
	// MotivoBloqueioEntrada (rodada 4, B4-8): preenchido quando o atleta NÃO pode entrar por conflito de
	// interesse (é o instrutor designado, instrutor aprovado vinculado à academia do desafio ou tem
	// candidatura ativa à vaga) — mesma mensagem do 409 de POST /entrar. Vazio/omitido nos demais casos.
	MotivoBloqueioEntrada string `json:"motivo_bloqueio_entrada,omitempty"`
	PodeSair              bool   `json:"pode_sair"`
	PodeIniciar           bool   `json:"pode_iniciar"`
	PodeMarcarPresenca    bool   `json:"pode_marcar_presenca"`
	PodeLancarPodio       bool   `json:"pode_lancar_podio"`
	PodeCancelar          bool   `json:"pode_cancelar"`
	PodeEditar            bool   `json:"pode_editar"`
	PodeAvaliar           bool   `json:"pode_avaliar"`
	// PodeCandidatar: instrutor pode dar lance (vaga aberta, desafio aberto, evento futuro).
	// PodeEscolherInstrutor: academia dona pode escolher entre os candidatos.
	PodeCandidatar        bool `json:"pode_candidatar"`
	PodeEscolherInstrutor bool `json:"pode_escolher_instrutor"`

	// Janela de contestação e anulação (moderação). podio_lancado_em = base da liberação;
	// pagamentos_liberam_em (só no detalhe) = quando os créditos do pódio entram nos saldos (null se já
	// liberados ou sem pódio); pagamentos_retidos = há contestação segurando a liberação.
	// resultado_anulado: o resultado foi anulado pelo admin (modo relancar_podio | devolver_inscricoes);
	// aguardando_relancamento: anulado em relancar_podio e de volta a em_andamento (só a academia dona relança).
	PodioLancadoEm         *time.Time `json:"podio_lancado_em,omitempty"`
	PagamentosLiberamEm    *time.Time `json:"pagamentos_liberam_em,omitempty"`
	PagamentosRetidos      bool       `json:"pagamentos_retidos"`
	ResultadoAnulado       bool       `json:"resultado_anulado"`
	ResultadoAnuladoEm     *time.Time `json:"resultado_anulado_em,omitempty"`
	ResultadoAnulacaoModo  *string    `json:"resultado_anulacao_modo,omitempty"`
	AguardandoRelancamento bool       `json:"aguardando_relancamento"`

	Participantes []ParticipanteResponse `json:"participantes"`
	Podio         []PodioResponse        `json:"podio"`
}

// ListaDesafiosResponse é a página de desafios.
type ListaDesafiosResponse struct {
	Itens  []DesafioResponse `json:"itens"`
	Total  int64             `json:"total"`
	Pagina int               `json:"pagina"`
	Limite int               `json:"limite"`
}

// DesafioRecomendado agrupa o DTO público do desafio com a razão textual da recomendação.
// Desafio é *DesafioResponse (não o model Desafio cru) para expor o mesmo contrato snake_case
// usado pelo resto da API de desafios (GET /desafios, GET /desafios/:id, etc.).
type DesafioRecomendado struct {
	Desafio *DesafioResponse `json:"desafio"`
	Razao   string           `json:"razao"`
}
