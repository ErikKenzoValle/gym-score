package services_test

import (
	"net/http"
	"testing"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
)

func novoAmizadeSvc() (services.AmizadeService, *mockAmizadeRepo) {
	usuarios := novoMockUsuarioRepo()
	for i := 1; i <= 3; i++ {
		_ = usuarios.Criar(&models.Usuario{Nome: "U"})
	}
	am := &mockAmizadeRepo{}
	return services.NovoAmizadeService(am, usuarios, services.NotificadorNulo{}), am
}

func TestAmizade_SoDestinatarioAceita(t *testing.T) {
	svc, repo := novoAmizadeSvc()
	if err := svc.AdicionarAmigo(1, 2); err != nil {
		t.Fatal(err)
	}
	// o remetente (1) tenta aceitar a própria solicitação; o terceiro (3) também
	if err := svc.AceitarAmizade(1, 2); err == nil || statusDe(t, err) != http.StatusNotFound {
		t.Errorf("remetente não pode aceitar: %v", err)
	}
	if err := svc.AceitarAmizade(3, 1); err == nil {
		t.Error("terceiro não pode aceitar")
	}
	if repo.rels[0].Status != models.StatusAmizadePendente {
		t.Fatalf("deveria continuar pendente: %s", repo.rels[0].Status)
	}
	if err := svc.AceitarAmizade(2, 1); err != nil {
		t.Errorf("destinatário deveria aceitar: %v", err)
	}
}

func TestAmizade_Regras(t *testing.T) {
	svc, _ := novoAmizadeSvc()
	if err := svc.AdicionarAmigo(1, 1); err == nil || statusDe(t, err) != http.StatusBadRequest {
		t.Errorf("autoamizade deveria dar 400: %v", err)
	}
	if err := svc.AdicionarAmigo(1, 99); err == nil || statusDe(t, err) != http.StatusNotFound {
		t.Errorf("usuário inexistente deveria dar 404: %v", err)
	}
	_ = svc.AdicionarAmigo(1, 2)
	if err := svc.AdicionarAmigo(1, 2); err == nil || statusDe(t, err) != http.StatusConflict {
		t.Errorf("duplicada deveria dar 409: %v", err)
	}
	if err := svc.AdicionarAmigo(2, 1); err == nil || statusDe(t, err) != http.StatusConflict {
		t.Errorf("direção oposta deveria dar 409: %v", err)
	}
}

func TestAmizade_RecusadaPodeSerReenviada(t *testing.T) {
	svc, repo := novoAmizadeSvc()
	_ = svc.AdicionarAmigo(1, 2)
	if err := svc.RecusarAmizade(2, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.AdicionarAmigo(2, 1); err != nil { // agora quem reenvia é o 2
		t.Fatalf("reenvio deveria funcionar: %v", err)
	}
	if r := repo.rels[0]; r.IDUsuario != 2 || r.IDAmigo != 1 || r.Status != models.StatusAmizadePendente {
		t.Errorf("relação deveria virar 2->1 pendente: %+v", r)
	}
}
