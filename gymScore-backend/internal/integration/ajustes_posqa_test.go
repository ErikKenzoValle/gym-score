//go:build integration

package integration

// Rodada de ajustes pós-QA (R1, R2, R4, R7): headers do painel por qualquer forma de caminho, trava
// das migrations entre instâncias, limite do login admin independente de RATE_LIMIT_MAX e cookies de
// sessão endurecidos (token fora do corpo, __Host-/__Secure-, Secure com https, logout/troca apagando).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/database"
	"gymScore-backend/internal/app"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// cookieApagado confere que a resposta apaga o cookie `nome` com Path, HttpOnly e Expires no passado.
func cookieApagado(t *testing.T, r resp, nome, path string) *http.Cookie {
	t.Helper()
	for _, c := range r.Cookies {
		if c.Name == nome {
			if c.Value != "" || c.Path != path || !c.HttpOnly || !c.Expires.Before(time.Now()) {
				t.Errorf("cookie %s não foi apagado corretamente: %s", nome, c.String())
			}
			return c
		}
	}
	t.Errorf("resposta não apaga o cookie %s: %v", nome, r.Cookies)
	return nil
}

// ─── R1: headers do admin por caminho decodificado/normalizado ──────────────

func TestR1_HeadersDoAdminEmQualquerFormaDeCaminho(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "admin", "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	for arq, conteudo := range map[string]string{
		"admin/index.html":  "<!doctype html><title>GymScore Admin</title>",
		"admin/login.html":  "<!doctype html><title>GymScore Admin Login</title>",
		"admin/js/admin.js": "console.log(1)",
		"index.html":        "<!doctype html><title>App</title>",
	} {
		if err := os.WriteFile(filepath.Join(dir, arq), []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := *cfgBase
	cfg.FrontendPath = dir
	srv := app.Construir(&cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})

	for _, caminho := range []string{
		"/%61dmin/index.html", "/%61dmin/login.html", "/%61dmin/", "/%61DMIN/index.html", "/ADMIN/", "/Admin",
		"/admin/../admin/index.html", "/x/../admin/index.html", "//admin/index.html", "/admin/./js/admin.js",
		"/%61dmin/js/admin.js", "/admin/index.html", "/admin/login",
	} {
		res, err := srv.Test(httptest.NewRequest("GET", caminho, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		corpo, _ := io.ReadAll(res.Body)
		res.Body.Close()
		csp := res.Header.Get("Content-Security-Policy")
		if csp != middlewares.CSPAdmin || res.Header.Get("X-Frame-Options") != "DENY" ||
			res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: %d sem os headers do admin: %v", caminho, res.StatusCode, res.Header)
		}
		t.Logf("%s -> %d %q", caminho, res.StatusCode, string(corpo[:min(len(corpo), 40)]))
	}
	// a página do admin servida pelo caminho codificado (o bug do QA) vem com os headers
	res, _ := srv.Test(httptest.NewRequest("GET", "/%61dmin/index.html", nil), -1)
	corpo, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(corpo), "GymScore Admin") || res.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("/%%61dmin/index.html: %d %s %v", res.StatusCode, corpo, res.Header)
	}
	// o app comum não recebe a CSP/X-Frame do admin
	res, _ = srv.Test(httptest.NewRequest("GET", "/index.html", nil), -1)
	if res.Header.Get("Content-Security-Policy") != "" || res.Header.Get("X-Frame-Options") != "" {
		t.Errorf("/index.html recebeu headers do admin: %v", res.Header)
	}
}

// ─── R2: trava das migrations entre instâncias ──────────────────────────────

