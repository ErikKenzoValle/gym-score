//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/app"
	"gymScore-backend/internal/jobs"
	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
)

// ─── Helpers da moderação ────────────────────────────────────────────────────

const descricaoValida = "Descrição detalhada do ocorrido, com mais de vinte caracteres."

// pngMinimo é o cabeçalho de um PNG (os magic bytes decidem o tipo) seguido de `extra` bytes.
func pngMinimo(extra int) []byte {
	b := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	return append(b, bytes.Repeat([]byte{0}, extra)...)
}

// enviarArquivo faz o POST multipart (campo `arquivo`) como o navegador faria, com o Bearer do usuário.
func enviarArquivo(t *testing.T, srv *fiber.App, caminho, token, nome string, conteudo []byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("arquivo", nome)
	_, _ = fw.Write(conteudo)
	_ = w.Close()
	req := httptest.NewRequest("POST", caminho, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

// baixar faz um GET e devolve status, headers e corpo cru (arquivos de evidência).
func baixar(t *testing.T, caminho, token string, cookieAdmin string) (int, http.Header, []byte) {
	t.Helper()
	req := httptest.NewRequest("GET", caminho, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookieAdmin != "" {
		req.AddCookie(&http.Cookie{Name: "admin_jwt", Value: cookieAdmin})
	}
	res, err := servidor.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, b
}

func denunciar(t *testing.T, quem *user, corpo map[string]any) resp {
	t.Helper()
	return api(t, "POST", "/api/denuncias", quem.Token, corpo)
}

func corpoDenuncia(alvo uint, categoria string, desafio uint) map[string]any {
	c := map[string]any{"id_denunciado": alvo, "categoria": categoria, "descricao": descricaoValida}
	if desafio != 0 {
		c["id_desafio"] = desafio
	}
	return c
}

func contarEventos(t *testing.T, acao, entidade string, id uint) int64 {
	t.Helper()
	var n int64
	db.Model(&models.ModeracaoEvento{}).Where("acao = ? AND entidade = ? AND id_entidade = ?", acao, entidade, id).Count(&n)
	return n
}

func contarNotifDB(t *testing.T, idUsuario uint, tipo string) int64 {
	t.Helper()
	var n int64
	db.Model(&models.Notificacao{}).Where("id_usuario = ? AND tipo = ?", idUsuario, tipo).Count(&n)
	return n
}

func tarefaModeracao(t *testing.T, nome string) jobs.Tarefa {
	t.Helper()
	_, tarefas := app.ConstruirComJobs(cfgBase, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
	for _, tf := range tarefas {
		if tf.Nome == nome {
			return tf
		}
	}
	t.Fatalf("tarefa %s não registrada", nome)
	return jobs.Tarefa{}
}

func emDoisDias() string { return time.Now().Add(48 * time.Hour).Format(time.RFC3339) }

// ─── Fluxo completo: denúncia ao admin, evidências, suspensão, recurso ───────

func TestModeracao_DenunciaAdminSuspensaoERecurso(t *testing.T) {
	a, b := novoUsuario(t), usuarioComSaldo(t, 5000)
	ad := novoAdminDB(t, models.PapelAdmin)

	// validações de borda
	esperar(t, denunciar(t, a, corpoDenuncia(a.ID, models.DenunciaCondutaAbusiva, 0)), 400) // autodenúncia
	esperar(t, denunciar(t, a, map[string]any{"id_denunciado": b.ID, "categoria": "conduta_abusiva", "descricao": "curta"}), 400)
	esperar(t, denunciar(t, a, corpoDenuncia(b.ID, "xpto", 0)), 400)
	esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaManipulacaoPodio, 0)), 400) // categoria de desafio sem desafio
	esperar(t, denunciar(t, a, corpoDenuncia(99999999, models.DenunciaOutro, 0)), 404)

	r := esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaCondutaAbusiva, 0)), 201)
	idDen := idDe(r)
	if r.data()["jurisdicao"] != "admin" || r.data()["status"] != "aberta" || strings.Contains(r.Raw, "denunciante") {
		t.Fatalf("denúncia criada: %s", r.Raw)
	}
	esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaCondutaAbusiva, 0)), 409) // uma aberta por caso

	// evidências: magic bytes, até 3, só o denunciante, pasta privada
	ev := fmt.Sprintf("/api/denuncias/%d/evidencias", idDen)
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "x.png", []byte("<html><script>alert(1)</script></html>")), 400)
	esperar(t, enviarArquivo(t, servidor, ev, b.Token, "x.png", pngMinimo(10)), 404) // não é o denunciante
	e1 := esperar(t, enviarArquivo(t, servidor, ev, a.Token, "../../etc/passwd.png", pngMinimo(100)), 201)
	if num(e1.data(), "ordem") != 1 || e1.data()["mime"] != "image/png" {
		t.Errorf("evidência 1: %s", e1.Raw)
	}
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "b.jpg", append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 50)...)), 201)
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "c.webp", []byte("RIFF\x20\x00\x00\x00WEBPVP8 \x00\x00\x00\x00")), 201)
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "d.png", pngMinimo(5)), 409) // já tem 3
	arquivos, _ := filepath.Glob(filepath.Join(evidenciasDir, "*"))
	if len(arquivos) < 3 {
		t.Fatalf("evidências deveriam estar em EVIDENCIAS_DIR: %v", arquivos)
	}
	for _, f := range arquivos {
		if strings.Contains(f, "passwd") {
			t.Errorf("nome original não pode ser usado: %s", f)
		}
		if st, _ := os.Stat(f); st.Mode().Perm()&0o077 != 0 {
			t.Errorf("evidência com permissão aberta: %v", st.Mode())
		}
	}
	if fora, _ := filepath.Glob(filepath.Join(uploadsDir, "*.png")); len(fora) > 0 {
		t.Errorf("evidência caiu na pasta pública: %v", fora)
	}
	nomeSalvo := filepath.Base(arquivos[0])
	if st, _, _ := baixar(t, "/uploads/"+nomeSalvo, "", ""); st == 200 {
		t.Error("evidência acessível pela rota pública /uploads")
	}
	st, h, corpo := baixar(t, ev+"/1", a.Token, "")
	if st != 200 || h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(h.Get("Content-Disposition"), "inline") || !bytes.Equal(corpo, pngMinimo(100)) {
		t.Errorf("download do denunciante: %d %v", st, h)
	}
	if st, _, _ := baixar(t, ev+"/1", b.Token, ""); st != 404 {
		t.Errorf("denunciado não pode ver a evidência: %d", st)
	}
	if st, _, _ := baixar(t, fmt.Sprintf("/api/admin/denuncias/%d/evidencias/2", idDen), "", ad.Cookie); st != 200 {
		t.Errorf("admin vê a evidência: %d", st)
	}

	// admin: fila com denunciante; academia não decide; decisão com suspensão de 2 dias
	l := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/denuncias?id_denunciado=%d&status=aberta,em_analise", b.ID), ad, nil), 200)
	its := itensDe(t, l)
	if len(its) != 1 || num(its[0], "id_denunciante") != int64(a.ID) {
		t.Fatalf("fila do admin: %s", l.Raw)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", idDen), ad, map[string]any{}), 400) // sem motivo
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", idDen), ad, motivo("vou analisar")), 200)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), ad, map[string]any{
		"status": "improcedente", "decisao_texto": "sem provas", "motivo": "triagem",
		"punicoes": []map[string]any{{"tipo": "advertencia"}},
	}), 400) // punição só em procedente
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), ad, map[string]any{
		"status": "procedente", "decisao_texto": "Ofensas comprovadas pelas imagens", "motivo": "denúncia comprovada",
		"punicoes": []map[string]any{{"tipo": "anulacao_resultado"}},
	}), 400) // anulação tem rota própria
	dec := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), ad, map[string]any{
		"status": "procedente", "decisao_texto": "Ofensas comprovadas pelas imagens", "motivo": "denúncia comprovada",
		"punicoes": []map[string]any{{"tipo": "suspensao", "fim": emDoisDias()}},
	}), 200)
	ps := dec.data()["punicoes"].([]any)
	if len(ps) != 1 || ps[0].(map[string]any)["tipo"] != "suspensao" || ps[0].(map[string]any)["aplicada_por"] != "admin" {
		t.Fatalf("punição da decisão: %s", dec.Raw)
	}
	idPun := uint(ps[0].(map[string]any)["id"].(float64))
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), ad, map[string]any{
		"status": "arquivada", "decisao_texto": "de novo", "motivo": "teste",
	}), 409)
	if contarAuditoria(t, "denuncia_decidir", "denuncia", idDen) != 1 || contarAuditoria(t, "denuncia_assumir", "denuncia", idDen) != 1 {
		t.Error("auditoria das ações do admin")
	}
	for _, acaoEv := range []string{"denuncia_criada", "evidencia_enviada", "denuncia_assumida", "denuncia_decidida"} {
		if contarEventos(t, acaoEv, "denuncia", idDen) < 1 {
			t.Errorf("evento %s ausente", acaoEv)
		}
	}
	var evAdmin models.ModeracaoEvento
	db.Where("acao = 'denuncia_decidida' AND id_entidade = ?", idDen).First(&evAdmin)
	if evAdmin.AtorPapel != "admin" || evAdmin.IDAdmin == nil || *evAdmin.IDAdmin != ad.ID || evAdmin.IDAtor != nil {
		t.Errorf("ator do evento do admin: %+v", evAdmin)
	}
	tl := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/moderacao/eventos?entidade=denuncia&id_entidade=%d", idDen), ad, nil), 200)
	if len(itensDe(t, tl)) < 4 {
		t.Errorf("linha do tempo: %s", tl.Raw)
	}

	// Q2: suspensão derruba a sessão; o suspenso faz login, lê, usa a carteira, mas não participa
	esperar(t, api(t, "GET", "/api/usuarios/me", b.Token, nil), 401)
	login := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": b.Email, "senha": b.Senha}), 200)
	b.Token = tokenDaSessao(t, login)
	if m, _ := login.data()["usuario"].(map[string]any)["moderacao"].(map[string]any); m == nil || m["suspenso_ate"] == nil {
		t.Errorf("login do suspenso deveria trazer moderacao.suspenso_ate: %s", login.Raw)
	}
	if me := esperar(t, api(t, "GET", "/api/usuarios/me", b.Token, nil), 200); me.data()["moderacao"] == nil {
		t.Errorf("/me do suspenso: %s", me.Raw)
	}
	esperar(t, api(t, "GET", "/api/carteira", b.Token, nil), 200)
	esperar(t, api(t, "GET", "/api/desafios", b.Token, nil), 200)
	sus := esperar(t, api(t, "POST", "/api/amigos/adicionar", b.Token, map[string]any{"id_amigo": a.ID}), 403)
	if sus.Body["codigo"] != "conta_suspensa" || sus.data()["suspenso_ate"] == nil {
		t.Errorf("403 conta_suspensa: %s", sus.Raw)
	}
	esperar(t, denunciar(t, b, corpoDenuncia(a.ID, models.DenunciaOutro, 0)), 403)

	// punido vê a punição e recorre (uma vez); denunciante vê a decisão sem detalhes de terceiros
	mp := esperar(t, api(t, "GET", "/api/punicoes/minhas", b.Token, nil), 200)
	if it := itensDe(t, mp); len(it) != 1 || it[0]["pode_recorrer"] != true || it[0]["vigente"] != true {
		t.Fatalf("minhas punições: %s", mp.Raw)
	}
	esperar(t, api(t, "POST", fmt.Sprintf("/api/punicoes/%d/recurso", idPun), b.Token, map[string]any{"texto": "curto"}), 400)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/punicoes/%d/recurso", idPun), a.Token, map[string]any{"texto": strings.Repeat("x", 30)}), 404)
	rec := esperar(t, api(t, "POST", fmt.Sprintf("/api/punicoes/%d/recurso", idPun), b.Token,
		map[string]any{"texto": "Fui provocado antes, tenho como provar isso."}), 201)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/punicoes/%d/recurso", idPun), b.Token,
		map[string]any{"texto": "Segundo recurso para a mesma punição."}), 409)
	minha := esperar(t, api(t, "GET", fmt.Sprintf("/api/denuncias/%d", idDen), a.Token, nil), 200)
	if minha.data()["status"] != "procedente" || minha.data()["gerou_punicao"] != true || strings.Contains(minha.Raw, "suspensao") {
		t.Errorf("visão do denunciante: %s", minha.Raw)
	}
	esperar(t, api(t, "GET", fmt.Sprintf("/api/denuncias/%d", idDen), b.Token, nil), 404) // denunciado nunca vê a denúncia
	if contarNotifDB(t, b.ID, models.NotifPunicaoAplicada) != 1 || contarNotifDB(t, a.ID, models.NotifDenunciaDecidida) != 1 {
		t.Error("notificações de punição/decisão")
	}

	// recurso deferido revoga a suspensão e libera a conta
	idRec := idDe(rec)
	fila := esperar(t, adm(t, "GET", "/api/admin/recursos?status=pendente&limite=100", ad, nil), 200)
	achou := false
	for _, it := range itensDe(t, fila) {
		if uint(num(it["recurso"].(map[string]any), "id")) == idRec {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("recurso fora da fila: %s", fila.Raw)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/recursos/%d/julgar", idRec), ad, map[string]any{
		"status": "deferido", "decisao_texto": "Provocação comprovada", "motivo": "recurso procedente",
	}), 200)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/recursos/%d/julgar", idRec), ad, map[string]any{
		"status": "indeferido", "decisao_texto": "x de novo", "motivo": "teste",
	}), 409)
	var u models.Usuario
	db.First(&u, b.ID)
	if u.SuspensoAte != nil || u.MotivoBloqueio != nil {
		t.Errorf("suspensão deveria ter sido recalculada para nada: %+v", u)
	}
	esperar(t, api(t, "POST", "/api/amigos/adicionar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/punicoes/%d/revogar", idPun), ad, motivo("de novo")), 409) // já revogada
}

