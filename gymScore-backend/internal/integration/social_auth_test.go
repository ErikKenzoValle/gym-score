//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"gymScore-backend/database"
	"gymScore-backend/internal/models"
)

// ─── Amizades ────────────────────────────────────────────────────────────────

func TestAmizade_SoODestinatarioAceita(t *testing.T) {
	a, b, c := novoUsuario(t), novoUsuario(t), novoUsuario(t)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)

	// o remetente não pode aceitar a própria solicitação; um terceiro também não
	esperar(t, api(t, "POST", "/api/amigos/aceitar", a.Token, map[string]any{"id_amigo": b.ID}), 404)
	esperar(t, api(t, "POST", "/api/amigos/aceitar", c.Token, map[string]any{"id_amigo": a.ID}), 404)
	esperar(t, api(t, "POST", "/api/amigos/aceitar", c.Token, map[string]any{"id_amigo": b.ID}), 404)
	var st string
	db.Model(&models.Amizade{}).Select("status").Where("id_usuario = ? AND id_amigo = ?", a.ID, b.ID).Scan(&st)
	if st != "pendente" {
		t.Fatalf("amizade deveria seguir pendente, está %q", st)
	}
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 404) // já processada
}

func TestAmizade_DuplicataEmQualquerDirecaoEAutoamizade(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": a.ID}), 400)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": 99999999}), 404)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 409)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", b.Token, map[string]any{"id_amigo": a.ID}), 409) // direção oposta
	var n int64
	db.Model(&models.Amizade{}).Where("(id_usuario = ? AND id_amigo = ?) OR (id_usuario = ? AND id_amigo = ?)", a.ID, b.ID, b.ID, a.ID).Count(&n)
	if n != 1 {
		t.Errorf("deveria existir uma única linha para o par, existem %d", n)
	}
}

func TestAmizade_RecusarReenviarERemover(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/recusar", a.Token, map[string]any{"id_amigo": b.ID}), 404) // remetente não recusa
	esperar(t, api(t, "POST", "/api/amigos/recusar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	// recusada some da lista
	lista := esperar(t, api(t, "GET", "/api/amigos", a.Token, nil), 200)
	if len(lista.Body["data"].([]any)) != 0 {
		t.Errorf("solicitação recusada não deve listar: %s", lista.Raw)
	}
	// pode reenviar; agora b aceita e depois qualquer um remove
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/remover", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/remover", b.Token, map[string]any{"id_amigo": a.ID}), 404)
}

func TestAmizade_ListaSemDadosSensiveis(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	amigos(t, a, b)
	r := esperar(t, api(t, "GET", "/api/amigos", a.Token, nil), 200)
	if strings.Contains(r.Raw, "@gymscore.test") || strings.Contains(r.Raw, "cpf") || strings.Contains(r.Raw, "email") {
		t.Errorf("lista de amigos vaza dados sensíveis: %s", r.Raw)
	}
	itens := r.Body["data"].([]any)
	if len(itens) != 1 || uint(itens[0].(map[string]any)["id"].(float64)) != b.ID {
		t.Errorf("lista inesperada: %s", r.Raw)
	}
	// rota antiga com :id não existe mais
	if res := api(t, "GET", fmt.Sprintf("/api/amigos/%d", b.ID), a.Token, nil); res.Status != 404 {
		t.Errorf("GET /api/amigos/:id deveria ter sido removida, status %d", res.Status)
	}
}

func TestAmizade_SolicitacaoRecebidaFlag(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	ra := esperar(t, api(t, "GET", "/api/amigos", a.Token, nil), 200).Body["data"].([]any)[0].(map[string]any)
	rb := esperar(t, api(t, "GET", "/api/amigos", b.Token, nil), 200).Body["data"].([]any)[0].(map[string]any)
	if ra["recebida"] != false || rb["recebida"] != true {
		t.Errorf("flag recebida errada: remetente=%v destinatário=%v", ra["recebida"], rb["recebida"])
	}
}

// ─── Usuários ────────────────────────────────────────────────────────────────

