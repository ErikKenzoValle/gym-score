package models

import (
	"encoding/json"
	"time"
)

// Moderação (Sprint 7, migrations 00013 + 00019). Os COMMENTs das colunas no banco são a especificação;
// regras de negócio em analise/moderacao-spec.md. Nada aqui é apagado: só INSERT e UPDATE condicional
// (as transições de estado ficam nos repositórios, nunca em Save() da linha inteira).

// Ledger: tipos da moderação (ambos débitos, valor negativo — CHECK chk_transacoes_sinal da 00016).
const (
	// TipoEstornoAnulacao devolve um crédito do pódio já liberado quando o resultado é anulado.
	TipoEstornoAnulacao TipoTransacao = "estorno_anulacao"
	// TipoAbatimentoDebito desconta um usuario_debitos de um crédito que chegou ao devedor.
	TipoAbatimentoDebito TipoTransacao = "abatimento_debito"
)

// ReceitaEstornoTaxa devolve a taxa da plataforma já liberada (valor NEGATIVO; migration 00019, Q6).
const ReceitaEstornoTaxa = "estorno_taxa"

// Categorias de denúncia.
const (
	DenunciaSubornoInstrutor  = "suborno_instrutor"
	DenunciaManipulacaoPodio  = "manipulacao_podio"
	DenunciaFraudePresenca    = "fraude_presenca"
	DenunciaCondutaAbusiva    = "conduta_abusiva"
	DenunciaPerfilFalso       = "perfil_falso"
	DenunciaOutro             = "outro"
	DenunciaDescricaoMin      = 20
	DenunciaDescricaoMax      = 2000
	DenunciaEvidenciasMaximo  = 3
	MotivoBloqueioMaxCaracter = 255
)

// CategoriaDeDesafio diz se a categoria exige id_desafio e retém os pagamentos do desafio.
func CategoriaDeDesafio(c string) bool {
	return c == DenunciaSubornoInstrutor || c == DenunciaManipulacaoPodio || c == DenunciaFraudePresenca
}

// Status e jurisdição da denúncia.
const (
	DenunciaAberta       = "aberta"
	DenunciaEmAnalise    = "em_analise"
	DenunciaProcedente   = "procedente"
	DenunciaImprocedente = "improcedente"
	DenunciaArquivada    = "arquivada"

	JurisdicaoAdmin    = "admin"
	JurisdicaoAcademia = "academia"
)

// Denuncia é uma denúncia entre usuários. IDDenunciante é sigiloso (só admin): nunca serializar o model
// direto na API. Responsável: IDResponsavel (conta da academia) OU IDResponsavelAdmin (00019).
type Denuncia struct {
	ID                  uint       `gorm:"primaryKey;autoIncrement"`
	IDDenunciante       uint       `gorm:"column:id_denunciante;not null"`
	IDDenunciado        uint       `gorm:"column:id_denunciado;not null"`
	IDDesafio           *uint      `gorm:"column:id_desafio"`
	Categoria           string     `gorm:"column:categoria;not null"`
	Descricao           string     `gorm:"column:descricao;type:text;not null"`
	Status              string     `gorm:"column:status;not null;default:aberta"`
	Jurisdicao          string     `gorm:"column:jurisdicao;not null;default:admin"`
	IDAcademiaJulgadora *uint      `gorm:"column:id_academia_julgadora"`
	IDResponsavel       *uint      `gorm:"column:id_responsavel"`
	IDResponsavelAdmin  *uint      `gorm:"column:id_responsavel_admin"`
	PrazoAcademiaAte    *time.Time `gorm:"column:prazo_academia_ate"`
	EncaminhadaEm       *time.Time `gorm:"column:encaminhada_em"`
	DecididaEm          *time.Time `gorm:"column:decidida_em"`
	DecisaoTexto        *string    `gorm:"column:decisao_texto;type:text"`
	// ChaveAberta é coluna gerada (STORED): somente leitura, nunca gravar.
	ChaveAberta  *string   `gorm:"column:chave_aberta;->"`
	CriadoEm     time.Time `gorm:"column:criado_em;autoCreateTime"`
	AtualizadoEm time.Time `gorm:"column:atualizado_em;autoUpdateTime"`

	Evidencias []DenunciaEvidencia `gorm:"foreignKey:IDDenuncia"`
}

