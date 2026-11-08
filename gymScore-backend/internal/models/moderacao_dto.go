package models

import (
	"encoding/json"
	"time"
)

// DTOs da moderação. Regra de sigilo: o denunciante (IDDenunciante / Denunciante) só é preenchido
// nas respostas do admin; os montadores do serviço nunca o copiam para usuário/academia.

// EstadoSessao é o que o AuthMiddleware lê do usuário a cada requisição (uma consulta).
type EstadoSessao struct {
	Versao      int
	Existe      bool
	BanidoEm    *time.Time
	SuspensoAte *time.Time
}

// ─── Requisições ─────────────────────────────────────────────────────────────

// CriarDenunciaRequest é POST /api/denuncias. id_desafio é obrigatório nas categorias de desafio.
type CriarDenunciaRequest struct {
	IDDenunciado uint   `json:"id_denunciado" validate:"required,gte=1"`
	IDDesafio    *uint  `json:"id_desafio" validate:"omitempty,gte=1"`
	Categoria    string `json:"categoria" validate:"required"`
	Descricao    string `json:"descricao" validate:"required"`
}

// EncaminharDenunciaRequest é o encaminhamento da academia ao admin.
type EncaminharDenunciaRequest struct {
	Justificativa string `json:"justificativa" validate:"required"`
}

// PunicaoAcademiaRequest é a punição que a academia pode aplicar ao decidir (advertencia | bloqueio_academia).
type PunicaoAcademiaRequest struct {
	Tipo string     `json:"tipo" validate:"required"`
	Fim  *time.Time `json:"fim"`
}

// DecidirDenunciaAcademiaRequest é a decisão da academia julgadora.
type DecidirDenunciaAcademiaRequest struct {
	Status       string                  `json:"status" validate:"required"`
	DecisaoTexto string                  `json:"decisao_texto" validate:"required"`
	Punicao      *PunicaoAcademiaRequest `json:"punicao"`
}

// PunicaoDecisaoRequest é uma punição aplicada pelo admin ao decidir a denúncia (alvo = denunciado).
type PunicaoDecisaoRequest struct {
	Tipo       string     `json:"tipo" validate:"required"`
	IDAcademia *uint      `json:"id_academia" validate:"omitempty,gte=1"`
	Fim        *time.Time `json:"fim"`
}

// DecidirDenunciaAdminRequest é a decisão do admin (motivo vai para admin_auditoria).
type DecidirDenunciaAdminRequest struct {
	Status       string                  `json:"status" validate:"required"`
	DecisaoTexto string                  `json:"decisao_texto" validate:"required"`
	Punicoes     []PunicaoDecisaoRequest `json:"punicoes" validate:"max=5,dive"`
	Motivo       string                  `json:"motivo" validate:"required,min=3,max=500"`
}

// AplicarPunicaoRequest é POST /api/admin/punicoes (aplicação direta, com ou sem denúncia).
type AplicarPunicaoRequest struct {
	IDUsuario     uint       `json:"id_usuario" validate:"required,gte=1"`
	Tipo          string     `json:"tipo" validate:"required"`
	IDAcademia    *uint      `json:"id_academia" validate:"omitempty,gte=1"`
	Fim           *time.Time `json:"fim"`
	Justificativa string     `json:"justificativa" validate:"required"`
	IDDenuncia    *uint      `json:"id_denuncia" validate:"omitempty,gte=1"`
	Motivo        string     `json:"motivo" validate:"required,min=3,max=500"`
}

// JulgarRecursoRequest é POST /api/admin/recursos/:id/julgar.
type JulgarRecursoRequest struct {
	Status       string `json:"status" validate:"required"`
	DecisaoTexto string `json:"decisao_texto" validate:"required"`
	Motivo       string `json:"motivo" validate:"required,min=3,max=500"`
}

// AbrirRecursoRequest é POST /api/punicoes/:id/recurso.
type AbrirRecursoRequest struct {
	Texto string `json:"texto" validate:"required"`
}

// RecursoBanimentoRequest é a rota pública do banido (Q1): sem texto só consulta.
type RecursoBanimentoRequest struct {
	Email string `json:"email" validate:"required,max=150"`
	Senha string `json:"senha" validate:"required,max=72"`
	Texto string `json:"texto"`
}