// ─── Jurisdição da academia, punição da academia, encaminhar e escalonar ─────

func TestModeracao_JurisdicaoAcademia(t *testing.T) {
	c := novoCenario(t)
	ad := novoAdminDB(t, models.PapelAdmin)
	x, y, z := usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000)
	d := criarDesafio(t, c, 1000, 10)
	inscrever(t, d, x, y, z)

	// suborno de instrutor em desafio de academia: jurisdição da academia, com prazo
	r := esperar(t, denunciar(t, x, corpoDenuncia(y.ID, models.DenunciaSubornoInstrutor, d)), 201)
	idDen := idDe(r)
	if r.data()["jurisdicao"] != "academia" {
		t.Fatalf("jurisdição: %s", r.Raw)
	}
	if contarNotifDB(t, c.A.ID, models.NotifDenunciaRecebida) != 1 {
		t.Error("academia deveria ser avisada")
	}
	// denunciado precisa ter vínculo com o desafio
	fora := novoUsuario(t)
	esperar(t, denunciar(t, x, corpoDenuncia(fora.ID, models.DenunciaSubornoInstrutor, d)), 400)
	// denúncia contra a conta da própria academia vai ao admin (conflito de interesse)
	ra := esperar(t, denunciar(t, z, corpoDenuncia(c.A.ID, models.DenunciaSubornoInstrutor, d)), 201)
	if ra.data()["jurisdicao"] != "admin" {
		t.Errorf("denúncia contra a academia: %s", ra.Raw)
	}

	// sigilo: a academia vê a denúncia sem o denunciante
	fila := esperar(t, api(t, "GET", "/api/academia/denuncias", c.A.Token, nil), 200)
	if !contemID(fila, idDen) || strings.Contains(fila.Raw, "denunciante") || strings.Contains(fila.Raw, x.Email) {
		t.Fatalf("fila da academia: %s", fila.Raw)
	}
	det := esperar(t, api(t, "GET", fmt.Sprintf("/api/academia/denuncias/%d", idDen), c.A.Token, nil), 200)
	if strings.Contains(det.Raw, "denunciante") || strings.Contains(det.Raw, x.Email) || strings.Contains(det.Raw, fmt.Sprintf(`"id":%d,"nome":"%s`, x.ID, x.Nome)) {
		t.Errorf("detalhe da academia vaza o denunciante: %s", det.Raw)
	}
	outra, _ := novaAcademia(t)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/academia/denuncias/%d", idDen), outra.Token, nil), 404)
	esperar(t, api(t, "GET", "/api/academia/denuncias", x.Token, nil), 403) // não é academia

	// admin não decide enquanto a jurisdição é da academia
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", idDen), ad, motivo("tentar")), 409)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), ad, map[string]any{
		"status": "arquivada", "decisao_texto": "Arquivar sem análise", "motivo": "tentar"}), 409)

	// academia assume e decide com bloqueio na academia; não pode suspender
	esperar(t, api(t, "POST", fmt.Sprintf("/api/academia/denuncias/%d/assumir", idDen), c.A.Token, nil), 200)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/academia/denuncias/%d/decidir", idDen), c.A.Token, map[string]any{
		"status": "procedente", "decisao_texto": "Tentou subornar o instrutor", "punicao": map[string]any{"tipo": "suspensao", "fim": emDoisDias()},
	}), 403)
	dec := esperar(t, api(t, "POST", fmt.Sprintf("/api/academia/denuncias/%d/decidir", idDen), c.A.Token, map[string]any{
		"status": "procedente", "decisao_texto": "Tentou subornar o instrutor", "punicao": map[string]any{"tipo": "bloqueio_academia"},
	}), 200)
	if dec.data()["status"] != "procedente" {
		t.Errorf("decisão da academia: %s", dec.Raw)
	}
	var p models.Punicao
	db.Where("id_denuncia = ?", idDen).First(&p)
	if p.Tipo != "bloqueio_academia" || p.Escopo != "academia" || p.IDAcademia == nil || *p.IDAcademia != c.AcademiaID ||
		p.AplicadaPor == nil || *p.AplicadaPor != c.A.ID || p.AplicadaPorAdmin != nil {
		t.Fatalf("punição da academia: %+v", p)
	}
	pa := esperar(t, api(t, "GET", "/api/academia/punicoes?vigente=true", c.A.Token, nil), 200)
	if !contemID(pa, p.ID) {
		t.Errorf("punições da academia: %s", pa.Raw)
	}
	// efeito do bloqueio: y não entra em outro desafio desta academia (mas entra nos de outra)
	d2 := criarDesafio(t, c, 500, 5)
	bl := esperar(t, acao(t, y, d2, "entrar", nil), 403)
	if !strings.Contains(bl.Raw, "bloqueado") {
		t.Errorf("bloqueio na academia: %s", bl.Raw)
	}
	c2 := novoCenario(t)
	esperar(t, acao(t, y, criarDesafio(t, c2, 500, 5), "entrar", nil), 200)

	// encaminhar: a academia passa a outra denúncia ao admin (só ida)
	r2 := esperar(t, denunciar(t, z, corpoDenuncia(y.ID, models.DenunciaSubornoInstrutor, d)), 201)
	id2 := idDe(r2)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/academia/denuncias/%d/encaminhar", id2), c.A.Token, map[string]any{"justificativa": "Conflito: envolve nosso gerente"}), 200)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/academia/denuncias/%d", id2), c.A.Token, nil), 404)
	var den2 models.Denuncia
	db.First(&den2, id2)
	if den2.Jurisdicao != "admin" || den2.EncaminhadaEm == nil || den2.IDAcademiaJulgadora == nil || den2.Status != "aberta" {
		t.Errorf("denúncia encaminhada: %+v", den2)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", id2), ad, motivo("encaminhada")), 200)

	// escalonamento por prazo (job, ator sistema)
	r3 := esperar(t, denunciar(t, y, corpoDenuncia(z.ID, models.DenunciaSubornoInstrutor, d)), 201)
	id3 := idDe(r3)
	db.Exec("UPDATE denuncias SET prazo_academia_ate = NOW(3) - INTERVAL 1 MINUTE WHERE id = ?", id3)
	jobs.ExecutarAgora(tarefaModeracao(t, "escalonar-denuncias"))
	var den3 models.Denuncia
	db.First(&den3, id3)
	if den3.Jurisdicao != "admin" || den3.EncaminhadaEm == nil {
		t.Fatalf("escalonamento: %+v", den3)
	}
	var evs models.ModeracaoEvento
	if err := db.Where("acao = 'denuncia_escalonada' AND id_entidade = ?", id3).First(&evs).Error; err != nil ||
		evs.AtorPapel != "sistema" || evs.IDAtor != nil || evs.IDAdmin != nil {
		t.Errorf("evento de escalonamento: %+v %v", evs, err)
	}
	if contarNotifDB(t, c.A.ID, models.NotifDenunciaEscalonada) < 1 {
		t.Error("academia avisada do escalonamento")
	}
	jobs.ExecutarAgora(tarefaModeracao(t, "escalonar-denuncias")) // idempotente
	if contarEventos(t, "denuncia_escalonada", "denuncia", id3) != 1 {
		t.Error("escalonamento repetido")
	}
}