func (Denuncia) TableName() string { return "denuncias" }

// DenunciaEvidencia é uma imagem de evidência (ordem 1..3; arquivo em EVIDENCIAS_DIR, nome aleatório).
type DenunciaEvidencia struct {
	ID           uint      `gorm:"primaryKey;autoIncrement"`
	IDDenuncia   uint      `gorm:"column:id_denuncia;not null"`
	Ordem        uint8     `gorm:"column:ordem;not null"`
	Caminho      string    `gorm:"column:caminho;size:255;not null"`
	Mime         string    `gorm:"column:mime;size:50;not null"`
	TamanhoBytes uint64    `gorm:"column:tamanho_bytes;not null"`
	CriadoEm     time.Time `gorm:"column:criado_em;autoCreateTime"`
}

func (DenunciaEvidencia) TableName() string { return "denuncia_evidencias" }

// Tipos e escopos de punição.
const (
	PunicaoAdvertencia       = "advertencia"
	PunicaoSuspensao         = "suspensao"
	PunicaoBanimento         = "banimento"
	PunicaoBloqueioAcademia  = "bloqueio_academia"
	PunicaoAnulacaoResultado = "anulacao_resultado"

	EscopoGlobal   = "global"
	EscopoAcademia = "academia"
)

// Punicao é uma punição aplicada. Aplicador: AplicadaPor (conta da academia, escopo academia) OU
// AplicadaPorAdmin (CHECK chk_punicoes_aplicador; escopo global ⇒ admin). RevogadaPor = administradores.id.
type Punicao struct {
	ID               uint       `gorm:"primaryKey;autoIncrement"`
	IDDenuncia       *uint      `gorm:"column:id_denuncia"`
	IDUsuario        uint       `gorm:"column:id_usuario;not null"`
	Tipo             string     `gorm:"column:tipo;not null"`
	Escopo           string     `gorm:"column:escopo;not null;default:global"`
	IDAcademia       *uint      `gorm:"column:id_academia"`
	IDDesafio        *uint      `gorm:"column:id_desafio"`
	Inicio           time.Time  `gorm:"column:inicio;not null"`
	Fim              *time.Time `gorm:"column:fim"`
	AplicadaPor      *uint      `gorm:"column:aplicada_por"`
	AplicadaPorAdmin *uint      `gorm:"column:aplicada_por_admin"`
	Justificativa    string     `gorm:"column:justificativa;type:text;not null"`
	RevogadaEm       *time.Time `gorm:"column:revogada_em"`
	RevogadaPor      *uint      `gorm:"column:revogada_por"`
	RevogadaMotivo   *string    `gorm:"column:revogada_motivo;type:text"`
	CriadoEm         time.Time  `gorm:"column:criado_em;autoCreateTime"`
}

func (Punicao) TableName() string { return "punicoes" }

// Vigente diz se a punição vale no instante `agora` (não revogada, já iniciada e sem fim vencido).
func (p *Punicao) Vigente(agora time.Time) bool {
	return p.RevogadaEm == nil && !p.Inicio.After(agora) && (p.Fim == nil || p.Fim.After(agora))
}

// Status do recurso.
const (
	RecursoPendente   = "pendente"
	RecursoDeferido   = "deferido"
	RecursoIndeferido = "indeferido"
)

// Recurso contra uma punição (no máximo 1 por punição). JulgadoPor = administradores.id (00019).
type Recurso struct {
	ID           uint       `gorm:"primaryKey;autoIncrement"`
	IDPunicao    uint       `gorm:"column:id_punicao;not null"`
	Texto        string     `gorm:"column:texto;type:text;not null"`
	Status       string     `gorm:"column:status;not null;default:pendente"`
	JulgadoPor   *uint      `gorm:"column:julgado_por"`
	JulgadoEm    *time.Time `gorm:"column:julgado_em"`
	DecisaoTexto *string    `gorm:"column:decisao_texto;type:text"`
	CriadoEm     time.Time  `gorm:"column:criado_em;autoCreateTime"`
}

func (Recurso) TableName() string { return "recursos" }

