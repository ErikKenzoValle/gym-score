//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/pkg/utils"
)

// Testes do QA (rodada 4, Q4-1): verificação independente de modo atleta/instrutor, cookie, CSRF,
// conflito de interesse, pode_entrar x 409 e endereço/CEP. Não substituem os testes dos devs.

// tokenModoForjado emite um JWT VÁLIDO (assinado com o segredo) com o claim `modo` escolhido — simula um
// token antigo/forjado: o servidor tem de reconferir no banco.
func qaR4TokenModo(t *testing.T, u *user, modo string) string {
	t.Helper()
	c := claimsDe(t, u.Token)
	tok, err := utils.GerarTokenModo(u.ID, u.Email, c.Versao, modo, cfgBase.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestQAR4_ClaimModoForjadoReconferidoNoBanco(t *testing.T) {
	c := novoCenario(t)
	idVaga := desafioVagaAberta(t, c)
	semPedido := novoUsuario(t)
	pendente := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/instrutores", pendente.Token, map[string]any{"cref": "123456-G/SP"}), 201)
	rejeitado := novoUsuario(t)
	rid := idDe(esperar(t, api(t, "POST", "/api/instrutores", rejeitado.Token, map[string]any{"cref": "123456-G/SP"}), 201))
	db.Exec("UPDATE instrutores SET status = 'rejeitada' WHERE id = ?", rid)

	for nome, u := range map[string]*user{"sem pedido": semPedido, "pendente": pendente, "rejeitado": rejeitado} {
		tok := qaR4TokenModo(t, u, "instrutor")
		for _, rota := range []string{"/api/instrutor/agenda", "/api/instrutor/vagas"} {
			if r := api(t, "GET", rota, tok, nil); r.Status != 403 || codigoErro(r) != "modo_indisponivel" {
				t.Errorf("%s com claim instrutor forjado em %s: %d %s", nome, rota, r.Status, r.Raw)
			}
		}
		if r := api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", idVaga), tok, map[string]any{"lance_centavos": 0}); r.Status != 403 {
			t.Errorf("%s: candidatura com claim forjado: %d %s", nome, r.Status, r.Raw)
		}
		// /me não promove: modos_disponiveis continua [atleta]
		if m := me(t, &user{Token: tok}); fmt.Sprint(m["modos_disponiveis"]) != "[atleta]" {
			t.Errorf("%s: /me com claim forjado: %v", nome, m)
		}
		// claim de academia numa conta de atleta não abre rota de academia
		if r := api(t, "GET", "/api/academia/perfil", qaR4TokenModo(t, u, "academia"), nil); r.Status != 403 {
			t.Errorf("%s: claim academia forjado abriu rota de academia: %d %s", nome, r.Status, r.Raw)
		}
	}
	// Instrutor aprovado com token modo=instrutor; aprovado → rejeitado → removido: modo_indisponivel.
	u, iid := instrutorAprovado(t, 0)
	modoInstrutor(t, u)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 200)
	db.Exec("UPDATE instrutores SET status = 'pendente' WHERE id = ?", iid)
	esperarCodigo(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 403, "modo_indisponivel")
	// a troca para instrutor também é recusada; para atleta funciona
	esperarCodigo(t, api(t, "POST", "/api/sessao/modo", u.Token, map[string]any{"modo": "instrutor"}), 403, "modo_indisponivel")
	modoAtleta(t, u)
	// modo errado → modo_incorreto com o modo necessário
	db.Exec("UPDATE instrutores SET status = 'aprovada' WHERE id = ?", iid)
	r := api(t, "GET", "/api/instrutor/agenda", u.Token, nil)
	esperarCodigo(t, r, 403, "modo_incorreto")
	if d, _ := r.Body["data"].(map[string]any); d == nil || d["modo_necessario"] != "instrutor" {
		t.Errorf("modo_incorreto sem modo_necessario: %s", r.Raw)
	}
}