// ─── Q1: banido recorre pela tela de login (rota pública, sem sessão) ────────

func TestModeracao_BanidoRecorrePeloLogin(t *testing.T) {
	u := novoUsuario(t)
	ad := novoAdminDB(t, models.PapelAdmin)
	esperar(t, adm(t, "POST", "/api/admin/punicoes", ad, map[string]any{
		"id_usuario": u.ID, "tipo": "banimento", "fim": emDoisDias(), "justificativa": "Fraude", "motivo": "teste"}), 400) // banimento sem fim
	pr := esperar(t, adm(t, "POST", "/api/admin/punicoes", ad, map[string]any{
		"id_usuario": u.ID, "tipo": "banimento", "justificativa": "Fraude em pagamentos", "motivo": "investigação PIX"}), 201)
	idPun := idDe(pr)
	esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 401) // sessões derrubadas

	lg := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 403)
	if lg.Body["codigo"] != "conta_banida" || num(lg.data(), "id_punicao") != int64(idPun) || lg.data()["pode_recorrer"] != true ||
		lg.data()["motivo"] != "Fraude em pagamentos" || len(lg.Cookies) != 0 {
		t.Fatalf("login do banido: %s %v", lg.Raw, lg.Cookies)
	}
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "senha-errada-1"}), 401)

	rota := "/api/recursos/banimento"
	esperar(t, api(t, "POST", rota, "", map[string]any{"email": u.Email, "senha": "senha-errada-1", "texto": strings.Repeat("a", 30)}), 401)
	esperar(t, api(t, "POST", rota, "", map[string]any{"email": "ninguem@gymscore.test", "senha": "x"}), 401)
	consulta := esperar(t, api(t, "POST", rota, "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	if consulta.data()["recurso"] != nil || consulta.data()["pode_recorrer"] != true {
		t.Errorf("consulta sem recurso: %s", consulta.Raw)
	}
	esperar(t, api(t, "POST", rota, "", map[string]any{"email": u.Email, "senha": u.Senha, "texto": "curto"}), 400)
	criado := esperar(t, api(t, "POST", rota, "", map[string]any{"email": u.Email, "senha": u.Senha,
		"texto": "Os pagamentos foram feitos pela minha empresa, posso comprovar."}), 201)
	if len(criado.Cookies) != 0 {
		t.Error("a rota pública não pode emitir sessão")
	}
	rec := criado.data()["recurso"].(map[string]any)
	if rec["status"] != "pendente" || criado.data()["pode_recorrer"] != false {
		t.Errorf("recurso criado: %s", criado.Raw)
	}
	again := esperar(t, api(t, "POST", rota, "", map[string]any{"email": u.Email, "senha": u.Senha, "texto": strings.Repeat("b", 30)}), 200)
	if num(again.data()["recurso"].(map[string]any), "id") != num(rec, "id") {
		t.Errorf("segundo envio deveria devolver o mesmo recurso: %s", again.Raw)
	}
	var evRec models.ModeracaoEvento
	db.Where("acao = 'recurso_aberto' AND id_entidade = ?", uint(num(rec, "id"))).First(&evRec)
	if evRec.AtorPapel != "usuario" || evRec.IDAtor == nil || *evRec.IDAtor != u.ID {
		t.Errorf("evento do recurso: %+v", evRec)
	}

	// R3-1/MB7: senha errada, conta inexistente e conta não banida (senha certa) → mesma resposta
	v := novoUsuario(t)
	r1 := api(t, "POST", rota, "", map[string]any{"email": u.Email, "senha": "senha-errada-1"})
	r2 := api(t, "POST", rota, "", map[string]any{"email": "ninguem-mb7@gymscore.test", "senha": "qualquer-123"})
	r3 := api(t, "POST", rota, "", map[string]any{"email": v.Email, "senha": v.Senha})
	if r1.Status != 401 || r2.Status != 401 || r3.Status != 401 || r1.Raw != r2.Raw || r2.Raw != r3.Raw {
		t.Errorf("respostas distinguíveis: %d %s | %d %s | %d %s", r1.Status, r1.Raw, r2.Status, r2.Raw, r3.Status, r3.Raw)
	}

	// deferido: login volta
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/recursos/%d/julgar", uint(num(rec, "id"))), ad, map[string]any{
		"status": "deferido", "decisao_texto": "Comprovado", "motivo": "documentos ok"}), 200)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	if contarAuditoria(t, "recurso_julgar", "recurso", uint(num(rec, "id"))) != 1 || contarAuditoria(t, "punicao_aplicar", "usuario", u.ID) != 1 {
		t.Error("auditoria")
	}
}