// ─── Respostas ───────────────────────────────────────────────────────────────

// UsuarioRefModeracao identifica uma pessoa numa resposta da moderação.
type UsuarioRefModeracao struct {
	ID       uint   `json:"id"`
	Nome     string `json:"nome"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"` // só admin
}

// EvidenciaResponse são os metadados de uma evidência (o arquivo vem por rota própria).
type EvidenciaResponse struct {
	Ordem        uint8     `json:"ordem"`
	Mime         string    `json:"mime"`
	TamanhoBytes uint64    `json:"tamanho_bytes"`
	CriadoEm     time.Time `json:"criado_em"`
}

// DenunciaResponse é a denúncia vista por denunciante, academia julgadora ou admin. Campos de admin
// (IDDenunciante, Denunciante, IDResponsavelAdmin, ...) ficam vazios para os outros públicos.
type DenunciaResponse struct {
	ID                  uint                 `json:"id"`
	Denunciado          UsuarioRefModeracao  `json:"denunciado"`
	IDDesafio           *uint                `json:"id_desafio,omitempty"`
	Categoria           string               `json:"categoria"`
	Descricao           string               `json:"descricao"`
	Status              string               `json:"status"`
	Jurisdicao          string               `json:"jurisdicao"`
	IDAcademiaJulgadora *uint                `json:"id_academia_julgadora,omitempty"`
	PrazoAcademiaAte    *time.Time           `json:"prazo_academia_ate,omitempty"`
	EncaminhadaEm       *time.Time           `json:"encaminhada_em,omitempty"`
	DecididaEm          *time.Time           `json:"decidida_em,omitempty"`
	DecisaoTexto        *string              `json:"decisao_texto,omitempty"`
	CriadoEm            time.Time            `json:"criado_em"`
	Evidencias          []EvidenciaResponse  `json:"evidencias"`
	GerouPunicao        *bool                `json:"gerou_punicao,omitempty"`
	EmAnaliseComigo     *bool                `json:"em_analise_comigo,omitempty"` // academia
	IDDenunciante       *uint                `json:"id_denunciante,omitempty"`    // só admin
	Denunciante         *UsuarioRefModeracao `json:"denunciante,omitempty"`       // só admin
	IDResponsavel       *uint                `json:"id_responsavel,omitempty"`    // só admin
	IDResponsavelAdmin  *uint                `json:"id_responsavel_admin,omitempty"`
}

// DenunciaAcademiaDetalhe é o detalhe da denúncia para a academia julgadora (sem denunciante).
type DenunciaAcademiaDetalhe struct {
	Denuncia DenunciaResponse `json:"denuncia"`
	// HistoricoNaAcademia são as punições do denunciado aplicadas no escopo desta academia.
	HistoricoNaAcademia []PunicaoResponse `json:"historico_na_academia"`
}

// DenunciaAdminDetalhe é GET /api/admin/denuncias/:id.
type DenunciaAdminDetalhe struct {
	Denuncia DenunciaResponse  `json:"denuncia"`
	Punicoes []PunicaoResponse `json:"punicoes"`
	Eventos  []EventoResponse  `json:"eventos"`
	// OutrasDoDenunciado são as demais denúncias contra o mesmo usuário (mais recentes primeiro, até 20).
	OutrasDoDenunciado []DenunciaResponse `json:"outras_do_denunciado"`
	// ReincidenciaAdvertencias90d conta advertências do denunciado nos últimos 90 dias.
	ReincidenciaAdvertencias90d int64 `json:"reincidencia_advertencias_90d"`
}

// RecursoResponse é um recurso contra uma punição.
type RecursoResponse struct {
	ID           uint       `json:"id"`
	IDPunicao    uint       `json:"id_punicao"`
	Texto        string     `json:"texto"`
	Status       string     `json:"status"`
	JulgadoEm    *time.Time `json:"julgado_em,omitempty"`
	DecisaoTexto *string    `json:"decisao_texto,omitempty"`
	CriadoEm     time.Time  `json:"criado_em"`
	JulgadoPor   *uint      `json:"julgado_por,omitempty"` // só admin
}

