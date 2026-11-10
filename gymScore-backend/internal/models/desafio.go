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
	PosicoesPodioMax         = PosicoesPodioTeto // 100 desde a 00022 (premiação configurável)
	// MarcaMaxCaracteres é o limite da marca (tempo/carga/reps) de cada posição do pódio.
	MarcaMaxCaracteres = 50
	// Resumo e regras (U2, rodada 6): obrigatórios em desafios novos, contados em caracteres (runas).
	ResumoMinCaracteres = 20
	ResumoMaxCaracteres = 280
	RegrasMinCaracteres = 50
	RegrasMaxCaracteres = 10000
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
	ID        uint    `gorm:"primaryKey;autoIncrement"`
	Titulo    string  `gorm:"type:varchar(200);not null"`
	Descricao *string `gorm:"type:text"` // LEGADO desde a 00022 (substituída por Resumo + Regras)
	// Resumo (até 280) e Regras (até 10000, CHECK) — 00022. Obrigatórios em desafios novos (20–280 / 50–10000,
	// validados na API); legados: derivados de Descricao (Regras pode ser NULL). Texto puro.
	Resumo               *string `gorm:"column:resumo;type:varchar(280)"`
	Regras               *string `gorm:"column:regras;type:text"`
	Categoria            string  `gorm:"column:categoria;type:varchar(30);not null;default:geral"`
	ValorEntradaCentavos int64   `gorm:"column:valor_entrada_centavos;not null"`
	Local                *string `gorm:"type:varchar(500)"` // texto montado do endereço (compat; 00021)
	// Endereço estruturado do evento (00021; NULL em desafios antigos). Mesmos CHECKs de academias.
	CEP           *string       `gorm:"column:cep;type:char(8)"` // só dígitos (CHECK no banco)
	Logradouro    *string       `gorm:"column:logradouro;size:150"`
	Numero        *string       `gorm:"column:numero;size:20"`
	Complemento   *string       `gorm:"column:complemento;size:100"`
	Bairro        *string       `gorm:"column:bairro;size:100"`
	Cidade        *string       `gorm:"column:cidade;size:100"`
	UF            *string       `gorm:"column:uf;type:char(2)"` // CHECK das 27 UFs no banco
	DataEvento    *time.Time    `gorm:"column:data_evento"`
	DataFimEvento *time.Time    `gorm:"column:data_fim_evento"` // 00021: fim do evento (> data_evento); obrigatório em novos (API)
	Latitude      *float64      `gorm:"type:decimal(10,8)"`
	Longitude     *float64      `gorm:"type:decimal(11,8)"`
	Status        StatusDesafio `gorm:"type:enum('aberto','em_andamento','encerrado','cancelado');default:'aberto'"`
	Vagas         int           `gorm:"column:vagas;not null;default:2"`
	PosicoesPodio int           `gorm:"column:posicoes_podio;not null;default:3"` // 1..100 (00022); divisão em PremioPosicoes
	// Filtros de entrada (00022), NULL = sem filtro: nível GymScore 1..10 e idade (na data do evento) 0..120,
	// min <= max (CHECKs no banco). Conferidos no Entrar.
	NivelMin         *int       `gorm:"column:nivel_min"`
	NivelMax         *int       `gorm:"column:nivel_max"`
	IdadeMin         *int       `gorm:"column:idade_min"`
	IdadeMax         *int       `gorm:"column:idade_max"`
	DataEncerramento *time.Time `gorm:"column:data_encerramento"`
	IDCriador        uint       `gorm:"not null"`
	IDAcademia       *uint      `gorm:"column:id_academia"`
	// IDInstrutor é LEGADO desde a 00021: espelho do responsável da equipe (desafio_instrutores),
	// gravado pelo backend na mesma transação que muda o responsável. Código novo lê a equipe.
	IDInstrutor *uint `gorm:"column:id_instrutor"`

	// Financeiro (migration 00012). taxa_plataforma_pct é o snapshot do .env na criação.
	// ReservaAcademiaCentavos é a garantia vigente debitada da academia (premio_fixo + instrutor_fixo,
	// ou premio_fixo + teto enquanto a vaga está aberta); não é zerada no encerramento nem no cancelamento.
	PremioFixoCentavos     int64  `gorm:"column:premio_fixo_centavos;not null;default:0"`
	DividirPote            bool   `gorm:"column:dividir_pote;not null"`
	PercentualPote         int    `gorm:"column:percentual_pote;not null"`
	TaxaPlataformaPct      int    `gorm:"column:taxa_plataforma_pct;not null;default:0"`
	InstrutorModoPagamento string `gorm:"column:instrutor_modo_pagamento;not null;default:fixo"`
	InstrutorFixoCentavos  int64  `gorm:"column:instrutor_fixo_centavos;not null;default:0"`
	InstrutorPct           int    `gorm:"column:instrutor_pct;not null;default:0"`
	// InstrutoresVagas (00021) é o tamanho da equipe (1..20); fixo e % acima são POR instrutor.
	InstrutoresVagas int `gorm:"column:instrutores_vagas;not null;default:1"`
	// InstrutorSelecao e InstrutorTetoCentavos são LEGADOS desde a 00021 (antigo leilão).
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
	// Equipe de instrutores (00021), responsável primeiro. Carregada por BuscarDetalhe/Listar e, nas
	// ações com o desafio travado, por comDesafioTravado.
	Equipe []DesafioInstrutor `gorm:"foreignKey:IDDesafio"`
	// Divisão do prêmio por posição (00022, tabela desafio_premio_posicoes), carregar ordenada por posição.
	PremioPosicoes []DesafioPremioPosicao `gorm:"foreignKey:IDDesafio"`

	// Preenchidos por DesafioRepository.Listar (não são colunas): contagens e a inscrição do
	// usuário que consulta. Em BuscarDetalhe o service os deriva de Participantes.
	QtdParticipantes  int                  `gorm:"-"`
	QtdPresentes      int                  `gorm:"-"`
	MinhaParticipacao *DesafioParticipante `gorm:"-"`
	// Preenchidos pelo service (equipe): candidaturas pendentes (visão da academia dona)
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
//
// Rodada 5: o local é o endereço estruturado (mesmas regras do endereço da academia; `local` texto é
// montado pelo backend e um `local` enviado é ignorado), o evento tem início E fim, e o instrutor virou
// uma EQUIPE de `instrutores_vagas` instrutores. `indicados` (instrutores vinculados e aprovados) entram
// direto; as vagas restantes ficam abertas à candidatura. Valores do instrutor são POR instrutor.
type CriarDesafioRequest struct {
	Titulo string `json:"titulo" validate:"required,min=3,max=200"`
	// Resumo (20–280 caracteres) e Regras (50–10000) são OBRIGATÓRIOS (rodada 6, U2); texto puro.
	// Descricao é LEGADO: aceita e gravada na coluna antiga, mas não substitui resumo/regras.
	Resumo    string `json:"resumo" validate:"max=2000"`
	Regras    string `json:"regras" validate:"max=40000"`
	Descricao string `json:"descricao" validate:"max=2000"`
	Categoria string `json:"categoria" validate:"max=30"`
	// Endereço estruturado do evento (obrigatório; o formulário vem pré-preenchido com o da academia).
	EnderecoAcademiaRequest
	Latitude  *float64 `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude *float64 `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	// DataEvento (início) e DataFimEvento aceitam RFC3339 ("2026-11-20T09:00:00-03:00") ou
	// "YYYY-MM-DDTHH:MM" / "YYYY-MM-DD HH:MM" (horário local do servidor). Início no futuro; fim > início.
	DataEvento           string `json:"data_evento" validate:"required"`
	DataFimEvento        string `json:"data_fim_evento" validate:"required"`
	Vagas                int    `json:"vagas" validate:"required,gte=2,lte=500"`
	ValorEntradaCentavos int64  `json:"valor_entrada_centavos" validate:"gte=0,lte=1000000"`
	PosicoesPodio        int    `json:"posicoes_podio" validate:"required,gte=1,lte=100"`
	// DivisaoPremioBP é a fatia do prêmio de cada posição em pontos-base (10000 = 100%; índice 0 = 1º):
	// exatamente posicoes_podio valores, cada um > 0, soma 10000, não crescente. Omitida = divisão
	// padrão (60/30/10… — ver GET /api/desafios/divisao-padrao).
	DivisaoPremioBP []int `json:"divisao_premio_bp" validate:"omitempty,max=100"`
	// Filtros de entrada (opcionais, U4): nível GymScore 1..10 e idade 0..120 na data do evento; min <= max.
	// 0 ou ausente = sem esse limite.
	NivelMin *int `json:"nivel_min" validate:"omitempty,gte=0,lte=10"`
	NivelMax *int `json:"nivel_max" validate:"omitempty,gte=0,lte=10"`
	IdadeMin *int `json:"idade_min" validate:"omitempty,gte=0,lte=120"`
	IdadeMax *int `json:"idade_max" validate:"omitempty,gte=0,lte=120"`

	// Premiação: prêmio fixo (reservado do saldo da academia) + parcela das inscrições (percentual_pote
	// de T, se dividir_pote). dividir_pote omitido = true; percentual_pote omitido = o máximo permitido
	// (100 - taxa da plataforma - instrutor_pct x instrutores_vagas). Com dividir_pote=false, percentual_pote é gravado 0.
	PremioFixoCentavos int64 `json:"premio_fixo_centavos" validate:"gte=0,lte=10000000"`
	DividirPote        *bool `json:"dividir_pote"`
	PercentualPote     *int  `json:"percentual_pote" validate:"omitempty,gte=0,lte=100"`

	// Equipe de instrutores: instrutores_vagas (1..20, omitido = 1) é o tamanho da equipe; indicados são
	// instrutores.id vinculados e aprovados que entram direto (no máx. instrutores_vagas, sem repetir);
	// id_responsavel (opcional, um dos indicados; padrão = o primeiro indicado) lança o pódio.
	// id_instrutor (legado) = indicados [id_instrutor] quando indicados vier vazio.
	// instrutor_modo_pagamento: fixo (padrão) | percentual | fixo_percentual; fixo e % são POR instrutor.
	InstrutoresVagas       int    `json:"instrutores_vagas" validate:"omitempty,gte=1,lte=20"`
	Indicados              []uint `json:"indicados" validate:"omitempty,max=20"`
	IDResponsavel          uint   `json:"id_responsavel"`
	IDInstrutor            uint   `json:"id_instrutor"`
	InstrutorModoPagamento string `json:"instrutor_modo_pagamento" validate:"omitempty,oneof=fixo percentual fixo_percentual"`
	InstrutorFixoCentavos  int64  `json:"instrutor_fixo_centavos" validate:"gte=0,lte=10000000"`
	InstrutorPct           int    `json:"instrutor_pct" validate:"gte=0,lte=100"`
	// LEGADO (leilão removido na rodada 5): instrutor_selecao=vaga_aberta ou instrutor_teto_centavos
	// respondem 400 com a orientação do modelo novo.
	InstrutorSelecao      string `json:"instrutor_selecao"`
	InstrutorTetoCentavos *int64 `json:"instrutor_teto_centavos"`
}

