package cli

import (
	"strings"
	"testing"
)

func TestParse_RecusaSenhaPorArgumentoEExtras(t *testing.T) {
	ruins := [][]string{
		{},
		{"criar-superadmin", "--email", "a@b.com", "--nome", "Ana", "--senha", "segredo-123456"},
		{"criar-superadmin", "--email", "a@b.com", "--nome", "Ana", "--password=segredo-123456"},
		{"criar-superadmin", "--email", "a@b.com", "--nome", "Ana", "segredo-123456"}, // posicional
		{"criar-superadmin", "--email", "nao-e-email", "--nome", "Ana"},
		{"criar-superadmin", "--email", "a@b.com"},               // sem nome
		{"resetar-senha", "--email", "a@b.com", "--nome", "Ana"}, // flag de outro comando
		{"apagar-tudo", "--email", "a@b.com"},
	}
	for _, args := range ruins {
		if _, err := parse(args); err == nil {
			t.Errorf("deveria recusar %v", args)
		}
	}
	c, err := parse([]string{"criar-superadmin", "--email", " Admin@Exemplo.com ", "--nome", "Ana Lima"})
	if err != nil || c.email != "admin@exemplo.com" || c.nomeAdmin != "Ana Lima" {
		t.Fatalf("parse válido: %+v %v", c, err)
	}
	if _, err := parse([]string{"resetar-senha", "--email=admin@exemplo.com"}); err != nil {
		t.Fatal(err)
	}
}

func TestLerSenhaConfirmada(t *testing.T) {
	casos := []struct {
		entrada string
		ok      bool
	}{
		{"senha-bem-longa-1\nsenha-bem-longa-1\n", true},
		{"senha-bem-longa-1\nsenha-bem-longa-2\n", false}, // não confere
		{"curta\ncurta\n", false},                         // < 12
		{"senha-bem-longa-1\n", false},                    // sem confirmação
		{"", false},
	}
	for _, c := range casos {
		s, err := lerSenhaConfirmada(LeitorDeTexto(strings.NewReader(c.entrada)))
		if (err == nil) != c.ok {
			t.Errorf("entrada %q: err=%v, esperava ok=%v", c.entrada, err, c.ok)
		}
		if c.ok && s != "senha-bem-longa-1" {
			t.Errorf("senha lida: %q", s)
		}
	}
}
