package cli

import (
	"errors"
	"testing"
)

func TestSeedTeste_RecusaProducaoSemTocarNoBanco(t *testing.T) {
	for _, env := range []string{"production", "prod", " Production ", "PROD"} {
		// db nil: se tentasse usar o banco, entraria em pânico
		if _, err := SeedTeste(env, nil, 10, nil); !errors.Is(err, ErrSeedEmProducao) {
			t.Errorf("APP_ENV=%q: esperava ErrSeedEmProducao, veio %v", env, err)
		}
	}
	for _, env := range []string{"", "development", "test", "staging"} {
		if EhProducao(env) {
			t.Errorf("APP_ENV=%q não é produção", env)
		}
	}
}

func TestDocumentosFicticiosComDigitosVerificadores(t *testing.T) {
	if c := cpfFicticio("111444777"); c != "111.444.777-35" {
		t.Errorf("cpf: %s", c)
	}
	if c := cnpjFicticio("112223330001"); c != "11.222.333/0001-81" {
		t.Errorf("cnpj: %s", c)
	}
}