// EditarDesafioRequest é o DTO do PATCH /desafios/:id: todos os campos são opcionais
// (ponteiro nil = não altera). Descricao "" limpa a descrição. O endereço só muda inteiro: enviar
// qualquer um de cep/logradouro/numero/complemento/bairro/cidade/uf exige o endereço completo.
type EditarDesafioRequest struct {
	Titulo *string `json:"titulo" validate:"omitempty,min=3,max=200"`
	// Resumo/Regras: mesmas regras da criação (não podem ser apagados). Descricao é LEGADO.
	Resumo               *string  `json:"resumo" validate:"omitempty,max=2000"`
	Regras               *string  `json:"regras" validate:"omitempty,max=40000"`
	Descricao            *string  `json:"descricao" validate:"omitempty,max=2000"`
	Categoria            *string  `json:"categoria" validate:"omitempty,max=30"`
	CEP                  *string  `json:"cep" validate:"omitempty,max=20"`
	Logradouro           *string  `json:"logradouro" validate:"omitempty,max=150"`
	Numero               *string  `json:"numero" validate:"omitempty,max=20"`
	Complemento          *string  `json:"complemento" validate:"omitempty,max=100"`
	Bairro               *string  `json:"bairro" validate:"omitempty,max=100"`
	Cidade               *string  `json:"cidade" validate:"omitempty,max=100"`
	UF                   *string  `json:"uf" validate:"omitempty,max=2"`
	Latitude             *float64 `json:"latitude" validate:"omitempty,gte=-90,lte=90"`
	Longitude            *float64 `json:"longitude" validate:"omitempty,gte=-180,lte=180"`
	DataEvento           *string  `json:"data_evento"`
	DataFimEvento        *string  `json:"data_fim_evento"`
	Vagas                *int     `json:"vagas" validate:"omitempty,gte=2,lte=500"`
	ValorEntradaCentavos *int64   `json:"valor_entrada_centavos" validate:"omitempty,gte=0,lte=1000000"`
	PosicoesPodio        *int     `json:"posicoes_podio" validate:"omitempty,gte=1,lte=100"`
	// Premiação por posição e filtros de entrada: só podem mudar ANTES da 1ª inscrição (409 depois).
	// Mudar posicoes_podio sem mandar divisao_premio_bp aplica a divisão padrão do novo tamanho.
	// Filtros: enviar o campo troca o valor; 0 remove o filtro (NULL).
	DivisaoPremioBP []int `json:"divisao_premio_bp" validate:"omitempty,max=100"`
	NivelMin        *int  `json:"nivel_min" validate:"omitempty,gte=0,lte=10"`
	NivelMax        *int  `json:"nivel_max" validate:"omitempty,gte=0,lte=10"`
	IdadeMin        *int  `json:"idade_min" validate:"omitempty,gte=0,lte=120"`
	IdadeMax        *int  `json:"idade_max" validate:"omitempty,gte=0,lte=120"`

	// Financeiro (mesmas regras da criação; a taxa da plataforma fica a do snapshot). Com atletas
	// inscritos o prêmio só pode aumentar. O pagamento por instrutor (modo/fixo/%) fica travado quando há
	// membro da equipe vindo de candidatura ou candidatura pendente (eles aceitaram aquelas condições).
	// instrutores_vagas não pode ficar abaixo do tamanho atual da equipe.
	PremioFixoCentavos     *int64  `json:"premio_fixo_centavos" validate:"omitempty,gte=0,lte=10000000"`
	DividirPote            *bool   `json:"dividir_pote"`
	PercentualPote         *int    `json:"percentual_pote" validate:"omitempty,gte=0,lte=100"`
	InstrutoresVagas       *int    `json:"instrutores_vagas" validate:"omitempty,gte=1,lte=20"`
	InstrutorModoPagamento *string `json:"instrutor_modo_pagamento" validate:"omitempty,oneof=fixo percentual fixo_percentual"`
	InstrutorFixoCentavos  *int64  `json:"instrutor_fixo_centavos" validate:"omitempty,gte=0,lte=10000000"`
	InstrutorPct           *int    `json:"instrutor_pct" validate:"omitempty,gte=0,lte=100"`
	// LEGADO: id_instrutor (troque a equipe por /desafios/:id/equipe) e instrutor_teto_centavos (leilão
	// removido) respondem 400.
	IDInstrutor           *uint  `json:"id_instrutor"`
	InstrutorTetoCentavos *int64 `json:"instrutor_teto_centavos"`
}