func TestR2_MigracoesConcorrentesComTrava(t *testing.T) {
	nome := nomeBanco + "_trava"
	admin, err := gorm.Open(mysql.Open(dsnAdmin), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	admin.Exec("DROP DATABASE IF EXISTS " + nome)
	if err := admin.Exec("CREATE DATABASE " + nome + " CHARACTER SET utf8mb4").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP DATABASE IF EXISTS " + nome) })
	dsn := config.AplicarFusoDSN(strings.Replace(dsnAdmin, "/?", "/"+nome+"?", 1), time.Local)

	// 3 "réplicas", cada uma com o seu pool, sobem juntas sobre o banco vazio (e de novo sobre o banco
	// já migrado): todas terminam sem erro e cada versão é aplicada uma única vez.
	for rodada := 0; rodada < 2; rodada++ {
		const n = 3
		errs := make([]error, n)
		var wg sync.WaitGroup
		inicio := make(chan struct{})
		for i := 0; i < n; i++ {
			pool, err := sql.Open("mysql", dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { pool.Close() })
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-inicio
				errs[i] = database.Migrar(context.Background(), pool)
			}(i)
		}
		close(inicio)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Errorf("rodada %d, instância %d: %v", rodada, i, err)
			}
		}
	}
	d, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Discard})
	var dup int64
	d.Raw("SELECT COUNT(*) FROM (SELECT version_id FROM goose_db_version WHERE version_id > 0 GROUP BY version_id HAVING COUNT(*) > 1) x").Scan(&dup)
	var maxV, maxBase int64
	d.Raw("SELECT MAX(version_id) FROM goose_db_version").Scan(&maxV)
	db.Raw("SELECT MAX(version_id) FROM goose_db_version").Scan(&maxBase)
	if dup != 0 || maxV != maxBase || maxV == 0 {
		t.Errorf("goose_db_version: %d versões duplicadas, máx %d (esperado %d)", dup, maxV, maxBase)
	}
	var travada int64
	d.Raw("SELECT COUNT(*) FROM goose_lock WHERE locked = 1").Scan(&travada)
	if travada != 0 {
		t.Errorf("a trava ficou presa depois das migrações")
	}
}

// ─── R4: limite do login admin independente de RATE_LIMIT_MAX ───────────────

func TestR4_LoginAdminLimitadoMesmoComRateLimitZero(t *testing.T) {
	cfg := *cfgBase
	cfg.RateLimitMax = 0           // limite geral desligado
	cfg.AdminLoginRateLimitMax = 0 // "desligar" o do admin não desliga: vira o padrão (5/min)
	srv := app.Construir(&cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
	a := novoAdminDB(t, "admin")
	codigos := []int{}
	for i := 0; i < config.AdminLoginRateLimitPadrao+2; i++ {
		r, _ := chamarAdmin(t, srv, "POST", "/api/admin/auth/login", "", map[string]any{"email": a.Email, "senha": "errada"})
		codigos = append(codigos, r.Status)
	}
	for i, c := range codigos {
		esperado := 401
		if i >= config.AdminLoginRateLimitPadrao {
			esperado = 429
		}
		if c != esperado {
			t.Errorf("tentativa %d: %d (esperado %d) — %v", i+1, c, esperado, codigos)
		}
	}
	// o login de usuário segue sem limite com RATE_LIMIT_MAX=0
	for i := 0; i < 8; i++ {
		esperar(t, chamar(t, srv, "POST", "/api/login", "", map[string]any{"email": "ninguem@gymscore.test", "senha": "x"}), 401)
	}
}

// ─── R7: cookies de sessão ──────────────────────────────────────────────────

// pedir faz uma requisição como o navegador: cookies + Origin.
func pedir(t *testing.T, srv *fiber.App, metodo, caminho, origem string, cookies []*http.Cookie, corpo any) resp {
	t.Helper()
	var rd io.Reader
	if corpo != nil {
		b, _ := json.Marshal(corpo)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(metodo, caminho, rd)
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origem != "" {
		req.Header.Set("Origin", origem)
	}
	for _, c := range cookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw), Cookies: res.Cookies()}
	_ = json.Unmarshal(raw, &out.Body)
	rotaDeSessao := false
	for _, p := range []string{"/api/login", "/api/logout", "/api/usuarios/senha", "/api/usuarios/me", "/api/admin/"} {
		rotaDeSessao = rotaDeSessao || strings.HasPrefix(caminho, p)
	}
	if res.StatusCode == 200 && rotaDeSessao {
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s %s sem Cache-Control: no-store: %q", metodo, caminho, res.Header.Get("Cache-Control"))
		}
	}
	return out
}

func cookieChamado(r resp, nome string) *http.Cookie {
	for _, c := range r.Cookies {
		if c.Name == nome {
			return c
		}
	}
	return nil
}