func TestUsuario_PerfilPublicoNaoVazaDados(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	me := esperar(t, api(t, "GET", "/api/usuarios/me", a.Token, nil), 200).data()
	if me["email"] != a.Email || me["cpf"] == nil || me["data_nascimento"] != "1995-03-15" {
		t.Errorf("/me deveria trazer o perfil completo: %v", me)
	}
	pub := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d", b.ID), a.Token, nil), 200)
	for _, proibido := range []string{"cpf", "email", "saldo", "senha", "data_nascimento", "genero"} {
		if strings.Contains(pub.Raw, `"`+proibido) {
			t.Errorf("perfil público vaza %q: %s", proibido, pub.Raw)
		}
	}
	esperar(t, api(t, "GET", "/api/usuarios/99999999", a.Token, nil), 404)
	esperar(t, api(t, "GET", "/api/usuarios/me", "", nil), 401)
}

func TestUsuario_ListagemTotalRemovida(t *testing.T) {
	a := novoUsuario(t)
	if r := api(t, "GET", "/api/usuarios", a.Token, nil); r.Status != 404 && r.Status != 405 {
		t.Errorf("GET /api/usuarios deveria não existir, status %d", r.Status)
	}
}

func TestUsuario_Busca(t *testing.T) {
	a := novoUsuario(t)
	alvo := novoUsuario(t) // Nome "TesteN"
	esperar(t, api(t, "GET", "/api/usuarios/busca?q=a", a.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/usuarios/busca", a.Token, nil), 400)
	r := esperar(t, api(t, "GET", "/api/usuarios/busca?q="+url.QueryEscape(alvo.Nome), a.Token, nil), 200)
	achou := false
	for _, it := range r.Body["data"].([]any) {
		m := it.(map[string]any)
		if uint(m["id"].(float64)) == alvo.ID {
			achou = true
		}
		if uint(m["id"].(float64)) == a.ID {
			t.Error("a busca não deve devolver o próprio usuário")
		}
		if len(m) != 3 {
			t.Errorf("busca deve devolver só id/nome/sobrenome: %v", m)
		}
	}
	if !achou {
		t.Errorf("deveria encontrar %s: %s", alvo.Nome, r.Raw)
	}
	// curingas do LIKE são literais: "%%" não casa com todo mundo
	rr := esperar(t, api(t, "GET", "/api/usuarios/busca?q=%25%25", a.Token, nil), 200)
	if len(rr.Body["data"].([]any)) != 0 {
		t.Errorf("curinga '%%%%' não deveria casar nada: %s", rr.Raw)
	}
	// máx. 20 resultados
	for i := 0; i < 22; i++ {
		_ = novoUsuario(t)
	}
	rl := esperar(t, api(t, "GET", "/api/usuarios/busca?q=Teste", a.Token, nil), 200)
	if n := len(rl.Body["data"].([]any)); n > 20 {
		t.Errorf("busca devolveu %d resultados (máx. 20)", n)
	}
}

func TestUsuario_CadastroValidacoes(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"nome": "Ana", "sobrenome": "Silva", "cpf": cpfValido(),
			"email": fmt.Sprintf("qa+val%d@gymscore.test", seq.Add(1)), "senha": "12345678",
			"data_nascimento": "1990-01-01", "genero": "F"}
	}
	caso := func(nome string, mut func(m map[string]any), esperado int) {
		m := base()
		mut(m)
		if r := api(t, "POST", "/api/usuarios", "", m); r.Status != esperado {
			t.Errorf("%s: esperava %d, obteve %d (%s)", nome, esperado, r.Status, r.Raw)
		}
	}
	caso("senha curta (7)", func(m map[string]any) { m["senha"] = "1234567" }, 400)
	caso("e-mail inválido", func(m map[string]any) { m["email"] = "xx" }, 400)
	caso("cpf inválido", func(m map[string]any) { m["cpf"] = "111.111.111-11" }, 400)
	caso("cpf sem máscara", func(m map[string]any) { m["cpf"] = "52998224725" }, 400)
	caso("gênero inválido", func(m map[string]any) { m["genero"] = "X" }, 400)
	caso("data inválida", func(m map[string]any) { m["data_nascimento"] = "15/03/1995" }, 400)
	caso("ok", func(m map[string]any) {}, 201)

	u := novoUsuario(t)
	dup := base()
	dup["email"] = u.Email
	if r := api(t, "POST", "/api/usuarios", "", dup); r.Status != 409 {
		t.Errorf("e-mail duplicado deveria ser 409, obteve %d", r.Status)
	}
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "errada-123"}), 401)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": "nao-existe@gymscore.test", "senha": "errada-123"}), 401)
}