// IndicarInstrutorRequest é o corpo de POST /desafios/:id/equipe (academia dona indica um vinculado).
type IndicarInstrutorRequest struct {
	IDInstrutor uint `json:"id_instrutor" validate:"required,gte=1"`
}

// ResponsavelRequest é o corpo de PUT /desafios/:id/equipe/responsavel.
type ResponsavelRequest struct {
	IDInstrutor uint `json:"id_instrutor" validate:"required,gte=1"`
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

// InstrutorResumo identifica um instrutor (o responsável da equipe em `instrutor`/`responsavel`).
type InstrutorResumo struct {
	ID        uint   `json:"id"`         // instrutores.id
	IDUsuario uint   `json:"id_usuario"` // usuarios.id do instrutor (ex.: para denunciá-lo pelo desafio)
	Nome      string `json:"nome"`
}

// MembroEquipeResponse é um instrutor da equipe do desafio.
type MembroEquipeResponse struct {
	ID          uint      `json:"id"`         // instrutores.id
	IDUsuario   uint      `json:"id_usuario"` // usuarios.id (denúncia pelo desafio)
	Nome        string    `json:"nome"`
	Origem      string    `json:"origem"` // indicado | candidatura
	Responsavel bool      `json:"responsavel"`
	Vinculado   bool      `json:"vinculado"` // vinculado à academia do desafio
	EntrouEm    time.Time `json:"entrou_em"`
}

// EnderecoResponse é o endereço estruturado (do desafio ou da academia). null em desafios antigos.
type EnderecoResponse struct {
	CEP         string `json:"cep"` // só dígitos
	Logradouro  string `json:"logradouro"`
	Numero      string `json:"numero"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Cidade      string `json:"cidade"`
	UF          string `json:"uf"`
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
	ID     uint   `json:"id"`
	Titulo string `json:"titulo"`
	// Resumo (curto, para cards) e Regras (texto completo) — rodada 6; texto puro (escapar no front).
	// Descricao é compat: a descrição legada ou, sem ela, o resumo.
	Resumo               string            `json:"resumo"`
	Regras               string            `json:"regras"`
	Descricao            string            `json:"descricao"`
	Categoria            string            `json:"categoria"`
	ValorEntradaCentavos int64             `json:"valor_entrada_centavos"`
	ValorEntrada         float64           `json:"valor_entrada"`
	PoteCentavos         int64             `json:"pote_centavos"`
	Pote                 float64           `json:"pote"`
	Local                string            `json:"local"`    // texto montado do endereço (compat)
	Endereco             *EnderecoResponse `json:"endereco"` // estruturado; null em desafios antigos
	Latitude             *float64          `json:"latitude,omitempty"`
	Longitude            *float64          `json:"longitude,omitempty"`
	DataEvento           *time.Time        `json:"data_evento"`     // início; null só em desafios legados
	DataFimEvento        *time.Time        `json:"data_fim_evento"` // fim; null em legados
	Status               StatusDesafio     `json:"status"`
	Vagas                int               `json:"vagas"`
	PosicoesPodio        int               `json:"posicoes_podio"`
	// DivisaoPremioBP: fatia de cada posição em pontos-base (10000 = 100%), posicoes_podio itens.
	DivisaoPremioBP []int `json:"divisao_premio_bp"`
	// Requisitos de entrada (null = sem limite): nível GymScore e idade na data do evento.
	NivelMin *int            `json:"nivel_min"`
	NivelMax *int            `json:"nivel_max"`
	IdadeMin *int            `json:"idade_min"`
	IdadeMax *int            `json:"idade_max"`
	Academia *AcademiaResumo `json:"academia"` // null só em legados
	// Instrutor = o RESPONSÁVEL da equipe (compat: null sem responsável). Equipe traz todos os membros
	// (responsável primeiro); InstrutoresVagas é o tamanho da equipe e VagasInstrutorAbertas as vagas
	// livres para candidatura.
	Instrutor             *InstrutorResumo       `json:"instrutor"`
	Equipe                []MembroEquipeResponse `json:"equipe"`
	InstrutoresVagas      int                    `json:"instrutores_vagas"`
	VagasInstrutorAbertas int                    `json:"vagas_instrutor_abertas"`
	// PremiacaoPrevistaCentavos tem posicoes_podio itens: o prêmio total ATUAL dividido pela tabela do
	// desafio (divisao_premio_bp; se houver menos presentes que posições, os valores finais são renormalizados).
	PremiacaoPrevistaCentavos []int64 `json:"premiacao_prevista_centavos"`
	// Premiacao (todos): prêmio total previsto + lista por posição. Financeiro só vai para a academia
	// dona, InstrutorGanho para membros da equipe e Vaga para instrutores (oferta de trabalho).
	Premiacao      PremiacaoResponse       `json:"premiacao"`
	Financeiro     *FinanceiroResponse     `json:"financeiro,omitempty"`
	InstrutorGanho *GanhoInstrutorResponse `json:"instrutor_ganho,omitempty"`
	Vaga           *VagaInstrutorResponse  `json:"vaga,omitempty"`
	// VagaInstrutorAberta: desafio aberto com vaga livre na equipe (aceita candidaturas). Não impede
	// iniciar: basta 1 instrutor e o responsável definido (ver pode_iniciar).
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
	// MotivoBloqueioEntrada (rodada 4, B4-8; rodada 5): preenchido quando o atleta NÃO pode entrar por
	// conflito de interesse (está na equipe de instrutores, é instrutor aprovado vinculado à academia do
	// desafio ou tem candidatura ativa) ou por CHOQUE DE HORÁRIO com outro desafio da mesma academia em
	// que já está inscrito — mesma mensagem do 409 de POST /entrar. Vazio/omitido nos demais casos.
	MotivoBloqueioEntrada string `json:"motivo_bloqueio_entrada,omitempty"`
	PodeSair              bool   `json:"pode_sair"`
	PodeIniciar           bool   `json:"pode_iniciar"`
	PodeMarcarPresenca    bool   `json:"pode_marcar_presenca"`
	PodeLancarPodio       bool   `json:"pode_lancar_podio"`
	PodeCancelar          bool   `json:"pode_cancelar"`
	PodeEditar            bool   `json:"pode_editar"`
	// PodeEditarPremiacao: academia dona, aberto e SEM inscritos — só então divisão do prêmio, nº de
	// premiados e filtros de nível/idade podem mudar (U1/U4).
	PodeEditarPremiacao bool `json:"pode_editar_premiacao"`
	PodeAvaliar         bool `json:"pode_avaliar"`
	// PodeCandidatar: instrutor pode se candidatar (vaga livre na equipe, desafio aberto, evento futuro).
	// PodeGerirEquipe: academia dona pode indicar/remover membros e aceitar/recusar candidaturas
	// (desafio aberto). PodeEscolherInstrutor é o nome antigo (mesmo valor). PodeDefinirResponsavel:
	// academia dona, desafio aberto ou em andamento, equipe não vazia.
	// VoceNaEquipe / VoceResponsavel: o instrutor do modo instrutor é membro / o responsável.
	PodeCandidatar         bool `json:"pode_candidatar"`
	PodeGerirEquipe        bool `json:"pode_gerir_equipe"`
	PodeEscolherInstrutor  bool `json:"pode_escolher_instrutor"`
	PodeDefinirResponsavel bool `json:"pode_definir_responsavel"`
	VoceNaEquipe           bool `json:"voce_na_equipe"`
	VoceResponsavel        bool `json:"voce_responsavel"`

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
//
// Rodada 6 (A6-1): Razao é o "por que" (UMA frase em pt-BR, TEXTO PURO — o front deve inserir como texto,
// nunca como HTML); RazaoFonte diz quem a escreveu ("ia" = Claude, validada; "regras" = frase determinística
// do backend, usada sem chave, em timeout/erro ou quando a saída do modelo é inválida); Base expõe os números
// em que a recomendação se baseou (nível, categoria e atributos do atleta) para a tela mostrar.
type DesafioRecomendado struct {
	Desafio    *DesafioResponse  `json:"desafio"`
	Razao      string            `json:"razao"`
	RazaoFonte string            `json:"razao_fonte"`
	Base       *BaseRecomendacao `json:"base"`
}

// BaseRecomendacao são os dados determinísticos que pontuaram o desafio para o atleta.
type BaseRecomendacao struct {
	Score           int    `json:"score"`
	Nivel           int    `json:"nivel"`
	Categoria       string `json:"categoria"`
	CategoriaRotulo string `json:"categoria_rotulo"`
	// ValorCategoria: valor 0..100 do atleta no eixo da categoria (null para geral/outros, sem eixo).
	ValorCategoria *int `json:"valor_categoria"`
	// Atributos: até 2 atributos que a categoria mais exige (peso_pct) com o valor 0..100 do atleta.
	Atributos       []EixoRecomendacao `json:"atributos"`
	NivelMin        *int               `json:"nivel_min"`
	NivelMax        *int               `json:"nivel_max"`
	ValorCompativel bool               `json:"valor_compativel"`
	SemDados        bool               `json:"sem_dados"`
}

// EixoRecomendacao é um atributo citado na base da recomendação.
type EixoRecomendacao struct {
	Chave   string `json:"chave"`
	Rotulo  string `json:"rotulo"`
	Valor   int    `json:"valor"`
	PesoPct int    `json:"peso_pct"`
}