func TestQAR4_CookiePreferenciaNaoEscala(t *testing.T) {
	b, _ := instrutorAprovado(t, 0) // dono "legítimo" de um cookie <id>-instrutor
	a := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/instrutores", a.Token, map[string]any{"cref": "123456-G/SP"}), 201) // pendente
	acad, _ := novaAcademia(t)
	casos := []struct {
		u     *user
		valor string
		modo  string
	}{
		{a, fmt.Sprintf("%d-instrutor", b.ID), "atleta"},
		{a, fmt.Sprintf("%d-instrutor", a.ID), "atleta"}, // pendente: não pode
		{a, fmt.Sprintf("%d-academia", a.ID), "atleta"},
		{a, fmt.Sprintf("%d-instrutor-x", a.ID), "atleta"},
		{a, fmt.Sprintf("0%d-instrutor", a.ID), "atleta"},
		{a, "-instrutor", "atleta"},
		{a, strings.Repeat("9", 3000) + "-instrutor", "atleta"},
		{b, fmt.Sprintf("%d-instrutor", b.ID), "instrutor"},
		{b, fmt.Sprintf("%d-atleta", b.ID), "atleta"},
		{acad, fmt.Sprintf("%d-instrutor", acad.ID), ""},
	}
	for _, k := range casos {
		login := chamarCookies(t, servidor, "POST", "/api/login", "", map[string]any{"email": k.u.Email, "senha": k.u.Senha},
			&http.Cookie{Name: "modo", Value: k.valor})
		if login.Status != 200 {
			t.Errorf("login com cookie %q: %d %s", k.valor[:min(len(k.valor), 30)], login.Status, login.Raw)
			continue
		}
		if c := claimsDe(t, tokenDaSessao(t, login)); c.Modo != k.modo {
			t.Errorf("cookie %q para %d: modo %q, esperava %q", k.valor[:min(len(k.valor), 30)], k.u.ID, c.Modo, k.modo)
		}
	}
}

// atributos do Set-Cookie sem o valor e sem a data (para comparar login × troca de modo).
var reValorCookie = regexp.MustCompile(`^([^=]+)=[^;]*`)
var reExpira = regexp.MustCompile(`(?i);\s*expires=[^;]*`)

func qaR4AtributosCookie(t *testing.T, srvResp *http.Response, nome string) string {
	t.Helper()
	for _, h := range srvResp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(h, nome+"=") {
			s := reValorCookie.ReplaceAllString(h, "$1=X")
			return reExpira.ReplaceAllString(s, "")
		}
	}
	return ""
}

func TestQAR4_TrocaDeModoMesmosAtributosDoLoginECSRF(t *testing.T) {
	for _, base := range []string{cfgBase.AppBaseURL, "https://gymscore.test"} {
		cfg := *cfgBase
		cfg.AppBaseURL = base
		srv := novoServidor(&cfg)
		nome := "jwt"
		if strings.HasPrefix(base, "https") {
			nome = "__Host-jwt"
		}
		u, _ := instrutorAprovado(t, 0)
		fazer := func(metodo, rota, origem, bearer string, corpo string, cookies ...*http.Cookie) *http.Response {
			req, _ := http.NewRequest(metodo, base+rota, strings.NewReader(corpo))
			req.Header.Set("Content-Type", "application/json")
			if origem != "" {
				req.Header.Set("Origin", origem)
			}
			if bearer != "" {
				req.Header.Set("Authorization", "Bearer "+bearer)
			}
			for _, c := range cookies {
				req.AddCookie(c)
			}
			res, err := srv.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		login := fazer("POST", "/api/login", "", "", fmt.Sprintf(`{"email":%q,"senha":%q}`, u.Email, u.Senha))
		if login.StatusCode != 200 {
			t.Fatalf("%s login: %d", base, login.StatusCode)
		}
		var sess *http.Cookie
		for _, c := range login.Cookies() {
			if c.Name == nome {
				sess = c
			}
		}
		if sess == nil {
			t.Fatalf("%s: login sem cookie %s", base, nome)
		}
		attLogin := qaR4AtributosCookie(t, login, nome)
		// CSRF: cookie + origem alheia / sem Origin nem Referer → 403; Referer alheio → 403
		for _, o := range []string{"https://atacante.example", ""} {
			if r := fazer("POST", "/api/sessao/modo", o, "", `{"modo":"instrutor"}`, sess); r.StatusCode != 403 {
				t.Errorf("%s CSRF origem %q: %d", base, o, r.StatusCode)
			}
		}
		troca := fazer("POST", "/api/sessao/modo", base, "", `{"modo":"instrutor"}`, sess)
		if troca.StatusCode != 200 {
			t.Fatalf("%s troca: %d", base, troca.StatusCode)
		}
		if cc := troca.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s troca sem no-store: %q", base, cc)
		}
		attTroca := qaR4AtributosCookie(t, troca, nome)
		if attLogin == "" || attLogin != attTroca {
			t.Errorf("%s atributos diferentes:\n login: %s\n troca: %s", base, attLogin, attTroca)
		}
		var novo *http.Cookie
		for _, c := range troca.Cookies() {
			if c.Name == nome {
				novo = c
			}
		}
		if novo == nil || novo.Value == sess.Value || claimsDe(t, novo.Value).Modo != "instrutor" {
			t.Errorf("%s: cookie novo não reemitido com modo instrutor", base)
		}
		// Bearer sem Origin (cliente não-navegador) passa
		if r := fazer("POST", "/api/sessao/modo", "", novo.Value, `{"modo":"atleta"}`); r.StatusCode != 200 {
			t.Errorf("%s Bearer sem Origin: %d", base, r.StatusCode)
		}
	}
}