// ─── Bloqueio unificado: painel, suspensões sobrepostas, fim de suspensão ────

func TestModeracao_BloqueioUnificadoESuspensoes(t *testing.T) {
	u := novoUsuario(t)
	ad := novoAdminDB(t, models.PapelAdmin)
	// "bloquear" do painel = banimento com linha em punicoes
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), ad, motivo("fraude")), 200)
	var n int64
	db.Model(&models.Punicao{}).Where("id_usuario = ? AND tipo = 'banimento' AND revogada_em IS NULL", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("bloquear deveria criar 1 banimento: %d", n)
	}
	l := esperar(t, adm(t, "GET", "/api/admin/usuarios?situacao=banido&q="+url.QueryEscape(u.Email), ad, nil), 200)
	if len(itensDe(t, l)) != 1 {
		t.Errorf("filtro situacao=banido: %s", l.Raw)
	}
	esperar(t, adm(t, "GET", "/api/admin/usuarios?situacao=xpto", ad, nil), 400)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/desbloquear", u.ID), ad, motivo("resolvido")), 200)
	db.Model(&models.Punicao{}).Where("id_usuario = ? AND tipo = 'banimento' AND revogada_em IS NULL", u.ID).Count(&n)
	if n != 0 {
		t.Fatalf("desbloquear deveria revogar: %d", n)
	}
	u.Token = tokenNovo(t, u)

	// duas suspensões: suspenso_ate = maior fim; revogar a maior recalcula para a menor
	curta := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	longa := time.Now().Add(72 * time.Hour).Truncate(time.Second)
	p1 := idDe(esperar(t, adm(t, "POST", "/api/admin/punicoes", ad, map[string]any{"id_usuario": u.ID, "tipo": "suspensao",
		"fim": curta.Format(time.RFC3339), "justificativa": "Primeira", "motivo": "teste"}), 201))
	p2 := idDe(esperar(t, adm(t, "POST", "/api/admin/punicoes", ad, map[string]any{"id_usuario": u.ID, "tipo": "suspensao",
		"fim": longa.Format(time.RFC3339), "justificativa": "Segunda", "motivo": "teste"}), 201))
	var us models.Usuario
	us = models.Usuario{}
	db.First(&us, u.ID)
	if us.SuspensoAte == nil || !us.SuspensoAte.Equal(longa) || us.MotivoBloqueio == nil || *us.MotivoBloqueio != "Segunda" {
		t.Fatalf("suspenso_ate deveria ser a maior: %v %v", us.SuspensoAte, us.MotivoBloqueio)
	}
	l = esperar(t, adm(t, "GET", "/api/admin/usuarios?situacao=suspenso&q="+url.QueryEscape(u.Email), ad, nil), 200)
	if its := itensDe(t, l); len(its) != 1 || its[0]["suspenso"] != true {
		t.Errorf("filtro situacao=suspenso: %s", l.Raw)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/punicoes/%d/revogar", p2), ad, motivo("excesso")), 200)
	us = models.Usuario{}
	db.First(&us, u.ID)
	if us.SuspensoAte == nil || !us.SuspensoAte.Equal(curta) {
		t.Fatalf("revogar a maior deveria recalcular para a menor: %v", us.SuspensoAte)
	}

	// fim de suspensão: o acesso volta em tempo real (sem job); o job limpa, registra e avisa
	vencida := time.Now().Add(-time.Minute).Truncate(time.Second)
	if err := db.Exec("UPDATE punicoes SET inicio = ?, fim = ? WHERE id = ?", vencida.Add(-48*time.Hour), vencida, p1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE usuarios SET suspenso_ate = ? WHERE id = ?", vencida, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	u.Token = tokenNovo(t, u)
	// passa do BloquearSuspenso e da checagem do serviço (400 = categoria inválida, não 403)
	esperar(t, denunciar(t, u, corpoDenuncia(ad.ID, "xpto", 0)), 400)
	jobs.ExecutarAgora(tarefaModeracao(t, "fim-suspensao"))
	us = models.Usuario{}
	db.First(&us, u.ID)
	if us.SuspensoAte != nil || us.MotivoBloqueio != nil {
		t.Errorf("job deveria limpar suspenso_ate: %v", us.SuspensoAte)
	}
	if contarEventos(t, "punicao_expirada", "punicao", p1) != 1 || contarNotifDB(t, u.ID, models.NotifSuspensaoEncerrada) != 1 {
		t.Errorf("evento/aviso de fim de suspensão: %d %d", contarEventos(t, "punicao_expirada", "punicao", p1), contarNotifDB(t, u.ID, models.NotifSuspensaoEncerrada))
	}
	jobs.ExecutarAgora(tarefaModeracao(t, "fim-suspensao")) // idempotente
	if contarEventos(t, "punicao_expirada", "punicao", p1) != 1 {
		t.Error("job repetido gerou evento de novo")
	}
}

// ─── Concorrência: decisões simultâneas na mesma denúncia ────────────────────

func TestModeracao_DecisoesSimultaneasNaMesmaDenuncia(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	adm1, adm2 := novoAdminDB(t, models.PapelAdmin), novoAdminDB(t, models.PapelSuperadmin)
	idDen := idDe(esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaPerfilFalso, 0)), 201))
	const n = 8
	codigos := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			quem := adm1
			if i%2 == 1 {
				quem = adm2
			}
			r, _ := chamarAdmin(t, servidor, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), quem.Cookie, map[string]any{
				"status": "procedente", "decisao_texto": fmt.Sprintf("Decisão %d", i), "motivo": "corrida",
				"punicoes": []map[string]any{{"tipo": "suspensao", "fim": emDoisDias()}, {"tipo": "advertencia"}},
			})
			codigos[i] = r.Status
		}(i)
	}
	wg.Wait()
	ok, conflito := 0, 0
	for _, c := range codigos {
		switch c {
		case 200:
			ok++
		case 409:
			conflito++
		}
	}
	if ok != 1 || conflito != n-1 {
		t.Fatalf("esperava 1 vencedor e %d conflitos: %v", n-1, codigos)
	}
	var punicoes, eventos, auditoria int64
	db.Model(&models.Punicao{}).Where("id_denuncia = ?", idDen).Count(&punicoes)
	db.Model(&models.ModeracaoEvento{}).Where("acao = 'denuncia_decidida' AND id_entidade = ?", idDen).Count(&eventos)
	db.Model(&models.AdminAuditoria{}).Where("acao = 'denuncia_decidir' AND alvo_id = ?", idDen).Count(&auditoria)
	if punicoes != 2 || eventos != 1 || auditoria != 1 {
		t.Errorf("efeitos duplicados: punições %d eventos %d auditoria %d", punicoes, eventos, auditoria)
	}
	var tv []int
	db.Raw("SELECT token_versao FROM usuarios WHERE id = ?", b.ID).Scan(&tv)
	if len(tv) != 1 || tv[0] != 1 {
		t.Errorf("token_versao deveria subir uma vez: %v", tv)
	}
}

