//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/database"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─── #1 Revogação de JWT (claim tv) ──────────────────────────────────────────

func tokenNovo(t *testing.T, u *user) string {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	return r.data()["token"].(string)
}

func TestToken_LogoutRevogaOTokenImediatamente(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 200)
	esperar(t, api(t, "POST", "/api/logout", u.Token, nil), 200)
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 401)
	esperar(t, api(t, "POST", "/api/carteira/saque", u.Token, map[string]any{"valor_centavos": 500, "chave_pix": "a@b.com", "tipo_chave": "EMAIL"}), 401)

	novo := tokenNovo(t, u) // novo login volta a funcionar
	esperar(t, api(t, "GET", "/api/carteira", novo, nil), 200)

	// o token antigo (já revogado) fazendo logout NÃO derruba a sessão nova
	esperar(t, api(t, "POST", "/api/logout", u.Token, nil), 200)
	esperar(t, api(t, "GET", "/api/carteira", novo, nil), 200)

	// logout sem token / com lixo é inofensivo
	esperar(t, api(t, "POST", "/api/logout", "", nil), 200)
	esperar(t, api(t, "POST", "/api/logout", "lixo.lixo.lixo", nil), 200)
}

func TestToken_LogoutPeloCookieRevoga(t *testing.T) {
	u := novoUsuario(t)
	req := httptest.NewRequest("POST", "/api/logout", nil)
	req.AddCookie(httpCookie{Name: "jwt", Value: u.Token}.c())
	res, _ := servidor.Test(req, -1)
	if res.StatusCode != 200 {
		t.Fatalf("logout: %d", res.StatusCode)
	}
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 401)
}

func TestToken_LogoutsConcorrentesIncrementamUmaVez(t *testing.T) {
	u := novoUsuario(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); api(t, "POST", "/api/logout", u.Token, nil) }()
	}
	wg.Wait()
	var v int
	db.Raw("SELECT token_versao FROM usuarios WHERE id = ?", u.ID).Scan(&v)
	if v != 1 {
		t.Fatalf("versão deveria ter subido exatamente 1 vez, está em %d", v)
	}
	esperar(t, api(t, "GET", "/api/carteira", tokenNovo(t, u), nil), 200)
}

func TestToken_TrocaDeSenhaRevogaAntigosEEmiteNovo(t *testing.T) {
	u := usuarioComSaldo(t, 1000)
	r := esperar(t, api(t, "PATCH", "/api/usuarios/senha", u.Token, map[string]any{"senha_atual": u.Senha, "nova_senha": "outra-senha-999"}), 200)
	novo, _ := r.data()["token"].(string)
	if novo == "" {
		t.Fatalf("a resposta deve trazer o token novo: %s", r.Raw)
	}
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 401) // token antigo morreu
	esperar(t, api(t, "GET", "/api/carteira", novo, nil), 200)    // o novo mantém a sessão
	if saldo2 := esperar(t, api(t, "GET", "/api/carteira", novo, nil), 200).data()["saldo_centavos"].(float64); saldo2 != 1000 {
		t.Errorf("saldo não pode mudar: %v", saldo2)
	}
}

func TestToken_ResetDeSenhaRevogaTokensEmitidos(t *testing.T) {
	u := novoUsuario(t) // u.Token foi emitido antes do reset (ex.: token roubado)
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	tok := tokenDoEmail(t, u.Email)
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 200)
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": tok, "nova_senha": "senha-pos-reset-1"}), 200)
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 401)
	u.Senha = "senha-pos-reset-1"
	esperar(t, api(t, "GET", "/api/carteira", tokenNovo(t, u), nil), 200)
}

func TestToken_UsuarioApagadoNaoAutentica(t *testing.T) {
	u := novoUsuario(t)
	db.Exec("DELETE FROM usuarios WHERE id = ?", u.ID)
	esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 401)
}

// ─── #6 Reset: tokens anteriores invalidados; rate limit no importar ─────────

func TestResetSenha_NovoTokenInvalidaOsAnteriores(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	t1 := tokenDoEmail(t, u.Email)
	mailer.mu.Lock()
	mailer.envios = nil
	mailer.mu.Unlock()
	esperar(t, api(t, "POST", "/api/auth/esqueci-senha", "", map[string]any{"email": u.Email}), 200)
	t2 := tokenDoEmail(t, u.Email)

	// o 1º token não pode mais ser usado, mesmo com o 2º ainda sem uso
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": t1, "nova_senha": "senha-hack-456"}), 400)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	var pendentes int64
	db.Model(&models.PasswordResetToken{}).Where("id_usuario = ? AND usado_em IS NULL", u.ID).Count(&pendentes)
	if pendentes != 1 {
		t.Errorf("deveria restar exatamente 1 token válido, restam %d", pendentes)
	}
	esperar(t, api(t, "POST", "/api/auth/redefinir-senha", "", map[string]any{"token": t2, "nova_senha": "senha-nova-456"}), 200)
}

