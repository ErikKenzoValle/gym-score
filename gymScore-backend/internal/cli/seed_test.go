package cli

import (
	"context"
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

// QA56-2: o seed automático é lista de permissão sobre o APP_ENV EXPLÍCITO ("" = variável ausente).
func TestPermiteSeedAuto_ListaDePermissao(t *testing.T) {
	for _, env := range []string{"development", "dev", "local", "test", " Development ", "TEST", "Local\n"} {
		if ok, motivo := PermiteSeedAuto(env); !ok {
			t.Errorf("APP_ENV=%q deveria permitir: %s", env, motivo)
		}
	}
	for _, env := range []string{"", "  ", "production", "prod", "staging", "homolog", "producao", "develop", "testing"} {
		if ok, motivo := PermiteSeedAuto(env); ok || motivo == "" {
			t.Errorf("APP_ENV=%q não deveria permitir (motivo=%q)", env, motivo)
		}
	}
}

// Recusas não tocam no banco: db nil não é usado.
func TestSeedTesteAuto_RecusaSemTocarNoBanco(t *testing.T) {
	for _, env := range []string{"", "staging", "production"} {
		r, err := SeedTesteAuto(context.Background(), env, true, nil, 10, nil)
		if err != nil || r.Executado || r.Motivo == "" {
			t.Errorf("APP_ENV=%q: %+v %v", env, r, err)
		}
	}
}