func TestR7_CookieDeSessaoUsuarioDev(t *testing.T) {
	u := novoUsuario(t) // tokenDaSessao já conferiu: corpo sem token, cookie jwt HttpOnly/Lax/Path=/
	login := pedir(t, servidor, "POST", "/api/login", "", nil, map[string]any{"email": u.Email, "senha": u.Senha})
	a := cookieChamado(login, "jwt")
	if a == nil || strings.Contains(login.Raw, "token") || strings.Contains(login.Raw, a.Value) {
		t.Fatalf("login: %v %s", login.Cookies, login.Raw)
	}
	if _, ok := login.data()["usuario"]; !ok {
		t.Errorf("login deveria trazer o resumo do usuário: %s", login.Raw)
	}
	exp := a.Expires
	if d := time.Until(exp); d < 23*time.Hour || d > 24*time.Hour+time.Minute {
		t.Errorf("Expires do cookie (%v) deveria ser o exp do JWT (24h)", exp)
	}
	esperar(t, pedir(t, servidor, "GET", "/api/usuarios/me", "", []*http.Cookie{a}, nil), 200)

	// troca de senha pelo cookie: emite cookie novo; o antigo deixa de valer
	troca := esperar(t, pedir(t, servidor, "PATCH", "/api/usuarios/senha", origemTeste, []*http.Cookie{a},
		map[string]any{"senha_atual": u.Senha, "nova_senha": "senha-nova-r7-123"}), 200)
	b := cookieChamado(troca, "jwt")
	if b == nil || b.Value == "" || b.Value == a.Value || strings.Contains(troca.Raw, b.Value) || strings.Contains(troca.Raw, "token") {
		t.Fatalf("troca de senha deveria gravar cookie novo sem token no corpo: %v %s", troca.Cookies, troca.Raw)
	}
	esperar(t, pedir(t, servidor, "GET", "/api/usuarios/me", "", []*http.Cookie{a}, nil), 401)
	esperar(t, pedir(t, servidor, "GET", "/api/usuarios/me", "", []*http.Cookie{b}, nil), 200)

	// logout só por POST (GET não existe) e apaga o cookie com os mesmos atributos
	if r := pedir(t, servidor, "GET", "/api/logout", "", []*http.Cookie{b}, nil); r.Status == 200 {
		t.Errorf("GET /api/logout não pode encerrar a sessão")
	}
	esperar(t, pedir(t, servidor, "GET", "/api/usuarios/me", "", []*http.Cookie{b}, nil), 200)
	sair := esperar(t, pedir(t, servidor, "POST", "/api/logout", origemTeste, []*http.Cookie{b}, nil), 200)
	if c := cookieApagado(t, sair, "jwt", "/"); c != nil && (c.Secure || c.SameSite != http.SameSiteLaxMode) {
		t.Errorf("logout apagou com atributos diferentes: %s", c.String())
	}
	esperar(t, pedir(t, servidor, "GET", "/api/usuarios/me", "", []*http.Cookie{b}, nil), 401)
}