// pode_entrar/motivo_bloqueio_entrada tem de prever exatamente o resultado do POST /entrar.
func qaR4Coerente(t *testing.T, u *user, id uint, rotulo string) {
	t.Helper()
	d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200).data()
	motivo, _ := d["motivo_bloqueio_entrada"].(string)
	r := acao(t, u, id, "entrar", nil)
	switch {
	case d["pode_entrar"] == true:
		if r.Status != 200 || motivo != "" {
			t.Errorf("%s: pode_entrar=true mas entrar=%d %s (motivo %q)", rotulo, r.Status, r.Raw, motivo)
		}
	case motivo != "":
		if r.Status != 409 || r.Body["error"] != motivo {
			t.Errorf("%s: motivo %q mas entrar=%d %s", rotulo, motivo, r.Status, r.Raw)
		}
	default:
		if r.Status == 200 {
			t.Errorf("%s: pode_entrar=false sem motivo mas entrou", rotulo)
		}
	}
}

func TestQAR4_PodeEntrarCoerenteCom409(t *testing.T) {
	c := novoCenario(t)
	b := novoCenario(t)
	// designado + vinculado
	idA := criarDesafio(t, c, 0, 5)
	modoAtleta(t, c.I)
	qaR4Coerente(t, c.I, idA, "designado")
	v, _ := novoInstrutorDe(t, c.AcademiaID)
	modoAtleta(t, v)
	qaR4Coerente(t, v, criarDesafio(t, c, 0, 5), "vinculado")
	qaR4Coerente(t, v, criarDesafio(t, b, 0, 5), "outra academia")
	// candidatura pendente (autônomo aprovado)
	auto, _ := instrutorAprovado(t, 0)
	idV := desafioVagaAberta(t, b)
	modoInstrutor(t, auto)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", idV), auto.Token, map[string]any{"lance_centavos": 0}), 201)
	modoAtleta(t, auto)
	qaR4Coerente(t, auto, idV, "candidatura pendente")
	// pedido pendente vinculado: não bloqueia (só quando designado)
	p := novoUsuario(t)
	pid := idDe(esperar(t, api(t, "POST", "/api/instrutores", p.Token, map[string]any{"cref": "123456-G/SP"}), 201))
	db.Exec("UPDATE instrutores SET id_academia = ? WHERE id = ?", b.AcademiaID, pid)
	qaR4Coerente(t, p, criarDesafio(t, b, 0, 5), "pendente vinculado")
	// pedido pendente designado (estado legado): bloqueia
	idPD := criarDesafio(t, b, 0, 5)
	db.Exec("UPDATE desafios SET id_instrutor = ? WHERE id = ?", pid, idPD)
	qaR4Coerente(t, p, idPD, "pendente designado")
	// rejeitado vinculado: não bloqueia
	db.Exec("UPDATE instrutores SET status = 'rejeitada' WHERE id = ?", pid)
	qaR4Coerente(t, p, criarDesafio(t, b, 0, 5), "rejeitado vinculado")
	// atleta comum
	qaR4Coerente(t, novoUsuario(t), criarDesafio(t, b, 0, 5), "atleta comum")
	// modo instrutor: pode_entrar=false e entrar → 403 modo_incorreto
	modoInstrutor(t, auto)
	id2 := criarDesafio(t, b, 0, 5)
	d := detalhe(t, auto, id2)
	if d["pode_entrar"] != false || d["motivo_bloqueio_entrada"] != nil {
		t.Errorf("modo instrutor: %v %v", d["pode_entrar"], d["motivo_bloqueio_entrada"])
	}
	esperarCodigo(t, acao(t, auto, id2, "entrar", nil), 403, "modo_incorreto")
}

