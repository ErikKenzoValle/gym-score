package utils_test

import (
	"strings"
	"testing"

	"gymScore-backend/pkg/utils"
)

type exemplo struct {
	Nome  string `json:"nome" validate:"required,min=3"`
	Senha string `json:"senha" validate:"required,min=8"`
}

func TestValidarStruct(t *testing.T) {
	if msg := utils.ValidarStruct(exemplo{Nome: "Ana", Senha: "12345678"}); msg != "" {
		t.Fatalf("esperava válido, obteve %q", msg)
	}
	msg := utils.ValidarStruct(exemplo{Nome: "Ana", Senha: "1234567"})
	if !strings.Contains(msg, "senha") || !strings.Contains(msg, "8") {
		t.Errorf("mensagem deveria citar o campo json 'senha' e o mínimo 8: %q", msg)
	}
	if msg := utils.ValidarStruct(exemplo{Senha: "12345678"}); !strings.Contains(msg, "nome") {
		t.Errorf("esperava erro de campo obrigatório 'nome': %q", msg)
	}
}

func TestCentavosParaReais(t *testing.T) {
	if got := utils.CentavosParaReais(1999); got != 19.99 {
		t.Errorf("esperava 19.99, obteve %v", got)
	}
}