// ─── Concorrência: julgamentos simultâneos do mesmo recurso ──────────────────

func TestModeracao_JulgamentosSimultaneosDoMesmoRecurso(t *testing.T) {
	u := novoUsuario(t)
	adm1, adm2 := novoAdminDB(t, models.PapelAdmin), novoAdminDB(t, models.PapelAdmin)
	idPun := idDe(esperar(t, adm(t, "POST", "/api/admin/punicoes", adm1, map[string]any{"id_usuario": u.ID, "tipo": "suspensao",
		"fim": emDoisDias(), "justificativa": "Conduta", "motivo": "teste"}), 201))
	u.Token = tokenNovo(t, u)
	idRec := idDe(esperar(t, api(t, "POST", fmt.Sprintf("/api/punicoes/%d/recurso", idPun), u.Token,
		map[string]any{"texto": "Peço revisão: não houve conduta inadequada."}), 201))
	const n = 8
	codigos := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			quem, st := adm1, "deferido"
			if i%2 == 1 {
				quem, st = adm2, "indeferido"
			}
			r, _ := chamarAdmin(t, servidor, "POST", fmt.Sprintf("/api/admin/recursos/%d/julgar", idRec), quem.Cookie, map[string]any{
				"status": st, "decisao_texto": "Julgamento concorrente", "motivo": "corrida"})
			codigos[i] = r.Status
		}(i)
	}
	// em paralelo, uma revogação direta da mesma punição (só pode revogar uma vez no total)
	var revogou int
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, _ := chamarAdmin(t, servidor, "POST", fmt.Sprintf("/api/admin/punicoes/%d/revogar", idPun), adm2.Cookie, motivo("revogação direta"))
		revogou = r.Status
	}()
	wg.Wait()
	ok := 0
	for _, c := range codigos {
		if c == 200 {
			ok++
		} else if c != 409 {
			t.Errorf("status inesperado: %v", codigos)
		}
	}
	if ok != 1 {
		t.Fatalf("esperava exatamente 1 julgamento: %v", codigos)
	}
	var rc models.Recurso
	db.First(&rc, idRec)
	var p models.Punicao
	db.First(&p, idPun)
	if rc.Status == "pendente" || rc.JulgadoEm == nil || rc.JulgadoPor == nil {
		t.Errorf("recurso: %+v", rc)
	}
	if p.RevogadaEm == nil && (rc.Status == "deferido" || revogou == 200) {
		t.Errorf("punição deveria estar revogada: recurso %s, revogação direta %d", rc.Status, revogou)
	}
	var revs int64
	db.Model(&models.ModeracaoEvento{}).Where("acao = 'punicao_revogada' AND id_entidade = ?", idPun).Count(&revs)
	if p.RevogadaEm != nil && revs != 1 {
		t.Errorf("revogação registrada %d vezes", revs)
	}
	var us models.Usuario
	us = models.Usuario{}
	db.First(&us, u.ID)
	if (p.RevogadaEm != nil) == (us.SuspensoAte != nil) {
		t.Errorf("estado da conta inconsistente com a punição: revogada=%v suspenso_ate=%v", p.RevogadaEm, us.SuspensoAte)
	}
}