// Corrida de TRÊS ações da mesma conta no mesmo instante: entrar (token atleta) no desafio de vaga aberta
// da academia, candidatar-se a ele (token instrutor) e aceitar o convite de vínculo da mesma academia.
// Invariante: nunca inscrito + (candidato ou vinculado); nenhum 5xx.
func TestQAR4_ConflitoTresViasConcorrente(t *testing.T) {
	for k := 0; k < 10; k++ {
		b := novoCenario(t)
		id := desafioVagaAberta(t, b)
		u, iid := instrutorAprovado(t, 0)
		atleta := &user{Token: u.Token}
		modoInstrutor(t, u)
		esperar(t, api(t, "POST", "/api/academia/instrutores", b.A.Token, map[string]any{"email": u.Email}), 201)
		var wg sync.WaitGroup
		st := make([]int, 3)
		inicio := make(chan struct{})
		for i, f := range []func() int{
			func() int {
				return api(t, "POST", fmt.Sprintf("/api/desafios/%d/entrar", id), atleta.Token, nil).Status
			},
			func() int {
				return api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), u.Token, map[string]any{"lance_centavos": 0}).Status
			},
			func() int { return api(t, "POST", "/api/instrutor/convite/aceitar", u.Token, nil).Status },
		} {
			wg.Add(1)
			go func(i int, f func() int) { defer wg.Done(); <-inicio; st[i] = f() }(i, f)
		}
		close(inicio)
		wg.Wait()
		for _, s := range st {
			if s >= 500 {
				t.Fatalf("rodada %d: 5xx %v", k, st)
			}
		}
		var inscrito, cand int64
		var vinc *uint
		db.Raw("SELECT COUNT(*) FROM desafio_participantes WHERE id_desafio = ? AND id_usuario = ?", id, u.ID).Scan(&inscrito)
		db.Raw("SELECT COUNT(*) FROM desafio_candidaturas_instrutor WHERE id_desafio = ? AND id_instrutor = ? AND status IN ('pendente','escolhida')", id, iid).Scan(&cand)
		db.Raw("SELECT id_academia FROM instrutores WHERE id = ?", iid).Scan(&vinc)
		if inscrito > 0 && (cand > 0 || vinc != nil) {
			t.Errorf("rodada %d: conflito gravado (inscrito=%d cand=%d vinc=%v) status=%v", k, inscrito, cand, vinc, st)
		}
		if (st[0] == 200) != (inscrito > 0) {
			t.Errorf("rodada %d: status entrar %d x banco %d", k, st[0], inscrito)
		}
	}
}

func TestQAR4_SuspensoNosDoisModosEPendenteNaoAfeta(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 0, 5)
	at := novoUsuario(t)
	inscrever(t, id, at)
	tokAtleta := qaR4TokenModo(t, c.I, "atleta")
	db.Exec("UPDATE usuarios SET suspenso_ate = ? WHERE id = ?", time.Now().Add(time.Hour), c.I.ID)
	esperarCodigo(t, acao(t, c.I, id, "iniciar", nil), 403, "conta_suspensa")
	esperarCodigo(t, acao(t, &user{Token: tokAtleta}, criarDesafio(t, novoCenario(t), 0, 5), "entrar", nil), 403, "conta_suspensa")
	// leitura continua liberada nos dois modos
	esperar(t, api(t, "GET", "/api/instrutor/agenda", c.I.Token, nil), 200)
	db.Exec("UPDATE usuarios SET suspenso_ate = NULL WHERE id = ?", c.I.ID)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)

	// pedido pendente: o atleta entra/sai/avalia normalmente e o perfil continua atleta
	p := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/instrutores", p.Token, map[string]any{"cref": "123456-G/SP"}), 201)
	id2 := criarDesafio(t, c, 0, 5)
	inscrever(t, id2, p)
	esperar(t, acao(t, p, id2, "sair", nil), 200)
	if m := me(t, p); m["perfil"] != "atleta" || m["modo_ativo"] != "atleta" {
		t.Errorf("pendente: %v", m)
	}
}

