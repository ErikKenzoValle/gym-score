package models

// Tipos de conta (usuarios.perfil, migration 00020). Instrutor NÃO é tipo de conta: é um segundo
// modo da conta de atleta, habilitado pela linha em `instrutores` com status aprovada.
const (
	PerfilAtleta   = "atleta"
	PerfilAcademia = "academia"
)

// Modos de uso de uma conta de atleta (claim `modo` do JWT — rodada 4, B4-1).
const (
	ModoAtleta    = "atleta"
	ModoInstrutor = "instrutor"
)

// PapeisUsuario é o retrato dos papéis de uma conta: o tipo (atleta | academia) e, para atleta, o
// pedido de instrutor (linha em `instrutores`), se existir. Lido por UsuarioRepository.PapeisDoUsuario.
type PapeisUsuario struct {
	IDUsuario uint
	Conta     string // PerfilAtleta | PerfilAcademia
	// InstrutorID é o id em `instrutores` (0 = nunca pediu). InstrutorStatus: pendente | aprovada |
	// rejeitada ("" = nunca pediu).
	InstrutorID     uint
	InstrutorStatus string
	// InstrutorAcademiaID: academia à qual o instrutor está vinculado (0 = autônomo/sem pedido).
	InstrutorAcademiaID     uint
	InstrutorMotivoRejeicao *string
	// AcademiaID / AcademiaStatus: registro em `academias` da conta de academia (0/"" se não houver).
	AcademiaID     uint
	AcademiaStatus string
}

// TemPedidoInstrutor diz se a conta já pediu o modo instrutor (qualquer status).
func (p *PapeisUsuario) TemPedidoInstrutor() bool { return p.InstrutorID != 0 }

// InstrutorAprovado diz se o modo instrutor está liberado (pedido aprovado pelo admin).
func (p *PapeisUsuario) InstrutorAprovado() bool {
	return p.InstrutorID != 0 && p.InstrutorStatus == CadastroAprovado
}

// ModosDisponiveis lista os modos em que a conta pode operar: conta de atleta tem sempre `atleta` e
// ganha `instrutor` com o pedido aprovado; conta de academia não tem modos (nil).
func (p *PapeisUsuario) ModosDisponiveis() []string {
	if p.Conta != PerfilAtleta {
		return nil
	}
	if p.InstrutorAprovado() {
		return []string{ModoAtleta, ModoInstrutor}
	}
	return []string{ModoAtleta}
}

// ModoValido diz se `modo` é um dos modos conhecidos (atleta | instrutor).
func ModoValido(modo string) bool { return modo == ModoAtleta || modo == ModoInstrutor }

// PodeUsarModo diz se a conta pode operar no modo informado AGORA (conferido no banco a cada troca
// de modo e a cada rota de instrutor): atleta sempre; instrutor só com o pedido aprovado.
func (p *PapeisUsuario) PodeUsarModo(modo string) bool {
	for _, m := range p.ModosDisponiveis() {
		if m == modo {
			return true
		}
	}
	return false
}

// ModoEfetivo normaliza o claim `modo` do token: vazio (tokens antigos) = atleta. Conta de academia
// não tem modos ("").
func (p *PapeisUsuario) ModoEfetivo(claim string) string {
	if p.Conta != PerfilAtleta {
		return ""
	}
	if claim == "" {
		return ModoAtleta
	}
	return claim
}

// InstrutorSessao é o status do pedido de instrutor devolvido no login e em /usuarios/me
// (null quando a conta nunca pediu o modo instrutor).
type InstrutorSessao struct {
	Status         string  `json:"status"` // pendente | aprovada | rejeitada
	MotivoRejeicao *string `json:"motivo_rejeicao"`
}

// Sessao devolve o bloco de modos da sessão (login, /usuarios/me, troca de modo).
func (p *PapeisUsuario) Sessao(modoAtivo string) SessaoModos {
	s := SessaoModos{Perfil: p.Conta, ModoAtivo: p.ModoEfetivo(modoAtivo), ModosDisponiveis: p.ModosDisponiveis()}
	if s.ModosDisponiveis == nil {
		s.ModosDisponiveis = []string{}
	}
	if p.TemPedidoInstrutor() {
		s.Instrutor = &InstrutorSessao{Status: p.InstrutorStatus, MotivoRejeicao: p.InstrutorMotivoRejeicao}
	}
	return s
}

// SessaoModos é o contrato de modos com o front (embutido nas respostas de sessão):
//   - perfil: tipo de conta, `atleta` | `academia`;
//   - modo_ativo: modo do token atual (`atleta` | `instrutor`; "" para academia). Se não estiver em
//     modos_disponiveis (instrutor rejeitado/revogado depois da troca), o front deve trocar para atleta;
//   - modos_disponiveis: `[]` (academia), `["atleta"]` ou `["atleta","instrutor"]`;
//   - instrutor: status do pedido de instrutor ({status, motivo_rejeicao}) ou null se nunca pediu.
type SessaoModos struct {
	Perfil           string           `json:"perfil"`
	ModoAtivo        string           `json:"modo_ativo"`
	ModosDisponiveis []string         `json:"modos_disponiveis"`
	Instrutor        *InstrutorSessao `json:"instrutor"`
}
