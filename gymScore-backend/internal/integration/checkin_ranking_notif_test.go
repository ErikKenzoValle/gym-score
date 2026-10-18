//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/models"
)

var (
	pngMin  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0}, 64)...)
	jpgMin  = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 64)...)
	webpMin = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0}, 64)...)
)

// postCheckin envia multipart/form-data. foto == nil não anexa arquivo.
func postCheckin(t *testing.T, u *user, id uint, campos map[string]string, nomeArquivo string, foto []byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range campos {
		_ = w.WriteField(k, v)
	}
	if foto != nil {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="foto"; filename="%s"`, nomeArquivo)}
		h["Content-Type"] = []string{"image/jpeg"} // mentira proposital: o servidor deve olhar os bytes
		pw, _ := w.CreatePart(h)
		_, _ = pw.Write(foto)
	}
	_ = w.Close()
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/desafios/%d/checkins", id), &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+u.Token)
	res, err := servidor.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func arquivosUpload() int {
	n := 0
	_ = filepath.Walk(filepath.Join(uploadsDir, "checkins"), func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// desafioEmAndamento cria um desafio com n participantes e já inicia.
func desafioEmAndamento(t *testing.T, n int) (uint, []*user) {
	t.Helper()
	us := make([]*user, n)
	for i := range us {
		us[i] = usuarioComSaldo(t, 3000)
	}
	id := criarDesafio(t, us[0], 1000, n)
	for _, u := range us[1:] {
		esperar(t, acao(t, u, id, "entrar", nil), 200)
	}
	esperar(t, acao(t, us[0], id, "iniciar", nil), 200)
	return id, us
}

// ─── Check-ins ───────────────────────────────────────────────────────────────

func TestCheckin_TextoUmPorDia(t *testing.T) {
	id, us := desafioEmAndamento(t, 2)
	r := esperar(t, postCheckin(t, us[1], id, map[string]string{"texto": "Treino de pernas", "latitude": "-23.55", "longitude": "-46.63"}, "", nil), 201)
	d := r.data()
	if d["texto"] != "Treino de pernas" || d["foto_url"] != nil || d["usuario"].(map[string]any)["id"].(float64) != float64(us[1].ID) {
		t.Errorf("check-in inesperado: %v", d)
	}
	esperar(t, postCheckin(t, us[1], id, map[string]string{"texto": "de novo"}, "", nil), 409) // 1 por dia
	esperar(t, postCheckin(t, us[0], id, nil, "", nil), 201)                                   // outro participante pode; sem nada também
}

func TestCheckin_FotoSalvaComNomeAleatorioEServida(t *testing.T) {
	id, us := desafioEmAndamento(t, 2)
	antes := arquivosUpload()
	r := esperar(t, postCheckin(t, us[0], id, map[string]string{"texto": "com foto"}, "../../etc/passwd.jpg", jpgMin), 201)
	url := r.data()["foto_url"].(string)
	if !strings.HasPrefix(url, "/uploads/checkins/") || !strings.HasSuffix(url, ".jpg") || strings.Contains(url, "passwd") {
		t.Fatalf("URL deveria ter nome aleatório gerado pelo servidor: %s", url)
	}
	if arquivosUpload() != antes+1 {
		t.Error("deveria ter gravado exatamente 1 arquivo")
	}
	res := esperar(t, api(t, "GET", url, "", nil), 200) // público (nome não adivinhável)
	_ = res
	req := httptest.NewRequest("GET", url, nil)
	hr, _ := servidor.Test(req, -1)
	if ct := hr.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type deveria ser image/jpeg: %s", ct)
	}
	if hr.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("faltou X-Content-Type-Options: nosniff")
	}
	// diretório não é listável e arquivo inexistente dá 404
	if r := api(t, "GET", "/uploads/checkins/", "", nil); r.Status == 200 {
		t.Error("listagem de diretório não deve ser exposta")
	}
	esperar(t, api(t, "GET", "/uploads/checkins/naoexiste.jpg", "", nil), 404)

	// PNG e WEBP também são aceitos (outro dia/desafio)
	id2, us2 := desafioEmAndamento(t, 2)
	if u := esperar(t, postCheckin(t, us2[0], id2, nil, "a.png", pngMin), 201).data()["foto_url"].(string); !strings.HasSuffix(u, ".png") {
		t.Errorf("png: %s", u)
	}
	if u := esperar(t, postCheckin(t, us2[1], id2, nil, "a.webp", webpMin), 201).data()["foto_url"].(string); !strings.HasSuffix(u, ".webp") {
		t.Errorf("webp: %s", u)
	}
}

func TestCheckin_FotoInvalidaNaoGravaNada(t *testing.T) {
	id, us := desafioEmAndamento(t, 2)
	antes := arquivosUpload()
	// HTML/texto com nome .jpg e Content-Type image/jpeg
	esperar(t, postCheckin(t, us[0], id, nil, "x.jpg", []byte("<html><script>alert(1)</script></html>")), 400)
	esperar(t, postCheckin(t, us[0], id, nil, "x.jpg", []byte("GIF89a....")), 400)
	esperar(t, postCheckin(t, us[0], id, nil, "x.jpg", []byte{}), 400)
	// RIFF que não é WEBP
	esperar(t, postCheckin(t, us[0], id, nil, "x.webp", append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), make([]byte, 32)...)), 400)
	// maior que 3 MB
	grande := append(append([]byte{}, pngMin...), make([]byte, 3<<20)...)
	esperar(t, postCheckin(t, us[0], id, nil, "grande.png", grande), 413)
	// exatamente 3 MB passa
	limite := append(append([]byte{}, pngMin...), make([]byte, (3<<20)-len(pngMin))...)
	esperar(t, postCheckin(t, us[0], id, nil, "limite.png", limite), 201)
	if arquivosUpload() != antes+1 {
		t.Errorf("só o arquivo válido deveria ter sido gravado (antes=%d, agora=%d)", antes, arquivosUpload())
	}
}

func TestCheckin_RegrasDePapelEStatus(t *testing.T) {
	// desafio aberto
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 2)
	esperar(t, postCheckin(t, a, id, nil, "", nil), 409) // ainda não iniciou

	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	estranho := novoUsuario(t)
	esperar(t, postCheckin(t, estranho, id, nil, "", nil), 403)
	esperar(t, postCheckin(t, a, 99999999, nil, "", nil), 404)
	esperar(t, postCheckin(t, a, id, map[string]string{"latitude": "10"}, "", nil), 400)                     // só latitude
	esperar(t, postCheckin(t, a, id, map[string]string{"latitude": "100", "longitude": "10"}, "", nil), 400) // fora do intervalo
	esperar(t, postCheckin(t, a, id, map[string]string{"latitude": "abc", "longitude": "10"}, "", nil), 400) // não numérico
	esperar(t, postCheckin(t, a, id, map[string]string{"texto": strings.Repeat("x", 501)}, "", nil), 400)    // texto longo
	esperar(t, postCheckin(t, a, id, map[string]string{"texto": strings.Repeat("é", 500)}, "", nil), 201)    // 500 runes ok
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/checkins", id), "", nil), 401)

	// em votação: não aceita mais
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)
	esperar(t, postCheckin(t, b, id, nil, "", nil), 409)
}

func TestCheckin_ConcorrenciaMesmoDiaGravaUm(t *testing.T) {
	id, us := desafioEmAndamento(t, 2)
	antes := arquivosUpload()
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if postCheckin(t, us[0], id, nil, "c.jpg", jpgMin).Status == 201 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	var n int64
	db.Model(&models.DesafioCheckin{}).Where("id_desafio = ? AND id_usuario = ?", id, us[0].ID).Count(&n)
	if ok != 1 || n != 1 || arquivosUpload() != antes+1 {
		t.Fatalf("esperava 1 check-in e 1 arquivo, obteve ok=%d linhas=%d arquivos=%d", ok, n, arquivosUpload()-antes)
	}
}

func TestCheckin_ListagemEDetalhe(t *testing.T) {
	id, us := desafioEmAndamento(t, 3)
	esperar(t, postCheckin(t, us[0], id, map[string]string{"texto": "A"}, "", nil), 201)
	esperar(t, postCheckin(t, us[1], id, map[string]string{"texto": "B"}, "x.png", pngMin), 201)

	lista := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/checkins", id), us[2].Token, nil), 200).Body["data"].([]any)
	if len(lista) != 2 || lista[0].(map[string]any)["texto"] != "B" { // mais recente primeiro
		t.Errorf("lista inesperada: %v", lista)
	}
	estranho := novoUsuario(t)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/checkins", id), estranho.Token, nil), 403)

	d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), us[0].Token, nil), 200).data()
	if d["checkins_total"].(float64) != 2 || d["voce_fez_checkin_hoje"] != true {
		t.Errorf("detalhe sem resumo de check-ins: total=%v hoje=%v", d["checkins_total"], d["voce_fez_checkin_hoje"])
	}
	for _, p := range d["participantes"].([]any) {
		pm := p.(map[string]any)
		want := 0.0
		if uint(pm["id"].(float64)) != us[2].ID {
			want = 1
		}
		if pm["checkins"].(float64) != want {
			t.Errorf("participante %v: checkins=%v esperado %v", pm["id"], pm["checkins"], want)
		}
	}
	d2 := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), us[2].Token, nil), 200).data()
	if d2["voce_fez_checkin_hoje"] != false {
		t.Error("us[2] ainda não fez check-in hoje")
	}
}

// ─── Ranking ─────────────────────────────────────────────────────────────────

func TestRanking_SoVoceEAmigosComPesoDoCheckin(t *testing.T) {
	a, b, c := novoUsuario(t), novoUsuario(t), novoUsuario(t) // c não é amigo
	amigos(t, a, b)
	for i := 0; i < 2; i++ {
		esperar(t, api(t, "POST", "/api/treinos/concluir", a.Token, map[string]any{"grupo_muscular": "peito"}), 200)
	}
	for i := 0; i < 10; i++ {
		esperar(t, api(t, "POST", "/api/treinos/concluir", c.Token, map[string]any{"grupo_muscular": "peito"}), 200)
	}
	// b faz 1 check-in de desafio (3 pontos)
	depositar(t, b, 3000)
	depositar(t, a, 3000)
	id := criarDesafio(t, a, 1000, 2)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, postCheckin(t, b, id, nil, "", nil), 201)

	r := esperar(t, api(t, "GET", "/api/ranking/amigos", a.Token, nil), 200).Body["data"].([]any)
	if len(r) != 2 {
		t.Fatalf("ranking deve ter só eu + 1 amigo: %v", r)
	}
	p0, p1 := r[0].(map[string]any), r[1].(map[string]any)
	if uint(p0["id"].(float64)) != b.ID || p0["pontos"].(float64) != 3 || p0["checkins"].(float64) != 1 || p0["posicao"].(float64) != 1 {
		t.Errorf("1º lugar deveria ser b com 3 pontos: %v", p0)
	}
	if uint(p1["id"].(float64)) != a.ID || p1["pontos"].(float64) != 2 || p1["exercicios"].(float64) != 2 || p1["voce"] != true {
		t.Errorf("2º lugar deveria ser a (eu) com 2 pontos: %v", p1)
	}
	// c só vê a si mesmo
	rc := esperar(t, api(t, "GET", "/api/ranking/amigos", c.Token, nil), 200).Body["data"].([]any)
	if len(rc) != 1 || rc[0].(map[string]any)["pontos"].(float64) != 10 {
		t.Errorf("c deveria ver só ele com 10 pontos: %v", rc)
	}
}

func TestRanking_JanelaDeDiasEValidacao(t *testing.T) {
	a := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/treinos/concluir", a.Token, map[string]any{"grupo_muscular": "costas"}), 200)
	db.Exec("INSERT INTO treino_conclusoes (id_usuario, grupo_muscular, concluido_em) VALUES (?, 'pernas', ?)", a.ID, time.Now().AddDate(0, 0, -40))

	pontos := func(q string) float64 {
		r := esperar(t, api(t, "GET", "/api/ranking/amigos"+q, a.Token, nil), 200).Body["data"].([]any)
		return r[0].(map[string]any)["pontos"].(float64)
	}
	if p := pontos(""); p != 1 {
		t.Errorf("padrão (30 dias) deveria contar 1, contou %v", p)
	}
	if p := pontos("?dias=60"); p != 2 {
		t.Errorf("60 dias deveria contar 2, contou %v", p)
	}
	if p := pontos("?dias=7"); p != 1 {
		t.Errorf("7 dias deveria contar 1, contou %v", p)
	}
	esperar(t, api(t, "GET", "/api/ranking/amigos?dias=366", a.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/ranking/amigos?dias=-1", a.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/ranking/amigos", "", nil), 401)
}

// ─── Notificações ────────────────────────────────────────────────────────────

func listaNotif(t *testing.T, u *user) []map[string]any {
	t.Helper()
	r := esperar(t, api(t, "GET", "/api/notificacoes?limite=100", u.Token, nil), 200)
	var out []map[string]any
	for _, it := range r.data()["itens"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func contarNotif(t *testing.T, u *user, tipo string) int {
	t.Helper()
	n := 0
	for _, it := range listaNotif(t, u) {
		if it["tipo"] == tipo {
			n++
		}
	}
	return n
}

func TestNotif_EventosDeAmizade(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	if contarNotif(t, b, models.NotifAmizadeRecebida) != 1 || contarNotif(t, a, models.NotifAmizadeRecebida) != 0 {
		t.Fatal("só o destinatário é notificado da solicitação")
	}
	n := listaNotif(t, b)[0]
	if !strings.Contains(n["mensagem"].(string), a.Nome) || n["link"] != "/amigos" {
		t.Errorf("mensagem/link inesperados: %v", n)
	}
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	if contarNotif(t, a, models.NotifAmizadeAceita) != 1 {
		t.Error("o remetente deve ser notificado do aceite")
	}
	// aceitar de novo (404) não duplica
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 404)
	if contarNotif(t, a, models.NotifAmizadeAceita) != 1 {
		t.Error("não pode duplicar notificação")
	}
}

func TestNotif_CicloDoDesafio(t *testing.T) {
	a, b, c := usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000)
	amigo := novoUsuario(t)
	amigos(t, a, amigo)
	id := criarDesafio(t, a, 1000, 3)

	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 200)
	if contarNotif(t, amigo, models.NotifConviteDesafio) != 1 {
		t.Fatal("amigo convidado deveria receber o convite")
	}
	conv := listaNotif(t, amigo)[0]
	if !strings.Contains(conv["link"].(string), fmt.Sprintf("id=%d", id)) {
		t.Errorf("link do convite deveria apontar para o desafio: %v", conv["link"])
	}

	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, c, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	if contarNotif(t, b, models.NotifDesafioIniciado) != 1 || contarNotif(t, c, models.NotifDesafioIniciado) != 1 || contarNotif(t, a, models.NotifDesafioIniciado) != 0 {
		t.Error("iniciado: participantes (menos o criador) notificados")
	}
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)
	if contarNotif(t, b, models.NotifVotacaoAberta) != 1 || contarNotif(t, a, models.NotifVotacaoAberta) != 0 {
		t.Error("votação aberta: participantes (menos o criador) notificados")
	}
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": b.ID}), 200)
	esperar(t, acao(t, b, id, "votar", map[string]any{"id_votado": c.ID}), 200)
	esperar(t, acao(t, c, id, "votar", map[string]any{"id_votado": b.ID}), 200) // b vence e apura
	for _, u := range []*user{a, b, c} {
		if contarNotif(t, u, models.NotifResultado) != 1 {
			t.Errorf("usuário %d deveria receber o resultado exatamente uma vez", u.ID)
		}
	}
	vencedor := listaNotif(t, b)[0]
	if !strings.Contains(vencedor["mensagem"].(string), "venceu") || !strings.Contains(vencedor["mensagem"].(string), "30,00") {
		t.Errorf("mensagem do vencedor: %v", vencedor["mensagem"])
	}
	// reapurar (409) não notifica de novo
	esperar(t, acao(t, a, id, "apurar", nil), 409)
	if contarNotif(t, b, models.NotifResultado) != 1 {
		t.Error("reapuração não pode duplicar notificações")
	}
}

func TestNotif_CancelamentoEDeposito(t *testing.T) {
	a, b := usuarioComSaldo(t, 3000), usuarioComSaldo(t, 3000)
	if contarNotif(t, a, models.NotifDeposito) != 1 {
		t.Fatal("depósito confirmado deveria notificar uma vez")
	}
	// consultar o mesmo pagamento de novo não duplica
	var pid string
	db.Model(&models.Transacao{}).Select("asaas_payment_id").Where("id_usuario = ? AND tipo = 'deposito'", a.ID).Scan(&pid)
	esperar(t, api(t, "GET", "/api/pagamento/pix/"+pid, a.Token, nil), 200)
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_RECEIVED", pid), 200)
	if contarNotif(t, a, models.NotifDeposito) != 1 {
		t.Error("confirmação repetida não pode duplicar a notificação")
	}
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "cancelar", nil), 200)
	if contarNotif(t, b, models.NotifDesafioCancel) != 1 || contarNotif(t, a, models.NotifDesafioCancel) != 0 {
		t.Error("cancelamento notifica os participantes (exceto o criador)")
	}
}

func TestNotif_EndpointsEIsolamento(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	for i := 0; i < 3; i++ { // 3 solicitações de amigos diferentes para b
		x := novoUsuario(t)
		esperar(t, api(t, "POST", "/api/amigos/adicionar", x.Token, map[string]any{"id_amigo": b.ID}), 200)
	}
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)

	cont := func(u *user) float64 {
		return esperar(t, api(t, "GET", "/api/notificacoes/nao-lidas", u.Token, nil), 200).data()["nao_lidas"].(float64)
	}
	if cont(b) != 4 || cont(a) != 0 {
		t.Fatalf("contadores errados: b=%v a=%v", cont(b), cont(a))
	}
	lista := listaNotif(t, b)
	primeira := uint(lista[0]["id"].(float64))

	// uma notificação alheia não pode ser lida por outro usuário nem revelar existência
	esperar(t, api(t, "POST", fmt.Sprintf("/api/notificacoes/%d/lida", primeira), a.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/notificacoes/99999999/lida", b.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/notificacoes/abc/lida", b.Token, nil), 400)
	if cont(b) != 4 {
		t.Fatal("tentativa de outro usuário não pode marcar como lida")
	}
	esperar(t, api(t, "POST", fmt.Sprintf("/api/notificacoes/%d/lida", primeira), b.Token, nil), 200)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/notificacoes/%d/lida", primeira), b.Token, nil), 200) // idempotente
	if cont(b) != 3 {
		t.Errorf("deveria restar 3 não lidas, restam %v", cont(b))
	}
	// não lidas primeiro: a lida vai para o fim
	lista = listaNotif(t, b)
	if lista[len(lista)-1]["lida"] != true || lista[0]["lida"] != false {
		t.Errorf("ordem deveria ser não lidas primeiro: %v", lista)
	}
	// paginação
	p := esperar(t, api(t, "GET", "/api/notificacoes?limite=2&pagina=2", b.Token, nil), 200).data()
	if len(p["itens"].([]any)) != 2 || p["total"].(float64) != 4 || p["nao_lidas"].(float64) != 3 {
		t.Errorf("paginação errada: %v", p)
	}
	m := esperar(t, api(t, "POST", "/api/notificacoes/lidas", b.Token, nil), 200).data()
	if m["marcadas"].(float64) != 3 || cont(b) != 0 {
		t.Errorf("marcar todas: %v, restam %v", m, cont(b))
	}
	esperar(t, api(t, "GET", "/api/notificacoes", "", nil), 401)
}