func TestUsuario_PerfilESenha(t *testing.T) {
	u := usuarioComSaldo(t, 1500)
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"nome": "Novo"}), 200)
	me := esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 200).data()
	if me["nome"] != "Novo" || me["saldo_centavos"].(float64) != 1500 {
		t.Errorf("atualizar perfil não pode tocar no saldo: %v", me)
	}
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"genero": "Z"}), 400)
	// id no corpo é ignorado: não altera outro usuário
	outro := novoUsuario(t)
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"nome": "Hack", "id": outro.ID, "id_usuario": outro.ID}), 200)
	if o := esperar(t, api(t, "GET", "/api/usuarios/me", outro.Token, nil), 200).data(); o["nome"] == "Hack" {
		t.Error("perfil de outro usuário foi alterado")
	}

	esperar(t, api(t, "PATCH", "/api/usuarios/senha", u.Token, map[string]any{"senha_atual": "errada", "nova_senha": "nova-senha-123"}), 400)
	esperar(t, api(t, "PATCH", "/api/usuarios/senha", u.Token, map[string]any{"senha_atual": u.Senha, "nova_senha": "curta"}), 400)
	troca := esperar(t, api(t, "PATCH", "/api/usuarios/senha", u.Token, map[string]any{"senha_atual": u.Senha, "nova_senha": "nova-senha-123"}), 200)
	u.Token = troca.data()["token"].(string) // o token antigo foi revogado; a resposta traz o novo
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "nova-senha-123"}), 200)
	if s := saldo(t, u); s != 1500 {
		t.Errorf("trocar a senha não pode mexer no saldo: %d", s)
	}
}

func TestAuth_RotasProtegidasExigemToken(t *testing.T) {
	for _, rota := range []struct{ m, p string }{
		{"GET", "/api/desafios"}, {"GET", "/api/desafios/meus"}, {"POST", "/api/desafios"},
		{"GET", "/api/amigos"}, {"GET", "/api/treinos"}, {"GET", "/api/usuarios/me"},
		{"POST", "/api/pagamento/pix"}, {"POST", "/api/desafios/1/entrar"},
	} {
		if r := api(t, rota.m, rota.p, "", nil); r.Status != 401 {
			t.Errorf("%s %s sem token: esperava 401, obteve %d", rota.m, rota.p, r.Status)
		}
		if r := api(t, rota.m, rota.p, "token.invalido.aqui", nil); r.Status != 401 {
			t.Errorf("%s %s com token inválido: esperava 401, obteve %d", rota.m, rota.p, r.Status)
		}
	}
}

func TestAuth_RateLimitNoLogin(t *testing.T) {
	cfg := *cfgBase
	cfg.RateLimitMax = 3
	srv := novoServidor(&cfg)
	var ultimo int
	for i := 0; i < 5; i++ {
		ultimo = chamar(t, srv, "POST", "/api/login", "", map[string]any{"email": "x@gymscore.test", "senha": "y"}).Status
	}
	if ultimo != 429 {
		t.Errorf("a 5ª tentativa com limite 3 deveria dar 429, obteve %d", ultimo)
	}
}

func TestAuth_CookieDeSessaoSecureEmProducao(t *testing.T) {
	u := novoUsuario(t)
	cfg := *cfgBase
	cfg.AppEnv = "production"
	srv := novoServidor(&cfg)
	b, _ := jsonBytes(map[string]any{"email": u.Email, "senha": u.Senha})
	res := doReq(t, srv, "POST", "/api/login", b)
	set := res.Header.Get("Set-Cookie")
	if l := strings.ToLower(set); !strings.Contains(l, "secure") || !strings.Contains(l, "httponly") {
		t.Errorf("cookie deveria ser Secure+HttpOnly em produção: %q", set)
	}
	// em desenvolvimento não é Secure
	res = doReq(t, servidor, "POST", "/api/login", b)
	if strings.Contains(strings.ToLower(res.Header.Get("Set-Cookie")), "secure") {
		t.Error("cookie não deveria ser Secure em desenvolvimento")
	}
}

// ─── Recuperação de senha ────────────────────────────────────────────────────

var reTokenEmail = regexp.MustCompile(`token=([A-Za-z0-9_\-%]+)`)

