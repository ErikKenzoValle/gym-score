package utils_test

import (
	"testing"

	"gymScore-backend/pkg/utils"
)

// ─── Testes de ValidarEmail ───────────────────────────────────────────────────

func TestValidarEmail_Validos(t *testing.T) {
	emails := []string{
		"usuario@example.com",
		"nome.sobrenome@empresa.com.br",
		"user+tag@domain.org",
		"test123@test.io",
	}

	for _, email := range emails {
		if !utils.ValidarEmail(email) {
			t.Errorf("esperava e-mail válido para: %s", email)
		}
	}
}

func TestValidarEmail_Invalidos(t *testing.T) {
	emails := []string{
		"email-sem-arroba",
		"@dominio.com",
		"usuario@",
		"usuario@.com",
		"",
		"usuario@dominio",
	}

	for _, email := range emails {
		if utils.ValidarEmail(email) {
			t.Errorf("esperava e-mail inválido para: %s", email)
		}
	}
}

// ─── Testes de ValidarCPF ────────────────────────────────────────────────────

func TestValidarCPF(t *testing.T) {
	validos := []string{"529.982.247-25", "52998224725"}
	for _, cpf := range validos {
		if !utils.ValidarCPF(cpf) {
			t.Errorf("esperava CPF válido: %s", cpf)
		}
	}
	invalidos := []string{"", "111.111.111-11", "529.982.247-26", "123", "abc.def.ghi-jk", "529.982.247-2a"}
	for _, cpf := range invalidos {
		if utils.ValidarCPF(cpf) {
			t.Errorf("esperava CPF inválido: %q", cpf)
		}
	}
}