func TestTreino_ImportarTemRateLimit(t *testing.T) {
	cfg := *cfgBase
	cfg.RateLimitMax = 3
	srv := novoServidor(&cfg)
	u := novoUsuario(t)
	var ultimo int
	for i := 0; i < 5; i++ {
		ultimo = chamar(t, srv, "POST", "/api/treinos/importar", u.Token, map[string]any{"codigo": fmt.Sprintf("ZZZZ%02d", i)}).Status
	}
	if ultimo != 429 {
		t.Errorf("a 5ª tentativa de importar com limite 3 deveria dar 429, obteve %d", ultimo)
	}
}

// ─── #3 Ranking: dias inválido => 400 ───────────────────────────────────────

func TestRanking_DiasInvalidoDa400(t *testing.T) {
	u := novoUsuario(t)
	for _, q := range []string{"dias=0", "dias=abc", "dias=", "dias=-5", "dias=1000", "dias=366", "dias=1.5", "dias=%201"} {
		if r := api(t, "GET", "/api/ranking/amigos?"+q, u.Token, nil); r.Status != 400 {
			t.Errorf("?%s deveria dar 400, obteve %d", q, r.Status)
		}
	}
	for _, q := range []string{"", "?dias=1", "?dias=30", "?dias=365"} {
		esperar(t, api(t, "GET", "/api/ranking/amigos"+q, u.Token, nil), 200)
	}
}

// ─── #5 Swagger só fora de produção ─────────────────────────────────────────

func TestSwagger_SoForaDeProducao(t *testing.T) {
	if r := api(t, "GET", "/swagger/doc.json", "", nil); r.Status != 200 {
		t.Errorf("em desenvolvimento o swagger deve existir, status %d", r.Status)
	}
	cfg := *cfgBase
	cfg.AppEnv = "production"
	prod := novoServidor(&cfg)
	for _, p := range []string{"/swagger/doc.json", "/swagger/index.html", "/swagger/"} {
		if r := chamar(t, prod, "GET", p, "", nil); r.Status != 404 {
			t.Errorf("em produção %s deveria dar 404, obteve %d", p, r.Status)
		}
	}
}

// ─── #2/#4 Invariante do ledger com usuário migrado do schema antigo ─────────

