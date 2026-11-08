package services

import (
	"strings"
	"testing"
)

func TestMascararCPF(t *testing.T) {
	cpf := "123.456.789-01"
	if got := MascararCPF(&cpf); got != "***.456.789-**" {
		t.Errorf("mascara: %q", got)
	}
	cru := "12345678901"
	if got := MascararCPF(&cru); got != "***.456.789-**" {
		t.Errorf("mascara sem pontuação: %q", got)
	}
	if MascararCPF(nil) != "" {
		t.Error("nil deveria virar vazio")
	}
	ruim := "123"
	if strings.Contains(MascararCPF(&ruim), "123") {
		t.Error("CPF malformado não pode vazar")
	}
}

func TestMotivoValido(t *testing.T) {
	if _, err := motivoValido("  ab "); err == nil {
		t.Error("motivo curto aceito")
	}
	if _, err := motivoValido(strings.Repeat("é", 501)); err == nil {
		t.Error("motivo longo aceito")
	}
	if m, err := motivoValido("  conferido no Asaas  "); err != nil || m != "conferido no Asaas" {
		t.Errorf("motivo válido: %q %v", m, err)
	}
}

func TestGerarSenhaTemporaria(t *testing.T) {
	vistos := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := GerarSenhaTemporaria()
		if err != nil || len(s) < SenhaAdminMinima {
			t.Fatalf("senha temporária %q: %v", s, err)
		}
		if strings.ContainsAny(s, "0O1lI") {
			t.Errorf("caractere ambíguo em %q", s)
		}
		if vistos[s] {
			t.Fatalf("senha repetida: %q", s)
		}
		vistos[s] = true
	}
}

func TestLimitarNaoQuebraUTF8(t *testing.T) {
	s := strings.Repeat("ã", 10) // 20 bytes
	if got := limitar(s, 5); got != "ãã" {
		t.Errorf("limitar: %q", got)
	}
	if limitar("abc", 10) != "abc" {
		t.Error("string curta alterada")
	}
}

func TestAtorSuperadminNasAcoesRestritas(t *testing.T) {
	s := &adminService{}
	comum := AtorAdmin{ID: 1, Papel: "admin"}
	if _, err := s.ReembolsarSaque(comum, 1, "motivo ok"); err != ErroSuperadmin {
		t.Errorf("reembolsar por admin comum: %v", err)
	}
	if _, err := s.MarcarSaquePago(comum, 1, "motivo ok"); err != ErroSuperadmin {
		t.Errorf("marcar pago por admin comum: %v", err)
	}
	if _, err := s.AlterarAtivoAdmin(comum, 2, false, "motivo ok"); err != ErroSuperadmin {
		t.Errorf("desativar por admin comum: %v", err)
	}
	if _, err := s.TrocarPapelAdmin(comum, 2, "admin", "motivo ok"); err != ErroSuperadmin {
		t.Errorf("trocar papel por admin comum: %v", err)
	}
	if _, err := s.ResetarSenhaAdmin(comum, 2, "motivo ok"); err != ErroSuperadmin {
		t.Errorf("resetar senha por admin comum: %v", err)
	}
	if _, err := s.CriarAdmin(comum, nil); err != ErroSuperadmin {
		t.Errorf("criar admin por admin comum: %v", err)
	}
	super := AtorAdmin{ID: 1, Papel: "superadmin"}
	if _, err := s.AlterarAtivoAdmin(super, 1, false, "motivo ok"); err == nil || !strings.Contains(err.Error(), "própria") {
		t.Errorf("desativar a si mesmo deveria ser recusado: %v", err)
	}
}