// ─── Concorrência: "bloquear" simultâneo e limite diário ─────────────────────

func TestModeracao_BloquearSimultaneoELimiteDiario(t *testing.T) {
	u := novoUsuario(t)
	adm1 := novoAdminDB(t, models.PapelAdmin)
	const n = 6
	alterados := make([]any, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, _ := chamarAdmin(t, servidor, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), adm1.Cookie, motivo("corrida"))
			alterados[i] = r.data()["alterado"]
		}(i)
	}
	wg.Wait()
	verdadeiros := 0
	for _, v := range alterados {
		if v == true {
			verdadeiros++
		}
	}
	var bans int64
	db.Model(&models.Punicao{}).Where("id_usuario = ? AND tipo = 'banimento'", u.ID).Count(&bans)
	if verdadeiros != 1 || bans != 1 {
		t.Fatalf("bloquear simultâneo: %v, banimentos %d", alterados, bans)
	}

	// limite por denunciante em 24h, inclusive com requisições simultâneas
	cfg := *cfgBase
	cfg.DenunciaLimiteDia = 2
	srv := novoServidor(&cfg)
	a := novoUsuario(t)
	alvos := []*user{novoUsuario(t), novoUsuario(t), novoUsuario(t), novoUsuario(t), novoUsuario(t)}
	cod := make([]int, len(alvos))
	for i, alvo := range alvos {
		wg.Add(1)
		go func(i int, alvo *user) {
			defer wg.Done()
			cod[i] = chamar(t, srv, "POST", "/api/denuncias", a.Token, corpoDenuncia(alvo.ID, models.DenunciaOutro, 0)).Status
		}(i, alvo)
	}
	wg.Wait()
	criadas, limitadas := 0, 0
	for _, c := range cod {
		switch c {
		case 201:
			criadas++
		case 429:
			limitadas++
		}
	}
	if criadas != 2 || limitadas != 3 {
		t.Errorf("limite diário: %v", cod)
	}
}

