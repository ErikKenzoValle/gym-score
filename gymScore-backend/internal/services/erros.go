package services

import (
	"errors"
	"net/http"
	"time"

	"gymScore-backend/internal/models"
)

// ErroNegocio é um erro de regra de negócio cuja mensagem pode ser mostrada ao cliente.
// Qualquer outro erro (banco, HTTP externo...) é tratado como interno: vai para o log
// e o cliente recebe uma mensagem genérica.
type ErroNegocio struct {
	Status   int
	Mensagem string
	// Codigo (opcional) é um identificador estável para o front (ex.: conta_banida, conta_suspensa);
	// Dados (opcional) vai junto na resposta. Nunca colocar aqui nada que o cliente não possa ver.
	Codigo string
	Dados  any
}

func (e *ErroNegocio) Error() string { return e.Mensagem }

func erro(status int, msg string) *ErroNegocio { return &ErroNegocio{Status: status, Mensagem: msg} }

func erroRequisicao(msg string) *ErroNegocio { return erro(http.StatusBadRequest, msg) }
func erroProibido(msg string) *ErroNegocio   { return erro(http.StatusForbidden, msg) }
func erroNaoEncontrado(msg string) *ErroNegocio {
	return erro(http.StatusNotFound, msg)
}
func erroConflito(msg string) *ErroNegocio { return erro(http.StatusConflict, msg) }

// Códigos de erro da moderação (campo `codigo` da resposta).
const (
	CodigoContaBanida   = "conta_banida"
	CodigoContaSuspensa = "conta_suspensa"
)

// MensagemContaBanida / MensagemContaSuspensa são os textos do 403 (também usados pelo AuthMiddleware).
const (
	MensagemContaBanida   = "Conta banida. Você pode recorrer da decisão pela tela de login"
	MensagemContaSuspensa = "Conta suspensa: esta ação fica bloqueada até o fim da suspensão"
)

// ErroContaBanida é o 403 do login do banido (Q1), com os detalhes do banimento quando disponíveis.
func ErroContaBanida(info *models.BanimentoInfo) *ErroNegocio {
	e := erro(http.StatusForbidden, MensagemContaBanida)
	e.Codigo = CodigoContaBanida
	if info != nil {
		e.Dados = info
	}
	return e
}

// ErroContaSuspensa é o 403 das ações de participação/gestão durante a suspensão (Q2).
func ErroContaSuspensa(fim *time.Time) *ErroNegocio {
	e := erro(http.StatusForbidden, MensagemContaSuspensa)
	e.Codigo = CodigoContaSuspensa
	e.Dados = map[string]any{"suspenso_ate": fim}
	return e
}

// ErroCadastroNaoAprovado: academia/instrutor ainda pendente (ou rejeitado) na aprovação do admin.
var ErroCadastroNaoAprovado = erro(http.StatusForbidden, "Seu cadastro ainda não foi aprovado pela equipe GymScore")

// Códigos dos modos da conta (rodada 4).
const (
	// CodigoModoIndisponivel: a conta não pode usar o modo pedido/do token (instrutor pendente, rejeitado
	// ou sem pedido). O front volta ao modo atleta e mostra o status do pedido.
	CodigoModoIndisponivel = "modo_indisponivel"
	// CodigoModoIncorreto: a ação é de outro modo (ex.: inscrever-se estando no modo instrutor). O front
	// oferece a troca de modo.
	CodigoModoIncorreto = "modo_incorreto"
)

// ErroModoIndisponivel é o 403 de quem pede/usa o modo instrutor sem o pedido aprovado.
func ErroModoIndisponivel(p *models.PapeisUsuario) *ErroNegocio {
	msg := "Você não tem o modo instrutor. Faça o pedido de instrutor no seu perfil"
	status := ""
	if p != nil && p.TemPedidoInstrutor() {
		status = p.InstrutorStatus
		switch p.InstrutorStatus {
		case models.CadastroPendente:
			msg = "Seu pedido de instrutor está em análise pela equipe GymScore"
		case models.CadastroRejeitado:
			msg = "Seu pedido de instrutor foi recusado"
		}
	}
	e := erro(http.StatusForbidden, msg)
	e.Codigo = CodigoModoIndisponivel
	e.Dados = map[string]any{"status_instrutor": status}
	return e
}

// ErroModoIncorreto é o 403 de uma ação de outro modo (o claim do token não é o modo da ação).
func ErroModoIncorreto(modoNecessario string) *ErroNegocio {
	msg := "Troque para o modo atleta para realizar esta ação"
	if modoNecessario == models.ModoInstrutor {
		msg = "Troque para o modo instrutor para realizar esta ação"
	}
	e := erro(http.StatusForbidden, msg)
	e.Codigo = CodigoModoIncorreto
	e.Dados = map[string]any{"modo_necessario": modoNecessario}
	return e
}

// ErroSaldoInsuficiente é devolvido quando um débito não pode ser feito.
var ErroSaldoInsuficiente = erro(http.StatusUnprocessableEntity, "Saldo insuficiente")

// AsErroNegocio extrai um *ErroNegocio da cadeia de erros.
func AsErroNegocio(err error) (*ErroNegocio, bool) {
	var e *ErroNegocio
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