func TestQAR4_EnderecoValidacoesEAdmin(t *testing.T) {
	base := func(extra map[string]any) map[string]any {
		n := seq.Add(1)
		b := map[string]any{"nome": "Acad", "email": fmt.Sprintf("qa+r4end%d@gymscore.test", n), "senha": senhaPadrao,
			"razao_social": "Academia QA LTDA", "nome_fantasia": fmt.Sprintf("QA R4 %d", n), "cnpj": cnpjUnico(),
			"cep": "20010000", "logradouro": "Rua X", "numero": "S/N", "bairro": "Centro", "cidade": "Rio de Janeiro", "uf": "RJ"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	for nome, extra := range map[string]map[string]any{
		"CEP 9 dígitos": {"cep": "200100001"}, "CEP vazio": {"cep": ""}, "CEP só máscara": {"cep": "-----"},
		"UF minúscula inválida": {"uf": "xx"}, "UF 3 letras": {"uf": "RJA"}, "UF número": {"uf": "12"},
		"número vazio": {"numero": ""}, "número 21 chars": {"numero": strings.Repeat("1", 21)},
		"logradouro 151": {"logradouro": strings.Repeat("a", 151)}, "bairro vazio": {"bairro": " "},
		"complemento 101": {"complemento": strings.Repeat("c", 101)}, "cidade 101": {"cidade": strings.Repeat("c", 101)},
	} {
		if r := api(t, "POST", "/api/academias", "", base(extra)); r.Status != 400 {
			t.Errorf("%s: esperava 400, veio %d %s", nome, r.Status, r.Raw)
		}
	}
	for nome, extra := range map[string]map[string]any{
		"CEP com máscara": {"cep": "20010-000"}, "UF minúscula": {"uf": "rj"},
		"número texto": {"numero": "12A"},
	} {
		if r := api(t, "POST", "/api/academias", "", base(extra)); r.Status != 201 {
			t.Errorf("%s: esperava 201, veio %d %s", nome, r.Status, r.Raw)
		} else if d := r.data(); d["cep"] != "20010000" || d["uf"] != "RJ" {
			t.Errorf("%s: normalização: %v", nome, d)
		}
	}
	// Admin vê o endereço estruturado da pendente.
	b := base(map[string]any{"complemento": "Loja 2"})
	esperar(t, api(t, "POST", "/api/academias", "", b), 201)
	a := novoAdminDB(t, "superadmin")
	achou := ""
	for pag := 1; pag <= 200 && achou == ""; pag++ {
		r := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/academias?status=pendente&limite=100&pagina=%d", pag), a, nil), 200)
		for _, it := range itensDe(t, r) {
			if it["nome_fantasia"] == b["nome_fantasia"] {
				achou = fmt.Sprint(it)
			}
		}
		if len(itensDe(t, r)) == 0 {
			break
		}
	}
	if !strings.Contains(achou, "complemento:Loja 2") || !strings.Contains(achou, "cep:20010000") || !strings.Contains(achou, "uf:RJ") ||
		!strings.Contains(achou, "cidade:Rio de Janeiro") {
		t.Errorf("admin sem o endereço estruturado: %s", achou)
	}
}

// 00001–00020 do zero, Down até 0 (nenhuma tabela sobra) e Up de novo até 20 com o mesmo schema.
func TestQAR4_MigracoesDoZeroAte20DownUpCompleto(t *testing.T) {
	d, sqlDB := bancoQA(t, "r4zero", 20)
	if v := versaoGoose(t, d); v != 20 {
		t.Fatalf("versão %d", v)
	}
	s1 := qaSchema(t, d)
	for _, col := range []string{"academias|cep|", "academias|uf|", "academias|logradouro|"} {
		if !strings.Contains(s1, col) {
			t.Errorf("coluna %s ausente", col)
		}
	}
	p := providerQA(t, sqlDB)
	if _, err := p.DownTo(context.Background(), 0); err != nil {
		t.Fatalf("DownTo 0: %v", err)
	}
	var restantes []string
	d.Raw("SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema = DATABASE() AND TABLE_NAME <> 'goose_db_version'").Scan(&restantes)
	if len(restantes) != 0 {
		t.Errorf("Down até 0 deixou tabelas: %v", restantes)
	}
	var procs int64
	d.Raw("SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = DATABASE()").Scan(&procs)
	if procs != 0 {
		t.Errorf("Down deixou %d procedures", procs)
	}
	if _, err := p.UpTo(context.Background(), 20); err != nil {
		t.Fatalf("UpTo 20 de novo: %v", err)
	}
	if s2 := qaSchema(t, d); s2 != s1 {
		t.Errorf("schema difere após Down/Up")
	}
}