// ─── Limite de corpo: 1 MiB nas rotas JSON, limite próprio no upload ─────────

func TestModeracao_LimiteDeCorpoProprioDoUpload(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	idDen := idDe(esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaOutro, 0)), 201))
	ev := fmt.Sprintf("/api/denuncias/%d/evidencias", idDen)
	// 1,5 MiB: acima do limite geral, abaixo do da evidência (2 MiB)
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "grande.png", pngMinimo(1536*1024)), 201)
	// acima do limite da evidência
	r := enviarArquivo(t, servidor, ev, a.Token, "enorme.png", pngMinimo(2*1024*1024+10))
	if r.Status != 413 {
		t.Errorf("evidência acima do limite: %d %s", r.Status, r.Raw)
	}
	// JSON de 1,5 MiB em outra rota: 413 (o limite maior vale só no upload)
	grande := map[string]any{"email": "x@y.com", "senha": strings.Repeat("a", 1536*1024)}
	if r := api(t, "POST", "/api/login", "", grande); r.Status != 413 {
		t.Errorf("rota JSON deveria manter 1 MiB: %d", r.Status)
	}
	if r := api(t, "POST", "/api/denuncias", a.Token, map[string]any{"descricao": strings.Repeat("a", 1536*1024)}); r.Status != 413 {
		t.Errorf("POST /api/denuncias deveria manter 1 MiB: %d", r.Status)
	}
}

