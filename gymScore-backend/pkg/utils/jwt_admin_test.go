package utils

import (
	"testing"
	"time"
)

func TestTokenAdmin_AudienciaESegredo(t *testing.T) {
	const segAdmin, segUser = "segredo-admin-de-teste-0123456789", "segredo-usuario-de-teste-0123456"
	tk, err := GerarTokenAdmin(7, 3, segAdmin, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ValidarTokenAdmin(tk, segAdmin)
	if err != nil || c.AdminID != 7 || c.Versao != 3 {
		t.Fatalf("token admin válido recusado: %v %+v", err, c)
	}
	if _, err := ValidarTokenAdmin(tk, segUser); err == nil {
		t.Error("token admin aceito com outro segredo")
	}
	// mesmo com o MESMO segredo, token admin não vale como usuário e vice-versa
	if _, err := ValidarToken(tk, segAdmin); err == nil {
		t.Error("token admin aceito como token de usuário")
	}
	tu, _ := GerarToken(7, "a@b.c", 0, segAdmin)
	if _, err := ValidarTokenAdmin(tu, segAdmin); err == nil {
		t.Error("token de usuário (sem aud) aceito no painel admin")
	}
	exp, _ := GerarTokenAdmin(7, 0, segAdmin, -time.Minute)
	if _, err := ValidarTokenAdmin(exp, segAdmin); err == nil {
		t.Error("token admin expirado aceito")
	}
}