// Com APP_BASE_URL https (sem ser produção): __Host-jwt / __Secure-admin_jwt, Secure.
func TestR7_CookiesComHTTPS(t *testing.T) {
	const origem = "https://app.gymscore.test"
	cfg := *cfgBase
	cfg.AppBaseURL = origem
	srv := app.Construir(&cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})

	u := novoUsuario(t)
	login := esperar(t, pedir(t, srv, "POST", "/api/login", "", nil, map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	c := cookieChamado(login, "__Host-jwt")
	if c == nil || cookieChamado(login, "jwt") != nil {
		t.Fatalf("esperava só __Host-jwt: %v", login.Cookies)
	}
	if !c.Secure || !c.HttpOnly || c.Path != "/" || c.Domain != "" || c.SameSite != http.SameSiteLaxMode || strings.Contains(login.Raw, c.Value) {
		t.Errorf("atributos de __Host-jwt: %s", c.String())
	}
	esperar(t, pedir(t, srv, "GET", "/api/usuarios/me", "", []*http.Cookie{c}, nil), 200)
	// o nome de dev não vale com cookies Secure
	esperar(t, pedir(t, srv, "GET", "/api/usuarios/me", "", []*http.Cookie{{Name: "jwt", Value: c.Value}}, nil), 401)
	// CSRF continua valendo para o cookie novo
	esperar(t, pedir(t, srv, "POST", "/api/logout", "https://evil.example", []*http.Cookie{c}, nil), 403)
	sair := esperar(t, pedir(t, srv, "POST", "/api/logout", origem, []*http.Cookie{c}, nil), 200)
	if a := cookieApagado(t, sair, "__Host-jwt", "/"); a != nil && (!a.Secure || a.SameSite != http.SameSiteLaxMode) {
		t.Errorf("logout apagou com atributos diferentes: %s", a.String())
	}
	esperar(t, pedir(t, srv, "GET", "/api/usuarios/me", "", []*http.Cookie{c}, nil), 401)

	// admin
	ad := novoAdminDB(t, "admin")
	la := esperar(t, pedir(t, srv, "POST", "/api/admin/auth/login", origem, nil, map[string]any{"email": ad.Email, "senha": ad.Senha}), 200)
	ca := cookieChamado(la, "__Secure-admin_jwt")
	if ca == nil || cookieChamado(la, "admin_jwt") != nil || strings.Contains(la.Raw, ca.Value) {
		t.Fatalf("esperava só __Secure-admin_jwt fora do corpo: %v %s", la.Cookies, la.Raw)
	}
	if !ca.Secure || !ca.HttpOnly || ca.Path != "/api/admin" || ca.SameSite != http.SameSiteStrictMode {
		t.Errorf("atributos de __Secure-admin_jwt: %s", ca.String())
	}
	if d := time.Until(ca.Expires); d <= 0 || d > cfg.TTLAdmin()+time.Minute {
		t.Errorf("Expires do cookie admin (%v) deveria ser o exp do token", ca.Expires)
	}
	esperar(t, pedir(t, srv, "GET", "/api/admin/auth/me", "", []*http.Cookie{ca}, nil), 200)
	esperar(t, pedir(t, srv, "GET", "/api/admin/auth/me", "", []*http.Cookie{{Name: "admin_jwt", Value: ca.Value}}, nil), 401)
	saiA := esperar(t, pedir(t, srv, "POST", "/api/admin/auth/logout", origem, []*http.Cookie{ca}, nil), 200)
	if a := cookieApagado(t, saiA, "__Secure-admin_jwt", "/api/admin"); a != nil && (!a.Secure || a.SameSite != http.SameSiteStrictMode) {
		t.Errorf("logout admin apagou com atributos diferentes: %s", a.String())
	}
	esperar(t, pedir(t, srv, "GET", "/api/admin/auth/me", "", []*http.Cookie{ca}, nil), 401)
}

// Troca de senha do admin: cookie novo, antigo recusado; logout em dev apaga admin_jwt.
func TestR7_CookieAdminTrocaDeSenhaELogoutDev(t *testing.T) {
	ad := novoAdminDB(t, "admin")
	antigo := &http.Cookie{Name: "admin_jwt", Value: ad.Cookie}
	r := esperar(t, pedir(t, servidor, "POST", "/api/admin/auth/senha", origemTeste, []*http.Cookie{antigo},
		map[string]any{"senha_atual": ad.Senha, "nova_senha": "senha-admin-nova-r7-456"}), 200)
	novo := cookieChamado(r, "admin_jwt")
	if novo == nil || novo.Value == "" || novo.Value == ad.Cookie || strings.Contains(r.Raw, novo.Value) {
		t.Fatalf("troca de senha admin: %v %s", r.Cookies, r.Raw)
	}
	esperar(t, pedir(t, servidor, "GET", "/api/admin/auth/me", "", []*http.Cookie{antigo}, nil), 401)
	esperar(t, pedir(t, servidor, "GET", "/api/admin/auth/me", "", []*http.Cookie{novo}, nil), 200)
	sair := esperar(t, pedir(t, servidor, "POST", "/api/admin/auth/logout", origemTeste, []*http.Cookie{novo}, nil), 200)
	if c := cookieApagado(t, sair, "admin_jwt", "/api/admin"); c != nil && (c.Secure || c.SameSite != http.SameSiteStrictMode) {
		t.Errorf("logout admin apagou com atributos diferentes: %s", c.String())
	}
	esperar(t, pedir(t, servidor, "GET", "/api/admin/auth/me", "", []*http.Cookie{novo}, nil), 401)
}