// PunicaoResponse é uma punição (vista pelo punido, pela academia do escopo ou pelo admin).
type PunicaoResponse struct {
	ID              uint                 `json:"id"`
	IDUsuario       uint                 `json:"id_usuario"`
	Usuario         *UsuarioRefModeracao `json:"usuario,omitempty"`
	Tipo            string               `json:"tipo"`
	Escopo          string               `json:"escopo"`
	IDAcademia      *uint                `json:"id_academia,omitempty"`
	IDDesafio       *uint                `json:"id_desafio,omitempty"`
	IDDenuncia      *uint                `json:"id_denuncia,omitempty"`
	Inicio          time.Time            `json:"inicio"`
	Fim             *time.Time           `json:"fim,omitempty"`
	Justificativa   string               `json:"justificativa"`
	Vigente         bool                 `json:"vigente"`
	AplicadaPor     string               `json:"aplicada_por"` // "academia" | "admin"
	RevogadaEm      *time.Time           `json:"revogada_em,omitempty"`
	RevogadaMotivo  *string              `json:"revogada_motivo,omitempty"`
	Recurso         *RecursoResponse     `json:"recurso,omitempty"`
	PodeRecorrer    bool                 `json:"pode_recorrer"`
	RecorrerAte     *time.Time           `json:"recorrer_ate,omitempty"`
	IDAplicadorUser *uint                `json:"id_aplicador_usuario,omitempty"` // só admin
	IDAplicadorAdm  *uint                `json:"id_aplicador_admin,omitempty"`   // só admin
	RevogadaPor     *uint                `json:"revogada_por,omitempty"`         // só admin
	CriadoEm        time.Time            `json:"criado_em"`
}

// RecursoFilaItem é um item da fila de recursos do admin (recurso + punição).
type RecursoFilaItem struct {
	Recurso RecursoResponse `json:"recurso"`
	Punicao PunicaoResponse `json:"punicao"`
}

// EventoResponse é uma linha da linha do tempo (moderacao_eventos), só para admin.
type EventoResponse struct {
	ID            uint            `json:"id"`
	AtorPapel     string          `json:"ator_papel"`
	IDAtor        *uint           `json:"id_ator,omitempty"`
	IDAdmin       *uint           `json:"id_admin,omitempty"`
	Acao          string          `json:"acao"`
	Entidade      string          `json:"entidade"`
	IDEntidade    uint            `json:"id_entidade"`
	IDDesafio     *uint           `json:"id_desafio,omitempty"`
	Justificativa *string         `json:"justificativa,omitempty"`
	Dados         json.RawMessage `json:"dados,omitempty" swaggertype:"object"`
	CriadoEm      time.Time       `json:"criado_em"`
}

// BanimentoInfo é o que o banido recebe no login recusado (403 conta_banida) e na rota pública de recurso.
type BanimentoInfo struct {
	IDPunicao    uint             `json:"id_punicao"`
	Motivo       string           `json:"motivo"`
	BanidoEm     time.Time        `json:"banido_em"`
	PodeRecorrer bool             `json:"pode_recorrer"`
	RecorrerAte  *time.Time       `json:"recorrer_ate,omitempty"`
	Recurso      *RecursoResponse `json:"recurso,omitempty"`
}

// ModeracaoSessao é o estado de moderação do próprio usuário (login e /usuarios/me).
type ModeracaoSessao struct {
	SuspensoAte *time.Time `json:"suspenso_ate,omitempty"`
	Motivo      *string    `json:"motivo,omitempty"`
}

// ─── Janela de contestação, anulação e débitos (parte 2) ─────────────────────

// AnularResultadoRequest é POST /api/admin/desafios/:id/anular-resultado (superadmin). modo:
// relancar_podio (só sem pagamento já liberado; a academia dona relança) | devolver_inscricoes (devolve
// as entradas INTEGRAIS e a reserva; estorna o que já foi liberado, inclusive a taxa da plataforma).
// id_usuario_punido recebe a punição anulacao_resultado; id_denuncia (opcional) é decidida procedente.
type AnularResultadoRequest struct {
	Modo            string `json:"modo" validate:"required"`
	Justificativa   string `json:"justificativa" validate:"required"`
	IDUsuarioPunido uint   `json:"id_usuario_punido" validate:"required,gte=1"`
	IDDenuncia      *uint  `json:"id_denuncia" validate:"omitempty,gte=1"`
	Motivo          string `json:"motivo" validate:"required,min=3,max=500"`
}

