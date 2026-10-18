package services

import (
	"errors"
	"net/http"
)

// ErroNegocio é um erro de regra de negócio cuja mensagem pode ser mostrada ao cliente.
// Qualquer outro erro (banco, HTTP externo...) é tratado como interno: vai para o log
// e o cliente recebe uma mensagem genérica.
type ErroNegocio struct {
	Status   int
	Mensagem string
}

func (e *ErroNegocio) Error() string { return e.Mensagem }

func erro(status int, msg string) *ErroNegocio { return &ErroNegocio{Status: status, Mensagem: msg} }

func erroRequisicao(msg string) *ErroNegocio { return erro(http.StatusBadRequest, msg) }
func erroProibido(msg string) *ErroNegocio   { return erro(http.StatusForbidden, msg) }
func erroNaoEncontrado(msg string) *ErroNegocio {
	return erro(http.StatusNotFound, msg)
}
func erroConflito(msg string) *ErroNegocio { return erro(http.StatusConflict, msg) }

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