func tokenDoEmail(t *testing.T, email string) string {
	t.Helper()
	// o envio é assíncrono: aguarda o fake receber
	var corpo string
	for i := 0; i < 100; i++ {
		if c, ok := mailer.ultimoPara(email); ok {
			corpo = c
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if corpo == "" {
		t.Fatalf("e-mail de reset não enviado para %s", email)
	}
	m := reTokenEmail.FindStringSubmatch(corpo)
	if m == nil {
		t.Fatalf("link com token não encontrado no e-mail: %s", corpo)
	}
	tok, _ := url.QueryUnescape(m[1])
	return tok
}

func TestResetSenha_FluxoCompletoEUsoUnico(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	token := tokenDoEmail(t, u.Email)

	// o banco guarda apenas o hash, nunca o token
	var n int64
	db.Model(&models.PasswordResetToken{}).Where("id_usuario = ? AND token_hash = ?", u.ID, token).Count(&n)
	if n != 0 {
		t.Fatal("token em texto puro encontrado no banco")
	}
	db.Model(&models.PasswordResetToken{}).Where("id_usuario = ? AND CHAR_LENGTH(token_hash) = 64", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("deveria existir 1 hash SHA-256 de 64 chars, existem %d", n)
	}

	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": token, "nova_senha": "curta"}), 400)
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": token, "nova_senha": "senha-nova-456"}), 200)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "senha-nova-456"}), 200)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 401)
	// uso único
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": token, "nova_senha": "outra-senha-789"}), 400)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "senha-nova-456"}), 200)
}

func TestResetSenha_TokenExpiradoEInvalido(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	token := tokenDoEmail(t, u.Email)
	// horários sempre vindos do Go (o relógio/fuso do MySQL de teste difere do local)
	db.Exec("UPDATE password_reset_tokens SET expira_em = ? WHERE id_usuario = ?", time.Now().Add(-time.Minute), u.ID)

	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": token, "nova_senha": "senha-nova-456"}), 400)
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": strings.Repeat("a", 43), "nova_senha": "senha-nova-456"}), 400)
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": "x", "nova_senha": "senha-nova-456"}), 400)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200) // senha antiga segue valendo
}

func TestResetSenha_ValidadeDe30Minutos(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	_ = tokenDoEmail(t, u.Email)
	var tk models.PasswordResetToken
	db.Where("id_usuario = ?", u.ID).First(&tk)
	if d := time.Until(tk.ExpiraEm); d < 29*time.Minute || d > 30*time.Minute+time.Second {
		t.Errorf("validade deveria ser ~30 min, obteve %v", d)
	}
}

func TestResetSenha_NovoPedidoInvalidaOUltimoUsoApagaOsOutros(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	t1 := tokenDoEmail(t, u.Email)
	mailer.mu.Lock()
	mailer.envios = nil
	mailer.mu.Unlock()
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	t2 := tokenDoEmail(t, u.Email)
	if t1 == t2 {
		t.Fatal("tokens devem ser distintos")
	}
	// usar o t2 invalida o t1 (outros pendentes do usuário são consumidos)
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": t2, "nova_senha": "senha-nova-456"}), 200)
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": t1, "nova_senha": "senha-hack-456"}), 400)
}

func TestResetSenha_RespostaIgualParaEmailInexistente(t *testing.T) {
	existente := novoUsuario(t)
	a := esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": existente.Email}), 200)
	b := esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": "ninguem@gymscore.test"}), 200)
	if a.Raw != b.Raw {
		t.Errorf("respostas diferentes revelam existência da conta:\n%s\n%s", a.Raw, b.Raw)
	}
	time.Sleep(100 * time.Millisecond)
	if _, ok := mailer.ultimoPara("ninguem@gymscore.test"); ok {
		t.Error("não deve enviar e-mail para conta inexistente")
	}
	// corpo vazio/ruim também responde 200 genérico
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{}), 200)
	// fluxo antigo e-mail+CPF foi removido
	if r := api(t, "POST", "/api/usuarios/recuperar-senha", "", map[string]any{"email": existente.Email, "cpf": "1", "nova_senha": "12345678"}); r.Status == 200 {
		t.Error("fluxo antigo de recuperação por e-mail+CPF ainda existe")
	}
}

func TestMigracoes_ReaplicarEhIdempotente(t *testing.T) {
	sqlDB, _ := db.DB()
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		t.Fatalf("reaplicar migrations deveria ser no-op: %v", err)
	}
}