func TestMigracao_UsuariosLegadosTemLastroNoLedger(t *testing.T) {
	const nome = "gymscore_it_mig"
	admin, err := gorm.Open(mysql.Open(dsnAdmin), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	admin.Exec("DROP DATABASE IF EXISTS " + nome)
	if err := admin.Exec("CREATE DATABASE " + nome + " CHARACTER SET utf8mb4").Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP DATABASE IF EXISTS " + nome)
	dbm, err := gorm.Open(mysql.Open(config.AplicarFusoDSN(strings.Replace(dsnAdmin, "/?", "/"+nome+"?", 1), time.Local)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := dbm.DB()
	ctx := context.Background()

	// 1) schema antigo (baseline = o que o AutoMigrate gerava) + dados legados
	if err := database.MigrarAte(ctx, sqlDB, 1); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(senhaPadrao), bcrypt.MinCost)
	type legado struct {
		saldo     string   // pode ser NULL
		depositos []string // "valor:status"
	}
	casos := []legado{
		{"123.45", []string{"50.00:received"}},
		{"0.10", nil},
		{"-5.50", nil},
		{"19.99", []string{"1000.00:received", "5.00:pending"}},
		{"0.00", []string{"20.00:received"}},
		{"100.00", []string{"100.00:refunded"}},
		{"NULL", nil},
		{"99999999.99", nil},
	}
	var ids []uint
	for i, c := range casos {
		saldoSQL := c.saldo
		res := dbm.Exec(fmt.Sprintf(`INSERT INTO usuarios (nome,sobrenome,cpf,email,senha,data_nascimento,genero,saldo,criado_em,atualizado_em)
			VALUES ('Legado%d','Teste',?,?,?,'1990-01-01','M',%s,NOW(3),NOW(3))`, i, saldoSQL),
			cpfValido(), fmt.Sprintf("qa+legado%d_%d@gymscore.test", i, seq.Add(1)), string(hash))
		if res.Error != nil {
			t.Fatal(res.Error)
		}
		var id uint
		dbm.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
		ids = append(ids, id)
		for j, d := range c.depositos {
			p := strings.SplitN(d, ":", 2)
			if err := dbm.Exec(`INSERT INTO transacoes (id_usuario,asaas_payment_id,valor,status,created_at,updated_at)
				VALUES (?,?,?,?,NOW(3),NOW(3))`, id, fmt.Sprintf("pay_leg_%d_%d_%d", i, j, seq.Add(1)), p[0], p[1]).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	// 2) demais migrations (conversão)
	if err := database.Migrar(ctx, sqlDB); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// 3) saldo == regra do ledger para TODOS, e o ajuste tem o formato esperado
	carteira := repositories.NovoCarteiraRepository(dbm)
	esperado := []int64{12345, 10, 0, 1999, 0, 10000, 0, 9999999999}
	for i, id := range ids {
		s, _ := carteira.Saldo(id)
		soma, _ := carteira.SomaLedger(id)
		if s != esperado[i] {
			t.Errorf("legado %d: saldo_centavos=%d esperado %d", i, s, esperado[i])
		}
		if s != soma {
			t.Errorf("legado %d: saldo %d != soma do ledger %d", i, s, soma)
		}
		var ajustes []models.Transacao
		dbm.Where("id_usuario = ? AND tipo = ?", id, models.TipoAjuste).Find(&ajustes)
		for _, a := range ajustes {
			if a.Status != models.TransacaoReceived || a.AsaasPaymentID != nil || a.Descricao == nil || !strings.HasPrefix(*a.Descricao, "Saldo migrado") {
				t.Errorf("legado %d: ajuste fora do formato: %+v", i, a)
			}
		}
	}
	// saldo legado negativo não some em silêncio: fica registrado no ajuste
	var desc string
	dbm.Raw("SELECT COALESCE(descricao,'') FROM transacoes WHERE id_usuario = ? AND tipo = 'ajuste'", ids[2]).Scan(&desc)
	if !strings.Contains(desc, "-5.50") {
		t.Errorf("a descrição do ajuste do saldo negativo deveria citar -5.50: %q", desc)
	}

	// 4) o app funciona sobre o banco migrado e a invariante continua valendo após operar
	srv := novoServidorEm(dbm)
	login := chamar(t, srv, "POST", "/api/login", "", map[string]any{"email": emailDe(dbm, ids[0]), "senha": senhaPadrao})
	if login.Status != 200 {
		t.Fatalf("login do usuário migrado: %d %s", login.Status, login.Raw)
	}
	tok := login.data()["token"].(string)
	esperarEm(t, chamar(t, srv, "POST", "/api/desafios", tok, map[string]any{"titulo": "Pós-migração", "valor_entrada_centavos": 2000}), 201)
	esperarEm(t, chamar(t, srv, "GET", "/api/carteira", tok, nil), 200)
	s, _ := carteira.Saldo(ids[0])
	soma, _ := carteira.SomaLedger(ids[0])
	if s != 10345 || s != soma {
		t.Errorf("após operar: saldo=%d soma=%d (esperado 10345 nos dois)", s, soma)
	}
	// o `created_at` do ajuste (NOW(3) rodado pela migration) deve estar na hora local, sem deslocamento
	var ajuste models.Transacao
	dbm.Where("id_usuario = ? AND tipo = ?", ids[0], models.TipoAjuste).First(&ajuste)
	if d := time.Since(ajuste.CreatedAt); d < -time.Minute || d > 5*time.Minute {
		t.Errorf("created_at do ajuste deslocado: %v (agora %v)", ajuste.CreatedAt, time.Now())
	}
	// o extrato do usuário migrado mostra o ajuste
	ext := chamar(t, srv, "GET", "/api/carteira/extrato", tok, nil)
	if !strings.Contains(ext.Raw, `"ajuste"`) {
		t.Errorf("extrato deveria listar a linha de ajuste: %s", ext.Raw)
	}
}

func emailDe(d *gorm.DB, id uint) string {
	var e string
	d.Raw("SELECT email FROM usuarios WHERE id = ?", id).Scan(&e)
	return e
}

func esperarEm(t *testing.T, r resp, status int) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("esperava HTTP %d, obteve %d: %s", status, r.Status, r.Raw)
	}
}