// PagamentoPendenteResponse é um crédito do pódio na janela de contestação. Usuario, IDDenunciaRetencao
// e IDTransacao só vão para o admin.
type PagamentoPendenteResponse struct {
	ID                 uint                 `json:"id"`
	IDDesafio          uint                 `json:"id_desafio"`
	TituloDesafio      string               `json:"titulo_desafio"`
	IDUsuario          *uint                `json:"id_usuario"` // null = plataforma (taxa_inscricao)
	Usuario            *UsuarioRefModeracao `json:"usuario,omitempty"`
	Tipo               string               `json:"tipo"`
	ValorCentavos      int64                `json:"valor_centavos"`
	Status             string               `json:"status"` // a_liberar | retido | liberado | cancelado
	LiberarEm          time.Time            `json:"liberar_em"`
	LiberadoEm         *time.Time           `json:"liberado_em,omitempty"`
	IDDenunciaRetencao *uint                `json:"id_denuncia_retencao,omitempty"`
	IDTransacao        *uint                `json:"id_transacao,omitempty"`
	CriadoEm           time.Time            `json:"criado_em"`
}

// DebitoResponse é um débito do usuário (o que a anulação não conseguiu estornar do saldo).
type DebitoResponse struct {
	ID                    uint                 `json:"id"`
	IDUsuario             uint                 `json:"id_usuario"`
	Usuario               *UsuarioRefModeracao `json:"usuario,omitempty"` // só admin
	IDDesafio             uint                 `json:"id_desafio"`
	TituloDesafio         string               `json:"titulo_desafio"`
	ValorCentavos         int64                `json:"valor_centavos"`
	SaldoRestanteCentavos int64                `json:"saldo_restante_centavos"`
	Motivo                string               `json:"motivo"`
	Quitado               bool                 `json:"quitado"`
	CriadoEm              time.Time            `json:"criado_em"`
	QuitadoEm             *time.Time           `json:"quitado_em,omitempty"`
}

// CarteiraPendentesResponse é GET /api/carteira/pendentes: totais a receber do pódio + página.
type CarteiraPendentesResponse struct {
	ALiberarCentavos int64                       `json:"a_liberar_centavos"`
	RetidoCentavos   int64                       `json:"retido_centavos"`
	Itens            []PagamentoPendenteResponse `json:"itens"`
	Total            int64                       `json:"total"`
	Pagina           int                         `json:"pagina"`
	Limite           int                         `json:"limite"`
}

// CarteiraDebitosResponse é GET /api/carteira/debitos: com débito aberto o saque é bloqueado.
type CarteiraDebitosResponse struct {
	AbertoCentavos int64            `json:"aberto_centavos"`
	SaqueBloqueado bool             `json:"saque_bloqueado"`
	Itens          []DebitoResponse `json:"itens"`
}

// AnulacaoResponse resume o que a anulação do resultado fez (tudo numa transação).
type AnulacaoResponse struct {
	IDDesafio                    uint             `json:"id_desafio"`
	Modo                         string           `json:"modo"`
	IDPunicao                    uint             `json:"id_punicao"`
	PendentesCancelados          int              `json:"pendentes_cancelados"`
	CanceladoCentavos            int64            `json:"cancelado_centavos"`
	EstornadoCentavos            int64            `json:"estornado_centavos"` // tirado dos saldos (estorno_anulacao)
	TaxaEstornadaCentavos        int64            `json:"taxa_estornada_centavos"`
	InscricoesDevolvidasCentavos int64            `json:"inscricoes_devolvidas_centavos"`
	ReservaDevolvidaCentavos     int64            `json:"reserva_devolvida_centavos"`
	Debitos                      []DebitoResponse `json:"debitos"`
	Status                       string           `json:"status_desafio"`
}