// ─── Retenção dos pagamentos pendentes pela denúncia de desafio ──────────────
// (os pendentes nascem no pódio — parte 2; aqui são inseridos direto para provar o gancho da denúncia)

func TestModeracao_DenunciaDeDesafioRetemPendentes(t *testing.T) {
	c := novoCenario(t)
	ad := novoAdminDB(t, models.PapelAdmin)
	x, y, z := usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000)
	d := criarDesafio(t, c, 1000, 10)
	inscrever(t, d, x, y, z)
	pend := models.PagamentoPendente{IDDesafio: d, IDUsuario: &y.ID, Tipo: models.PendentePremio, ValorCentavos: 1500,
		Status: models.PendenteALiberar, LiberarEm: time.Now().Add(48 * time.Hour)}
	if err := db.Create(&pend).Error; err != nil {
		t.Fatal(err)
	}
	statusPend := func() (string, *uint) {
		var p models.PagamentoPendente
		db.First(&p, pend.ID)
		return p.Status, p.IDDenunciaRetencao
	}
	// conduta abusiva no desafio não retém
	esperar(t, denunciar(t, z, corpoDenuncia(y.ID, models.DenunciaCondutaAbusiva, d)), 201)
	if st, _ := statusPend(); st != models.PendenteALiberar {
		t.Fatalf("conduta abusiva não deveria reter: %s", st)
	}
	d1 := idDe(esperar(t, denunciar(t, x, corpoDenuncia(y.ID, models.DenunciaManipulacaoPodio, d)), 201))
	if st, por := statusPend(); st != models.PendenteRetido || por == nil || *por != d1 {
		t.Fatalf("manipulação de pódio deveria reter: %s %v", st, por)
	}
	d2 := idDe(esperar(t, denunciar(t, y, corpoDenuncia(x.ID, models.DenunciaFraudePresenca, d)), 201))
	// julgar a 1ª com outra retentora aberta: a retenção passa para a 2ª
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", d1), ad, map[string]any{
		"status": "improcedente", "decisao_texto": "Sem indícios", "motivo": "análise"}), 200)
	if st, por := statusPend(); st != models.PendenteRetido || por == nil || *por != d2 {
		t.Fatalf("retenção deveria passar para a outra denúncia: %s %v", st, por)
	}
	// julgar a última retentora: volta a a_liberar
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", d2), ad, map[string]any{
		"status": "arquivada", "decisao_texto": "Duplicada", "motivo": "análise"}), 200)
	if st, por := statusPend(); st != models.PendenteALiberar || por != nil {
		t.Fatalf("sem retentora aberta o pendente volta a a_liberar: %s %v", st, por)
	}
	if contarEventos(t, "pagamento_reaberto", "desafio", d) != 1 || contarEventos(t, "pagamento_retido", "desafio", d) != 2 {
		t.Error("eventos de retenção")
	}
}
