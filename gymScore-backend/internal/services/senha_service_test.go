package services_test

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
)

func TestHashToken_Sha256Hex(t *testing.T) {
	h := services.HashToken("abc")
	if h != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("SHA-256 de 'abc' incorreto: %s", h)
	}
	if len(services.HashToken("qualquer")) != 64 {
		t.Error("hash deve ter 64 caracteres")
	}
}

func TestSenhaService_TokenGuardaSoHashEExpira(t *testing.T) {
	usuarios := novoMockUsuarioRepo()
	_ = usuarios.Criar(&models.Usuario{Nome: "Ana", Email: "ana@gymscore.test"})
	tokens := &mockResetRepo{senhas: map[uint]string{}}
	mail := &mailerCaptura{}
	agora := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	svc := services.NovoSenhaServiceSincrono(usuarios, tokens, mail, "http://app.test/", func() time.Time { return agora })

	svc.EsqueciSenha("desconhecido@gymscore.test") // silencioso
	if len(tokens.tokens) != 0 || len(mail.corpos) != 0 {
		t.Fatal("e-mail desconhecido não gera token nem mensagem")
	}
	svc.EsqueciSenha("ana@gymscore.test")
	if len(tokens.tokens) != 1 || len(mail.corpos) != 1 {
		t.Fatalf("esperava 1 token e 1 e-mail, obteve %d e %d", len(tokens.tokens), len(mail.corpos))
	}
	m := regexp.MustCompile(`http://app\.test/redefinir-senha\?token=([^\s]+)`).FindStringSubmatch(mail.corpos[0])
	if m == nil {
		t.Fatalf("link inesperado no e-mail: %s", mail.corpos[0])
	}
	token, _ := url.QueryUnescape(m[1])
	if tokens.tokens[0].TokenHash == token || tokens.tokens[0].TokenHash != services.HashToken(token) {
		t.Error("o banco deve guardar apenas o hash do token")
	}
	if got := tokens.tokens[0].ExpiraEm.Sub(agora); got != 30*time.Minute {
		t.Errorf("validade deveria ser 30 min, obteve %v", got)
	}
	if strings.Contains(mail.corpos[0], tokens.tokens[0].TokenHash) {
		t.Error("o hash não pode aparecer no e-mail")
	}
}

func TestSenhaService_UsoUnicoEExpiracao(t *testing.T) {
	usuarios := novoMockUsuarioRepo()
	_ = usuarios.Criar(&models.Usuario{Nome: "Ana", Email: "ana@gymscore.test"})
	tokens := &mockResetRepo{senhas: map[uint]string{}}
	mail := &mailerCaptura{}
	relogio := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	svc := services.NovoSenhaServiceSincrono(usuarios, tokens, mail, "http://app.test", func() time.Time { return relogio })

	pegarToken := func() string {
		svc.EsqueciSenha("ana@gymscore.test")
		m := regexp.MustCompile(`token=([^\s]+)`).FindStringSubmatch(mail.corpos[len(mail.corpos)-1])
		tk, _ := url.QueryUnescape(m[1])
		return tk
	}

	t1 := pegarToken()
	if err := svc.RedefinirSenha(&models.RedefinirSenhaRequest{Token: t1, NovaSenha: "curta"}); err == nil {
		t.Error("senha curta deve falhar")
	}
	if err := svc.RedefinirSenha(&models.RedefinirSenhaRequest{Token: t1, NovaSenha: "nova-senha-123"}); err != nil {
		t.Fatalf("token válido deveria funcionar: %v", err)
	}
	if err := svc.RedefinirSenha(&models.RedefinirSenhaRequest{Token: t1, NovaSenha: "outra-senha-123"}); err == nil {
		t.Error("token já usado deveria falhar")
	}

	t2 := pegarToken()
	relogio = relogio.Add(31 * time.Minute) // passou da validade
	if err := svc.RedefinirSenha(&models.RedefinirSenhaRequest{Token: t2, NovaSenha: "nova-senha-123"}); err == nil {
		t.Error("token expirado deveria falhar")
	}
	if err := svc.RedefinirSenha(&models.RedefinirSenhaRequest{Token: "inventado-inventado-inventado", NovaSenha: "nova-senha-123"}); err == nil {
		t.Error("token inexistente deveria falhar")
	}
}