// Tipos e status de pagamento pendente (janela de contestação do pódio).
const (
	PendentePremio             = "premio_desafio"
	PendentePagamentoInstrutor = "pagamento_instrutor"
	PendenteReceitaAcademia    = "receita_academia"
	PendenteTaxaInscricao      = "taxa_inscricao"

	PendenteALiberar  = "a_liberar"
	PendenteRetido    = "retido"
	PendenteLiberado  = "liberado"
	PendenteCancelado = "cancelado"
)

// PagamentoPendente é um crédito do pódio fora do saldo até `LiberarEm`. IDUsuario NULL = plataforma
// (tipo taxa_inscricao, liberado em plataforma_receitas).
type PagamentoPendente struct {
	ID                 uint       `gorm:"primaryKey;autoIncrement"`
	IDDesafio          uint       `gorm:"column:id_desafio;not null"`
	IDUsuario          *uint      `gorm:"column:id_usuario"`
	Tipo               string     `gorm:"column:tipo;not null"`
	ValorCentavos      int64      `gorm:"column:valor_centavos;not null"`
	Status             string     `gorm:"column:status;not null;default:a_liberar"`
	LiberarEm          time.Time  `gorm:"column:liberar_em;not null"`
	IDDenunciaRetencao *uint      `gorm:"column:id_denuncia_retencao"`
	LiberadoEm         *time.Time `gorm:"column:liberado_em"`
	IDTransacao        *uint      `gorm:"column:id_transacao"`
	CriadoEm           time.Time  `gorm:"column:criado_em;autoCreateTime"`
	AtualizadoEm       time.Time  `gorm:"column:atualizado_em;autoUpdateTime"`
}

func (PagamentoPendente) TableName() string { return "pagamentos_pendentes" }

// UsuarioDebito é o que faltou estornar do saldo numa anulação; com débito aberto o saque é bloqueado.
type UsuarioDebito struct {
	ID                    uint       `gorm:"primaryKey;autoIncrement"`
	IDUsuario             uint       `gorm:"column:id_usuario;not null"`
	IDDesafio             uint       `gorm:"column:id_desafio;not null"`
	ValorCentavos         int64      `gorm:"column:valor_centavos;not null"`
	SaldoRestanteCentavos int64      `gorm:"column:saldo_restante_centavos;not null"`
	Motivo                string     `gorm:"column:motivo;size:255;not null"`
	CriadoEm              time.Time  `gorm:"column:criado_em;autoCreateTime"`
	QuitadoEm             *time.Time `gorm:"column:quitado_em"`
}

func (UsuarioDebito) TableName() string { return "usuario_debitos" }

// Papéis do ator e entidades da linha do tempo.
const (
	AtorAdmin    = "admin"
	AtorAcademia = "academia"
	AtorUsuario  = "usuario"
	AtorSistema  = "sistema"

	EntidadeDenuncia  = "denuncia"
	EntidadePunicao   = "punicao"
	EntidadeRecurso   = "recurso"
	EntidadeDesafio   = "desafio"
	EntidadePagamento = "pagamento"
	EntidadeDebito    = "debito"
	EntidadeUsuario   = "usuario"
)

// ModeracaoEvento é uma linha append-only da linha do tempo. Ator: IDAtor (usuario/academia) OU
// IDAdmin (admin) OU nenhum (sistema) — CHECK chk_eventos_ator (00019).
type ModeracaoEvento struct {
	ID            uint            `gorm:"primaryKey;autoIncrement"`
	IDAtor        *uint           `gorm:"column:id_ator"`
	IDAdmin       *uint           `gorm:"column:id_admin"`
	AtorPapel     string          `gorm:"column:ator_papel;not null"`
	Acao          string          `gorm:"column:acao;size:60;not null"`
	Entidade      string          `gorm:"column:entidade;not null"`
	IDEntidade    uint            `gorm:"column:id_entidade;not null"`
	IDDesafio     *uint           `gorm:"column:id_desafio"`
	Justificativa *string         `gorm:"column:justificativa;type:text"`
	Dados         json.RawMessage `gorm:"column:dados;type:json"`
	CriadoEm      time.Time       `gorm:"column:criado_em;autoCreateTime"`
}

func (ModeracaoEvento) TableName() string { return "moderacao_eventos" }

// Modos de anulação do resultado (desafios.resultado_anulacao_modo).
const (
	AnulacaoRelancarPodio      = "relancar_podio"
	AnulacaoDevolverInscricoes = "devolver_inscricoes"
)
