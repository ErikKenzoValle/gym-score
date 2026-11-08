//go:build integration

package integration

// Testes independentes do QA (rodada 3 — moderação reconstruída). Não reutilizam os cenários dos
// devs: só os helpers genéricos do harness (usuários, academia, desafio, admin, pódio).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/app"
	"gymScore-backend/internal/jobs"
	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ─── helpers do QA ───────────────────────────────────────────────────────────

// qaChamarCookie chama a API autenticando SÓ pelo cookie jwt (como o navegador), com o Origin dado.
func qaChamarCookie(t *testing.T, metodo, caminho, token, origin string, corpo []byte, ctype string) resp {
	t.Helper()
	req := httptest.NewRequest(metodo, caminho, bytes.NewReader(corpo))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.AddCookie(&http.Cookie{Name: "jwt", Value: token})
	res, err := servidor.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Raw: string(raw)}
}

// qaAdminOrigin chama o painel com o cookie admin e um Origin arbitrário.
func qaAdminOrigin(t *testing.T, metodo, caminho string, a *adminT, origin string, corpo any) resp {
	t.Helper()
	b, _ := jsonBytes(corpo)
	req := httptest.NewRequest(metodo, caminho, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.AddCookie(&http.Cookie{Name: "admin_jwt", Value: a.Cookie})
	res, err := servidor.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Raw: string(raw)}
}

// qaSemDenunciante falha se a resposta (não-admin) trouxer qualquer identificação do denunciante.
func qaSemDenunciante(t *testing.T, onde string, r resp, den *user) {
	t.Helper()
	if strings.Contains(r.Raw, "denunciante") || strings.Contains(r.Raw, den.Email) ||
		strings.Contains(r.Raw, `"`+den.Nome+`"`) || strings.Contains(r.Raw, fmt.Sprintf(`"id_usuario":%d,`, den.ID)) {
		t.Errorf("SIGILO: %s expõe o denunciante %d (%s): %s", onde, den.ID, den.Email, r.Raw)
	}
}

func qaJPEG(n int) []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xDB}, bytes.Repeat([]byte{1}, n)...)
}

func qaRecurso(t *testing.T, u *user, idPun uint) resp {
	return api(t, "POST", fmt.Sprintf("/api/punicoes/%d/recurso", idPun), u.Token,
		map[string]any{"texto": "Contesto esta punição: tenho provas de que não fiz isso."})
}

func qaReceitasDesafio(t *testing.T, id uint) (taxa, estorno int64) {
	t.Helper()
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").Where("id_desafio = ? AND origem = 'taxa_inscricao'", id).Scan(&taxa)
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").Where("id_desafio = ? AND origem = 'estorno_taxa'", id).Scan(&estorno)
	return
}

// ─── 1. Migrations do zero + Down/Up completo (schema idêntico) ─────────────

func qaSchema(t *testing.T, d *gorm.DB) string {
	t.Helper()
	var nome string
	d.Raw("SELECT DATABASE()").Scan(&nome)
	var partes []string
	for _, q := range []string{
		"SELECT CONCAT_WS('|',TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,IFNULL(COLUMN_DEFAULT,'∅'),EXTRA,IFNULL(GENERATION_EXPRESSION,'')) FROM information_schema.columns WHERE table_schema = ? AND TABLE_NAME <> 'goose_db_version'",
		"SELECT CONCAT_WS('|',TABLE_NAME,INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME) FROM information_schema.statistics WHERE table_schema = ? AND TABLE_NAME <> 'goose_db_version'",
		"SELECT CONCAT_WS('|',cc.CONSTRAINT_NAME,cc.CHECK_CLAUSE) FROM information_schema.check_constraints cc WHERE cc.CONSTRAINT_SCHEMA = ?",
		"SELECT CONCAT_WS('|',TABLE_NAME,CONSTRAINT_NAME,COLUMN_NAME,IFNULL(REFERENCED_TABLE_NAME,'')) FROM information_schema.key_column_usage WHERE table_schema = ? AND REFERENCED_TABLE_NAME IS NOT NULL",
		"SELECT CONCAT_WS('|',ROUTINE_NAME,ROUTINE_TYPE) FROM information_schema.routines WHERE ROUTINE_SCHEMA = ?",
		"SELECT CONCAT_WS('|',TRIGGER_NAME,EVENT_OBJECT_TABLE) FROM information_schema.triggers WHERE TRIGGER_SCHEMA = ?",
	} {
		var linhas []string
		if err := d.Raw(q, nome).Scan(&linhas).Error; err != nil {
			t.Fatal(err)
		}
		partes = append(partes, linhas...)
	}
	sort.Strings(partes)
	return strings.Join(partes, "\n")
}

func TestQA_R3_MigracoesDoZeroDownUpCompleto(t *testing.T) {
	d, sqlDB := bancoQA(t, "r3zero", 19)
	if v := versaoGoose(t, d); v != 19 {
		t.Fatalf("versão %d", v)
	}
	s1 := qaSchema(t, d)
	for _, tb := range []string{"denuncias", "denuncia_evidencias", "punicoes", "recursos", "pagamentos_pendentes", "usuario_debitos", "moderacao_eventos"} {
		if !strings.Contains(s1, tb+"|id|") {
			t.Errorf("tabela %s ausente", tb)
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
	if _, err := p.UpTo(context.Background(), 19); err != nil {
		t.Fatalf("UpTo 19 de novo: %v", err)
	}
	if s2 := qaSchema(t, d); s2 != s1 {
		a, b := strings.Split(s1, "\n"), strings.Split(s2, "\n")
		ma := map[string]bool{}
		for _, x := range a {
			ma[x] = true
		}
		for _, x := range b {
			if !ma[x] {
				t.Errorf("schema difere após Down/Up: %s", x)
			}
		}
		t.Errorf("schema 19 → 0 → 19 diferente (%d vs %d linhas)", len(a), len(b))
	}
	// Down/Up parcial (19 → 12 → 19)
	if _, err := p.DownTo(context.Background(), 12); err != nil {
		t.Fatalf("DownTo 12: %v", err)
	}
	if _, err := p.UpTo(context.Background(), 19); err != nil {
		t.Fatalf("UpTo 19: %v", err)
	}
	if s3 := qaSchema(t, d); s3 != s1 {
		t.Error("schema 19 → 12 → 19 diferente")
	}
}

// ─── 2. Denúncia: limite/dia, uma aberta por caso, evidências, sigilo, corpo ─

func TestQA_R3_DenunciaLimiteUnicidadeEvidenciasESigilo(t *testing.T) {
	cfg := *cfgBase
	cfg.DenunciaLimiteDia = 3
	cfg.DenunciaEvidenciaMaxBytes = 4096
	srv := novoServidor(&cfg)
	ad := novoAdminDB(t, models.PapelAdmin)
	den := novoUsuario(t)
	alvos := []*user{usuarioComSaldo(t, 3000), novoUsuario(t), novoUsuario(t), novoUsuario(t)}

	// limite/dia (3) — a 4ª é 429; a repetida é 409 e não conta
	var ids []uint
	for i := 0; i < 3; i++ {
		r := esperar(t, chamar(t, srv, "POST", "/api/denuncias", den.Token, corpoDenuncia(alvos[i].ID, models.DenunciaCondutaAbusiva, 0)), 201)
		ids = append(ids, idDe(r))
	}
	if r := chamar(t, srv, "POST", "/api/denuncias", den.Token, corpoDenuncia(alvos[0].ID, models.DenunciaPerfilFalso, 0)); r.Status != 409 && r.Status != 429 {
		t.Errorf("mesmo caso (denunciante, denunciado, sem desafio) com outra categoria: %d %s", r.Status, r.Raw)
	}
	r4 := chamar(t, srv, "POST", "/api/denuncias", den.Token, corpoDenuncia(alvos[3].ID, models.DenunciaCondutaAbusiva, 0))
	if r4.Status != 429 {
		t.Errorf("limite diário: esperava 429, veio %d %s", r4.Status, r4.Raw)
	}
	// limite é por denunciante: outro usuário denuncia normalmente
	esperar(t, chamar(t, srv, "POST", "/api/denuncias", alvos[3].Token, corpoDenuncia(alvos[0].ID, models.DenunciaOutro, 0)), 201)

	// uma aberta por caso: no servidor padrão (sem limite) a duplicata é 409
	esperar(t, denunciar(t, den, corpoDenuncia(alvos[1].ID, models.DenunciaCondutaAbusiva, 0)), 409)

	idDen := ids[0]
	alvo := alvos[0]
	ev := fmt.Sprintf("/api/denuncias/%d/evidencias", idDen)
	// magic bytes: GIF, texto, RIFF não-WebP, vazio, SVG → 400
	for nome, conteudo := range map[string][]byte{
		"a.gif":  []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00"),
		"b.png":  []byte("isto nao e imagem nenhuma"),
		"c.webp": []byte("RIFF\x20\x00\x00\x00AVI LIST\x00\x00"),
		"d.png":  {},
		"e.svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"f.jpg":  {0xFF, 0xD8},
	} {
		if r := enviarArquivo(t, srv, ev, den.Token, nome, conteudo); r.Status != 400 {
			t.Errorf("magic bytes: %s aceito (%d %s)", nome, r.Status, r.Raw)
		}
	}
	// limite de corpo da rota (4096): 5000 bytes → 413 (ou 400), nada gravado
	if r := enviarArquivo(t, srv, ev, den.Token, "grande.png", pngMinimo(5000)); r.Status != 413 && r.Status != 400 {
		t.Errorf("evidência acima do limite: %d %s", r.Status, r.Raw)
	}
	var nEv int64
	db.Model(&models.DenunciaEvidencia{}).Where("id_denuncia = ?", idDen).Count(&nEv)
	if nEv != 0 {
		t.Fatalf("nenhuma evidência deveria ter sido gravada: %d", nEv)
	}
	// JSON de 2 MiB numa rota comum → 413 (o limite grande é só do upload)
	grande := corpoDenuncia(alvos[2].ID, models.DenunciaOutro, 0)
	grande["descricao"] = strings.Repeat("x", 2<<20)
	if r := api(t, "POST", "/api/denuncias", alvos[1].Token, grande); r.Status != 413 {
		t.Errorf("corpo JSON grande: %d", r.Status)
	}
	// terceiro / denunciado não enviam evidência
	esperar(t, enviarArquivo(t, srv, ev, alvo.Token, "x.png", pngMinimo(10)), 404)
	esperar(t, enviarArquivo(t, srv, ev, alvos[1].Token, "x.png", pngMinimo(10)), 404)
	esperar(t, enviarArquivo(t, srv, ev, den.Token, "x.png", pngMinimo(10)), 201)
	esperar(t, enviarArquivo(t, srv, ev, den.Token, "y.jpg", qaJPEG(10)), 201)
	esperar(t, enviarArquivo(t, srv, ev, den.Token, "z.webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8L\x00\x00\x00\x00")), 201)
	esperar(t, enviarArquivo(t, srv, ev, den.Token, "w.png", pngMinimo(10)), 409)

	// leitura da evidência: denunciante e admin; 404 para denunciado, terceiro, academia qualquer, ordem fora
	if st, h, _ := baixar(t, ev+"/2", den.Token, ""); st != 200 || h.Get("Content-Type") != "image/jpeg" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Cache-Control"), "no-store") {
		t.Errorf("evidência do denunciante: %d %v", st, h)
	}
	outraAcad, _ := novaAcademia(t)
	for nome, tk := range map[string]string{"denunciado": alvo.Token, "terceiro": alvos[1].Token, "academia": outraAcad.Token} {
		if st, _, _ := baixar(t, ev+"/1", tk, ""); st != 404 {
			t.Errorf("evidência para %s: %d", nome, st)
		}
		if st, _, _ := baixar(t, fmt.Sprintf("/api/academia/denuncias/%d/evidencias/1", idDen), tk, ""); st != 404 && st != 403 {
			t.Errorf("evidência (rota da academia) para %s: %d", nome, st)
		}
	}
	if st, _, _ := baixar(t, ev+"/4", den.Token, ""); st != 404 && st != 400 {
		t.Errorf("ordem 4: %d", st)
	}
	if st, _, _ := baixar(t, "/api/denuncias/999999999/evidencias/1", den.Token, ""); st != 404 {
		t.Errorf("denúncia inexistente: %d", st)
	}
	if st, _, _ := baixar(t, fmt.Sprintf("/api/admin/denuncias/%d/evidencias/3", idDen), "", ad.Cookie); st != 200 {
		t.Errorf("admin: %d", st)
	}
	// IDOR: terceiro/denunciado não leem a denúncia
	esperar(t, api(t, "GET", fmt.Sprintf("/api/denuncias/%d", idDen), alvo.Token, nil), 404)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/denuncias/%d", idDen), alvos[1].Token, nil), 404)

	// decisão do admin com advertência + suspensão; sigilo em TODAS as respostas do denunciado
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", idDen), ad, motivo("analisar")), 200)
	dec := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", idDen), ad, map[string]any{
		"status": "procedente", "decisao_texto": "Conduta comprovada", "motivo": "qa-teste",
		"punicoes": []map[string]any{{"tipo": "advertencia"}, {"tipo": "suspensao", "fim": emDoisDias()}},
	}), 200)
	if !strings.Contains(dec.Raw, den.Email) && !strings.Contains(dec.Raw, fmt.Sprintf(`"id_denunciante":%d`, den.ID)) {
		t.Errorf("admin deveria ver o denunciante: %s", dec.Raw)
	}
	// evidência após decidir: não aceita mais
	if r := enviarArquivo(t, srv, ev, den.Token, "k.png", pngMinimo(1)); r.Status == 201 {
		t.Error("evidência aceita em denúncia decidida")
	}
	lg := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": alvo.Email, "senha": alvo.Senha}), 200)
	alvo.Token = tokenDaSessao(t, lg)
	qaSemDenunciante(t, "login do punido", lg, den)
	for _, rota := range []string{"/api/usuarios/me", "/api/punicoes/minhas", "/api/notificacoes", "/api/notificacoes?limite=100",
		"/api/denuncias/minhas", "/api/carteira", "/api/carteira/pendentes", "/api/carteira/debitos",
		fmt.Sprintf("/api/usuarios/%d/publico", alvo.ID)} {
		r := api(t, "GET", rota, alvo.Token, nil)
		if r.Status != 200 {
			t.Errorf("%s: %d", rota, r.Status)
		}
		qaSemDenunciante(t, rota, r, den)
	}
	// recurso do punido também não revela
	var idSusp uint
	db.Model(&models.Punicao{}).Select("id").Where("id_denuncia = ? AND tipo = 'suspensao'", idDen).Scan(&idSusp)
	rr := esperar(t, qaRecurso(t, alvo, idSusp), 201)
	qaSemDenunciante(t, "recurso", rr, den)
	// o denunciante vê só o status, sem a punição
	md := esperar(t, api(t, "GET", fmt.Sprintf("/api/denuncias/%d", idDen), den.Token, nil), 200)
	if strings.Contains(md.Raw, "suspensao") || strings.Contains(md.Raw, "advertencia") || md.data()["gerou_punicao"] != true {
		t.Errorf("visão do denunciante: %s", md.Raw)
	}
	if lst := esperar(t, api(t, "GET", "/api/denuncias/minhas", den.Token, nil), 200); len(itensDe(t, lst)) < 3 {
		t.Errorf("minhas denúncias: %s", lst.Raw)
	} else if !strings.Contains(lst.Raw, `"ordem":3`) || strings.Contains(lst.Raw, "caminho") || strings.Contains(lst.Raw, evidenciasDir) {
		t.Errorf("minhas denúncias: evidências (sem caminho interno): %s", lst.Raw)
	}
}

// ─── 3. Jurisdição, escalonamento por job, IDOR de academia, punição por papel ─

func TestQA_R3_JurisdicaoEscalonamentoEPapeis(t *testing.T) {
	c := novoCenario(t)
	c2 := novoCenario(t)
	ad := novoAdminDB(t, models.PapelAdmin)
	x, y := usuarioComSaldo(t, 5000), usuarioComSaldo(t, 5000)
	d := criarDesafio(t, c, 1000, 10)
	inscrever(t, d, x, y)

	// x denuncia o INSTRUTOR por suborno → jurisdição da academia dona, com prazo
	r := esperar(t, denunciar(t, x, corpoDenuncia(c.I.ID, models.DenunciaSubornoInstrutor, d)), 201)
	idDen := idDe(r)
	if r.data()["jurisdicao"] != "academia" {
		t.Fatalf("jurisdição: %s", r.Raw)
	}
	esperar(t, enviarArquivo(t, servidor, fmt.Sprintf("/api/denuncias/%d/evidencias", idDen), x.Token, "a.png", pngMinimo(10)), 201)
	// outras categorias do mesmo desafio vão ao admin
	ro := esperar(t, denunciar(t, x, corpoDenuncia(y.ID, models.DenunciaFraudePresenca, d)), 201)
	if ro.data()["jurisdicao"] != "admin" {
		t.Errorf("fraude_presenca deveria ir ao admin: %s", ro.Raw)
	}

	// IDOR: outra academia, instrutor, participante comum
	base := fmt.Sprintf("/api/academia/denuncias/%d", idDen)
	for _, rt := range []struct{ m, p string }{{"GET", base}, {"GET", base + "/evidencias/1"}, {"POST", base + "/assumir"},
		{"POST", base + "/encaminhar"}, {"POST", base + "/decidir"}} {
		var corpo any
		if rt.m == "POST" {
			corpo = map[string]any{"justificativa": "tentativa de outra academia", "status": "improcedente", "decisao_texto": "tentativa de outra academia"}
		}
		if rr := api(t, rt.m, rt.p, c2.A.Token, corpo); rr.Status != 404 {
			t.Errorf("IDOR academia: %s %s por outra academia → %d %s", rt.m, rt.p, rr.Status, rr.Raw)
		}
		if rr := api(t, rt.m, rt.p, c.I.Token, corpo); rr.Status != 403 && rr.Status != 404 {
			t.Errorf("instrutor em %s %s → %d", rt.m, rt.p, rr.Status)
		}
	}
	if fl := esperar(t, api(t, "GET", "/api/academia/denuncias", c2.A.Token, nil), 200); contemID(fl, idDen) {
		t.Error("fila da outra academia mostra a denúncia")
	}
	// sigilo para a academia julgadora
	fl := esperar(t, api(t, "GET", "/api/academia/denuncias", c.A.Token, nil), 200)
	qaSemDenunciante(t, "fila da academia", fl, x)
	dt := esperar(t, api(t, "GET", base, c.A.Token, nil), 200)
	qaSemDenunciante(t, "detalhe da academia", dt, x)
	if st, _, _ := baixar(t, base+"/evidencias/1", c.A.Token, ""); st != 200 {
		t.Errorf("academia julgadora deveria ver a evidência: %d", st)
	}
	// a fila do instrutor-denunciado e as notificações dele não revelam nada
	for _, rota := range []string{"/api/notificacoes", "/api/denuncias/minhas", "/api/punicoes/minhas"} {
		qaSemDenunciante(t, "instrutor "+rota, esperar(t, api(t, "GET", rota, c.I.Token, nil), 200), x)
	}

	// prazo vence → job escalona; academia perde acesso (inclusive à evidência)
	db.Exec("UPDATE denuncias SET prazo_academia_ate = NOW(3) - INTERVAL 1 SECOND WHERE id = ?", idDen)
	jobs.ExecutarAgora(tarefaModeracao(t, "escalonar-denuncias"))
	jobs.ExecutarAgora(tarefaModeracao(t, "escalonar-denuncias"))
	var dn models.Denuncia
	db.First(&dn, idDen)
	if dn.Jurisdicao != "admin" || dn.EncaminhadaEm == nil || contarEventos(t, "denuncia_escalonada", "denuncia", idDen) != 1 {
		t.Fatalf("escalonamento: %+v", dn)
	}
	if st, _, _ := baixar(t, base+"/evidencias/1", c.A.Token, ""); st != 404 {
		t.Errorf("academia ainda vê a evidência depois do escalonamento: %d", st)
	}
	if rr := api(t, "POST", base+"/decidir", c.A.Token, map[string]any{"status": "improcedente", "decisao_texto": "tarde demais para decidir"}); rr.Status == 200 {
		t.Error("academia decidiu denúncia escalonada")
	}

	// academia: só advertência/bloqueio_academia; ator gravado na coluna de usuário
	r2 := esperar(t, denunciar(t, y, corpoDenuncia(c.I.ID, models.DenunciaSubornoInstrutor, d)), 201)
	id2 := idDe(r2)
	b2 := fmt.Sprintf("/api/academia/denuncias/%d", id2)
	esperar(t, api(t, "POST", b2+"/assumir", c.A.Token, nil), 200)
	for _, tp := range []string{"suspensao", "banimento", "anulacao_resultado"} {
		corpo := map[string]any{"status": "procedente", "decisao_texto": "Suborno comprovado", "punicao": map[string]any{"tipo": tp, "fim": emDoisDias()}}
		if rr := api(t, "POST", b2+"/decidir", c.A.Token, corpo); rr.Status != 403 && rr.Status != 400 {
			t.Errorf("academia aplicou %s: %d %s", tp, rr.Status, rr.Raw)
		}
	}
	esperar(t, api(t, "POST", b2+"/decidir", c.A.Token, map[string]any{"status": "procedente", "decisao_texto": "Suborno comprovado", "punicao": map[string]any{"tipo": "advertencia"}}), 200)
	var pa models.Punicao
	db.Where("id_denuncia = ?", id2).First(&pa)
	if pa.AplicadaPor == nil || *pa.AplicadaPor != c.A.ID || pa.AplicadaPorAdmin != nil || pa.Escopo != "academia" || pa.IDAcademia == nil || *pa.IDAcademia != c.AcademiaID {
		t.Errorf("punição da academia: %+v", pa)
	}
	var evA models.ModeracaoEvento
	db.Where("acao = 'denuncia_decidida' AND id_entidade = ?", id2).First(&evA)
	if evA.AtorPapel != "academia" || evA.IDAtor == nil || *evA.IDAtor != c.A.ID || evA.IDAdmin != nil {
		t.Errorf("evento da academia: %+v", evA)
	}

	// admin: punição direta grava aplicada_por_admin, revogação grava revogada_por (admin), auditoria + evento
	pr := esperar(t, adm(t, "POST", "/api/admin/punicoes", ad, map[string]any{"id_usuario": y.ID, "tipo": "advertencia",
		"justificativa": "Comportamento inadequado", "motivo": "qa-teste"}), 201)
	idP := idDe(pr)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/punicoes/%d/revogar", idP), ad, motivo("qa revogou")), 200)
	var pp models.Punicao
	db.First(&pp, idP)
	if pp.AplicadaPor != nil || pp.AplicadaPorAdmin == nil || *pp.AplicadaPorAdmin != ad.ID || pp.RevogadaPor == nil || *pp.RevogadaPor != ad.ID {
		t.Errorf("atores admin da punição: %+v", pp)
	}
	var nAud int64
	db.Model(&models.AdminAuditoria{}).Where("id_admin = ? AND acao IN ('punicao_aplicar','punicao_revogar')", ad.ID).Count(&nAud)
	if nAud != 2 {
		t.Errorf("auditoria das ações do admin: %d", nAud)
	}
	var nEvAdm int64
	db.Model(&models.ModeracaoEvento{}).Where("id_admin = ? AND ator_papel = 'admin' AND id_ator IS NULL", ad.ID).Count(&nEvAdm)
	if nEvAdm < 2 {
		t.Errorf("eventos com ator admin: %d", nEvAdm)
	}
	// admin não decide denúncia da academia enquanto a jurisdição é dela
	r3 := esperar(t, denunciar(t, y, corpoDenuncia(c.I.ID, models.DenunciaSubornoInstrutor, d)), 201)
	if rr := adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", idDe(r3)), ad, motivo("furar a fila")); rr.Status != 409 {
		t.Errorf("admin assumiu denúncia da academia no prazo: %d", rr.Status)
	}
}

// ─── 4. Suspensão (bloqueia × permite), banimento, recurso pelo login, rate limit ─

func TestQA_R3_SuspensaoBanimentoRecurso(t *testing.T) {
	ad := novoAdminDB(t, models.PapelAdmin)
	c := novoCenario(t)
	u := usuarioComSaldo(t, 5000)
	outro := novoUsuario(t)
	d1 := criarDesafio(t, c, 1000, 10)
	d2 := criarDesafio(t, c, 1000, 10)
	inscrever(t, d1, u)
	tokenAntigo := u.Token

	pr := esperar(t, adm(t, "POST", "/api/admin/punicoes", ad, map[string]any{"id_usuario": u.ID, "tipo": "suspensao", "fim": emDoisDias(),
		"justificativa": "Ofensas no chat", "motivo": "qa-teste"}), 201)
	idSusp := idDe(pr)
	esperar(t, api(t, "GET", "/api/usuarios/me", tokenAntigo, nil), 401) // sessão derrubada
	lg := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	u.Token = tokenDaSessao(t, lg)

	// permite
	for _, rota := range []string{"/api/usuarios/me", "/api/carteira", "/api/notificacoes", "/api/denuncias/minhas", "/api/punicoes/minhas", "/api/desafios", fmt.Sprintf("/api/desafios/%d", d2)} {
		esperar(t, api(t, "GET", rota, u.Token, nil), 200)
	}
	if rs := sacar(t, u, 500); rs.Status != 201 {
		t.Errorf("suspenso deveria sacar (só débito bloqueia): %d %s", rs.Status, rs.Raw)
	}
	if rs := acao(t, u, d1, "sair", nil); rs.Status != 200 {
		t.Errorf("suspenso deveria poder sair do desafio: %d %s", rs.Status, rs.Raw)
	}
	// bloqueia (403 conta_suspensa com data)
	for nome, rr := range map[string]resp{
		"entrar":   acao(t, u, d2, "entrar", nil),
		"denúncia": denunciar(t, u, corpoDenuncia(outro.ID, models.DenunciaOutro, 0)),
		"amizade":  api(t, "POST", "/api/amigos/adicionar", u.Token, map[string]any{"id_amigo": outro.ID}),
	} {
		if rr.Status != 403 || rr.Body["codigo"] != "conta_suspensa" || rr.data()["suspenso_ate"] == nil {
			t.Errorf("suspenso em %s: %d %s", nome, rr.Status, rr.Raw)
		}
	}
	// recurso liberado
	esperar(t, qaRecurso(t, u, idSusp), 201)
	esperar(t, qaRecurso(t, u, idSusp), 409)
	esperar(t, qaRecurso(t, outro, idSusp), 404) // IDOR

	// fim de suspensão pelo job: punição com fim no passado → conta volta
	db.Exec("UPDATE punicoes SET inicio = NOW(3) - INTERVAL 1 DAY, fim = NOW(3) - INTERVAL 2 SECOND WHERE id = ?", idSusp)
	db.Exec("UPDATE usuarios SET suspenso_ate = NOW() - INTERVAL 1 SECOND WHERE id = ?", u.ID)
	jobs.ExecutarAgora(tarefaModeracao(t, "fim-suspensao"))
	var uu models.Usuario
	db.First(&uu, u.ID)
	if uu.SuspensoAte != nil {
		t.Errorf("fim-suspensao não limpou suspenso_ate: %v", uu.SuspensoAte)
	}
	esperar(t, api(t, "POST", "/api/amigos/adicionar", u.Token, map[string]any{"id_amigo": outro.ID}), 200)

	// banimento pelo painel (bloquear) → sessão cai; login 403 conta_banida sem cookie
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), ad, motivo("fraude grave")), 200)
	if r := api(t, "GET", "/api/usuarios/me", u.Token, nil); r.Status != 401 && r.Status != 403 {
		t.Errorf("token do banido: %d", r.Status)
	}
	if r := api(t, "GET", "/api/carteira", u.Token, nil); r.Status == 200 {
		t.Error("banido acessa a carteira com o token antigo")
	}
	lb := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 403)
	if lb.Body["codigo"] != "conta_banida" || len(lb.Cookies) != 0 || lb.data()["id_punicao"] == nil {
		t.Fatalf("login do banido: %s", lb.Raw)
	}
	// senha errada no login do banido NÃO revela o banimento
	if lw := api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "errada-123456"}); lw.Status != 401 || strings.Contains(lw.Raw, "banid") {
		t.Errorf("login do banido com senha errada vaza o estado: %d %s", lw.Status, lw.Raw)
	}
	idBan := uint(num(lb.data(), "id_punicao"))
	rb := "/api/recursos/banimento"
	if r := api(t, "POST", rb, "", map[string]any{"email": u.Email, "senha": "errada-123456", "texto": strings.Repeat("a", 40)}); r.Status != 401 || strings.Contains(r.Raw, "banid") {
		t.Errorf("recurso com senha errada: %d %s", r.Status, r.Raw)
	}
	cr := esperar(t, api(t, "POST", rb, "", map[string]any{"email": u.Email, "senha": u.Senha, "texto": "Nunca cometi fraude; peço revisão do banimento."}), 201)
	if len(cr.Cookies) != 0 {
		t.Error("rota pública emitiu cookie")
	}
	idRec := uint(num(cr.data()["recurso"].(map[string]any), "id"))
	var nr int64
	db.Model(&models.Recurso{}).Where("id_punicao = ?", idBan).Count(&nr)
	if nr != 1 {
		t.Errorf("recursos do banimento: %d", nr)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/recursos/%d/julgar", idRec), ad, map[string]any{"status": "deferido", "decisao_texto": "Erro do sistema", "motivo": "qa-teste"}), 200)
	var rec models.Recurso
	db.First(&rec, idRec)
	if rec.JulgadoPor == nil || *rec.JulgadoPor != ad.ID {
		t.Errorf("julgado_por deveria ser o admin %d: %+v", ad.ID, rec.JulgadoPor)
	}
	var pb models.Punicao
	db.First(&pb, idBan)
	if pb.RevogadaEm == nil || pb.RevogadaPor == nil || *pb.RevogadaPor != ad.ID {
		t.Errorf("banimento deferido não revogado com o admin: %+v", pb)
	}
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)

	// rate limit da rota pública (RATE_LIMIT_MAX=3 neste servidor)
	cfg := *cfgBase
	cfg.RateLimitMax = 3
	srv := novoServidor(&cfg)
	st := []int{}
	for i := 0; i < 5; i++ {
		st = append(st, chamar(t, srv, "POST", rb, "", map[string]any{"email": outro.Email, "senha": "errada-" + fmt.Sprint(i)}).Status)
	}
	if st[3] != 429 || st[4] != 429 {
		t.Errorf("rate limit do recurso público: %v", st)
	}

	// MB7: o contador por IP é o mesmo do /api/login (não dobra o orçamento de tentativas)
	srv2 := novoServidor(&cfg)
	mix := []int{}
	for i := 0; i < 4; i++ {
		rota := "/api/login"
		if i%2 == 1 {
			rota = rb
		}
		mix = append(mix, chamar(t, srv2, "POST", rota, "", map[string]any{"email": outro.Email, "senha": "errada-" + fmt.Sprint(i)}).Status)
	}
	if mix[0] != 401 || mix[1] != 401 || mix[2] != 401 || mix[3] != 429 {
		t.Errorf("rate limit compartilhado login × recurso: %v", mix)
	}
}

// ─── 5. CSRF por Origin nas rotas novas (cookie de sessão / cookie admin) ─────

func TestQA_R3_CSRFNasRotasNovas(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	ad := novoAdminDB(t, models.PapelSuperadmin)
	mal := "https://evil.example"
	corpo, _ := jsonBytes(corpoDenuncia(b.ID, models.DenunciaOutro, 0))
	if r := qaChamarCookie(t, "POST", "/api/denuncias", a.Token, mal, corpo, "application/json"); r.Status != 403 {
		t.Errorf("POST /api/denuncias com Origin alheio: %d", r.Status)
	}
	if r := qaChamarCookie(t, "POST", "/api/denuncias", a.Token, "", corpo, "application/json"); r.Status != 403 {
		t.Errorf("POST /api/denuncias só com cookie e sem Origin: %d", r.Status)
	}
	r := qaChamarCookie(t, "POST", "/api/denuncias", a.Token, origemTeste, corpo, "application/json")
	if r.Status != 201 {
		t.Fatalf("POST /api/denuncias com Origin próprio: %d %s", r.Status, r.Raw)
	}
	for _, p := range []string{"/api/denuncias/1/evidencias", "/api/punicoes/1/recurso", "/api/academia/denuncias/1/assumir",
		"/api/academia/denuncias/1/decidir", "/api/academia/denuncias/1/encaminhar"} {
		if rr := qaChamarCookie(t, "POST", p, a.Token, mal, []byte(`{}`), "application/json"); rr.Status != 403 || !strings.Contains(rr.Raw, "Origem") {
			t.Errorf("CSRF %s: %d %s", p, rr.Status, rr.Raw)
		}
	}
	for _, p := range []string{"/api/admin/punicoes", "/api/admin/punicoes/1/revogar", "/api/admin/denuncias/1/assumir", "/api/admin/denuncias/1/decidir",
		"/api/admin/recursos/1/julgar", "/api/admin/desafios/1/anular-resultado", "/api/admin/usuarios/1/bloquear"} {
		if rr := qaAdminOrigin(t, "POST", p, ad, mal, map[string]any{"motivo": "x"}); rr.Status != 403 {
			t.Errorf("CSRF admin %s: %d %s", p, rr.Status, rr.Raw)
		}
		if rr := qaAdminOrigin(t, "POST", p, ad, "", map[string]any{"motivo": "x"}); rr.Status != 403 {
			t.Errorf("CSRF admin sem Origin %s: %d", p, rr.Status)
		}
	}
	// o token do usuário não vale no painel e vice-versa
	if rr := api(t, "GET", "/api/admin/denuncias", a.Token, nil); rr.Status != 401 {
		t.Errorf("token de usuário no /api/admin: %d", rr.Status)
	}
}

// ─── 6. Dinheiro: janela, retenção, anulação nos dois modos, débito, conferência ─

func TestQA_R3_PodioJanelaRetencaoELiberacao(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	srv := servidorMod(48)
	p := lancarPodioMod(t, srv, 3)
	s0 := saldo(t, p.us[0])
	premio := pendenteDe(t, p.id, p.us[0].ID)
	if premio <= 0 || s0 != 2000 {
		t.Fatalf("pódio: prêmio pendente %d, saldo %d", premio, s0)
	}
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência após pódio: %d → %d", div0, dv)
	}
	liberarPagamentos(t)
	if saldo(t, p.us[0]) != 2000 {
		t.Fatal("liberou antes da janela")
	}
	// denúncia de manipulação do pódio retém tudo
	rd := esperar(t, denunciar(t, p.us[1], corpoDenuncia(p.us[0].ID, models.DenunciaManipulacaoPodio, p.id)), 201)
	idDen := idDe(rd)
	for _, x := range pendentesDo(t, p.id) {
		if x.Status != models.PendenteRetido || x.IDDenunciaRetencao == nil || *x.IDDenunciaRetencao != idDen {
			t.Errorf("pendente %d não retido: %+v", x.ID, x)
		}
	}
	// o retido aparece ao premiado sem a denúncia
	pl := esperar(t, api(t, "GET", "/api/carteira/pendentes", p.us[0].Token, nil), 200)
	if num(pl.data(), "retido_centavos") != premio {
		t.Errorf("carteira/pendentes: %s", pl.Raw)
	}
	qaSemDenunciante(t, "pendentes do premiado", pl, p.us[1])
	db.Exec("UPDATE pagamentos_pendentes SET liberar_em = NOW(3) - INTERVAL 1 SECOND WHERE id_desafio = ?", p.id)
	liberarPagamentos(t)
	if saldo(t, p.us[0]) != 2000 {
		t.Fatal("retido foi liberado")
	}
	// segunda denúncia retentora: julgar a primeira repassa a retenção; julgar a segunda devolve a a_liberar
	rd2 := esperar(t, denunciar(t, p.us[2], corpoDenuncia(p.us[0].ID, models.DenunciaManipulacaoPodio, p.id)), 201)
	id2 := idDe(rd2)
	dec := func(id uint) {
		esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/assumir", id), sa, motivo("qa-teste")), 200)
		esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", id), sa, map[string]any{"status": "improcedente", "decisao_texto": "Sem provas de manipulação", "motivo": "qa-teste"}), 200)
	}
	dec(idDen)
	for _, x := range pendentesDo(t, p.id) {
		if x.Status != models.PendenteRetido || x.IDDenunciaRetencao == nil || *x.IDDenunciaRetencao != id2 {
			t.Errorf("retenção deveria passar à denúncia %d: %+v", id2, x)
		}
	}
	liberarPagamentos(t)
	if saldo(t, p.us[0]) != 2000 {
		t.Fatal("liberou com a 2ª denúncia aberta")
	}
	dec(id2)
	liberarPagamentos(t)
	if v := saldo(t, p.us[0]); v != 2000+premio {
		t.Errorf("depois de julgadas as denúncias e vencida a janela: saldo %d (esperava %d)", v, 2000+premio)
	}
	for _, x := range pendentesDo(t, p.id) {
		if x.Status != models.PendenteLiberado {
			t.Errorf("pendente %d: %s", x.ID, x.Status)
		}
	}
	// denúncia depois da liberação não retém nada nem desfaz
	esperar(t, denunciar(t, p.us[1], corpoDenuncia(p.us[0].ID, models.DenunciaFraudePresenca, p.id)), 201)
	if saldo(t, p.us[0]) != 2000+premio {
		t.Error("denúncia tardia mexeu no saldo")
	}
	ledgerConfere(t, todos(p)...)
	invariantes(t, p.id)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência final: %d → %d", div0, dv)
	}
}

func TestQA_R3_AnulacaoRelancarQ3Q4(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	comum := novoAdminDB(t, models.PapelAdmin)
	div0 := divergencia(t, sa)
	srv := servidorMod(48)
	p := lancarPodioMod(t, srv, 3)
	saldos := map[uint]int64{}
	for _, u := range todos(p) {
		saldos[u.ID] = saldo(t, u)
	}
	pts := pontos(t, p.us[0])
	if r := anular(t, comum, p.id, models.AnulacaoRelancarPodio, p.us[0].ID, 0); r.Status != 403 {
		t.Errorf("admin comum anulou: %d", r.Status)
	}
	esperar(t, anular(t, sa, p.id, "xpto", p.us[0].ID, 0), 400)
	ra := esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.us[0].ID, 0), 200).data()
	idPun := uint(num(ra, "id_punicao"))
	esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.us[0].ID, 0), 409) // 2ª anulação
	if statusDesafio(t, p.us[0], p.id) != "em_andamento" {
		t.Errorf("desafio deveria voltar a em_andamento")
	}
	for _, x := range pendentesDo(t, p.id) {
		if x.Status != models.PendenteCancelado {
			t.Errorf("pendente %d: %s", x.ID, x.Status)
		}
	}
	if pontos(t, p.us[0]) >= pts && pts > p.pontosAntes {
		t.Errorf("pontos do pódio não desfeitos: %d (antes do pódio %d)", pontos(t, p.us[0]), p.pontosAntes)
	}
	for _, u := range todos(p) {
		if s := saldo(t, u); s != saldos[u.ID] {
			t.Errorf("anulação sem liberação mexeu no saldo de %d: %d → %d", u.ID, saldos[u.ID], s)
		}
	}
	var pn models.Punicao
	db.First(&pn, idPun)
	if pn.Tipo != "anulacao_resultado" || pn.AplicadaPorAdmin == nil || *pn.AplicadaPorAdmin != sa.ID || pn.AplicadaPor != nil || pn.IDDesafio == nil {
		t.Errorf("punição de anulação: %+v", pn)
	}
	if contarAuditoria(t, "desafio_anular_resultado", "desafio", p.id) != 1 {
		t.Error("auditoria da anulação")
	}
	// Q4: revogar direto → 409; recurso deferido revoga só no histórico
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/punicoes/%d/revogar", idPun), sa, motivo("tentar")), 409)
	rr := esperar(t, qaRecurso(t, p.us[0], idPun), 201)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/recursos/%d/julgar", idDe(rr)), sa, map[string]any{"status": "deferido", "decisao_texto": "Deferido", "motivo": "qa-teste"}), 200)
	db.First(&pn, idPun)
	if pn.RevogadaEm == nil {
		t.Error("recurso deferido deveria revogar a anulação no histórico")
	}
	for _, x := range pendentesDo(t, p.id) {
		if x.Status != models.PendenteCancelado {
			t.Errorf("Q4: deferimento reverteu o financeiro (pendente %d %s)", x.ID, x.Status)
		}
	}
	if statusDesafio(t, p.us[0], p.id) != "em_andamento" {
		t.Error("Q4: deferimento mudou o desafio")
	}
	// Q3: só a academia dona relança; instrutor e outra academia não
	body := podioBody(entradaPodio(p.us[1], "09:00"), entradaPodio(p.us[2], "09:30"))
	if r := chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", p.id), p.c.I.Token, body); r.Status != 403 {
		t.Errorf("instrutor relançou: %d %s", r.Status, r.Raw)
	}
	outra, _ := novaAcademia(t)
	if r := chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", p.id), outra.Token, body); r.Status != 403 && r.Status != 404 {
		t.Errorf("outra academia relançou: %d", r.Status)
	}
	if r := chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", p.id), p.us[1].Token, body); r.Status != 403 && r.Status != 404 {
		t.Errorf("atleta relançou: %d", r.Status)
	}
	// expiração não cancela durante a carência contada da anulação
	cfgCar := *cfgBase
	cfgCar.DesafioCarencia = 48 * time.Hour
	_, tfs := app.ConstruirComJobs(&cfgCar, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
	for _, tf := range tfs {
		if tf.Nome == "expirar-desafios" {
			jobs.ExecutarAgora(tf)
		}
	}
	if statusDesafio(t, p.us[0], p.id) != "em_andamento" {
		t.Error("job de expiração cancelou desafio aguardando relançamento")
	}
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", p.id), p.c.A.Token, body), 200)
	if statusDesafio(t, p.us[0], p.id) != "encerrado" || pendenteDe(t, p.id, p.us[1].ID) <= 0 {
		t.Error("relançamento não gerou novos pendentes")
	}
	var nVivos int64
	db.Model(&models.PagamentoPendente{}).Where("id_desafio = ? AND status = 'a_liberar'", p.id).Count(&nVivos)
	if nVivos == 0 {
		t.Error("relançamento sem pendentes a_liberar")
	}
	db.Exec("UPDATE pagamentos_pendentes SET liberar_em = NOW(3) - INTERVAL 1 SECOND WHERE id_desafio = ? AND status = 'a_liberar'", p.id)
	liberarPagamentos(t)
	invariantes(t, p.id)
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência: %d → %d", div0, dv)
	}

	// relançar com pendente já liberado → 409 (só devolver_inscricoes)
	p2 := lancarPodioMod(t, servidorMod(0), 3)
	liberarPagamentos(t)
	esperar(t, anular(t, sa, p2.id, models.AnulacaoRelancarPodio, p2.us[0].ID, 0), 409)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência: %d → %d", div0, dv)
	}
}

func TestQA_R3_AnulacaoDevolverIntegralTaxaDebitoESaque(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	srv := servidorMod(0)
	p := lancarPodioMod(t, srv, 3)
	liberarPagamentos(t)
	taxa, _ := qaReceitasDesafio(t, p.id)
	premio := pendenteDe(t, p.id, p.us[0].ID)
	if taxa <= 0 {
		t.Fatalf("sem taxa liberada: %d", taxa)
	}
	var reserva int64
	db.Model(&models.Desafio{}).Select("reserva_academia_centavos").Where("id = ?", p.id).Scan(&reserva)
	// o 1º saca tudo
	tudo := saldo(t, p.us[0])
	esperar(t, sacar(t, p.us[0], tudo), 201)
	s2, s3 := saldo(t, p.us[1]), saldo(t, p.us[2])
	premio2 := pendenteDe(t, p.id, p.us[1].ID)
	acadAntes := saldo(t, p.c.A)
	// denúncia de origem (do desafio, contra o punido) vira procedente
	rd := esperar(t, denunciar(t, p.us[2], corpoDenuncia(p.us[0].ID, models.DenunciaManipulacaoPodio, p.id)), 201)
	ra := esperar(t, anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.us[0].ID, idDe(rd)), 200).data()
	if num(ra, "inscricoes_devolvidas_centavos") != 3000 || num(ra, "taxa_estornada_centavos") != taxa || num(ra, "reserva_devolvida_centavos") != reserva {
		t.Errorf("resposta da anulação: %v (taxa %d reserva %d)", ra, taxa, reserva)
	}
	var dn models.Denuncia
	db.First(&dn, idDe(rd))
	if dn.Status != "procedente" {
		t.Errorf("denúncia de origem: %s", dn.Status)
	}
	// entrada INTEGRAL (1000) a cada inscrito, inclusive com taxa já liberada
	for _, u := range p.us {
		var n, soma int64
		db.Model(&models.Transacao{}).Where("id_usuario = ? AND id_desafio = ? AND tipo = 'estorno_desafio'", u.ID, p.id).Count(&n)
		db.Model(&models.Transacao{}).Select("COALESCE(SUM(valor_centavos),0)").Where("id_usuario = ? AND id_desafio = ? AND tipo = 'estorno_desafio'", u.ID, p.id).Scan(&soma)
		if n != 1 || soma != 1000 {
			t.Errorf("devolução da entrada de %d: n=%d soma=%d", u.ID, n, soma)
		}
	}
	tx, est := qaReceitasDesafio(t, p.id)
	if est != -taxa || tx+est != 0 {
		t.Errorf("estorno_taxa: taxa %d estorno %d", tx, est)
	}
	// 2º lugar: +1000 de volta, −prêmio estornado (saldo cobria)
	if v := saldo(t, p.us[1]); v != s2+1000-premio2 || debitoAberto(t, p.us[1].ID) != 0 {
		t.Errorf("2º lugar: saldo %d (esperava %d), débito %d", v, s2+1000-premio2, debitoAberto(t, p.us[1].ID))
	}
	if v := saldo(t, p.us[2]); v != s3+1000 {
		t.Errorf("3º inscrito: saldo %d (esperava %d)", v, s3+1000)
	}
	// 1º lugar: tinha 0, recebeu 1000 e o estorno levou; resto do prêmio é débito
	deb := premio - 1000
	if v := saldo(t, p.us[0]); v != 0 || debitoAberto(t, p.us[0].ID) != deb {
		t.Errorf("1º lugar: saldo %d débito %d (esperava 0 e %d)", v, debitoAberto(t, p.us[0].ID), deb)
	}
	_ = acadAntes
	if statusDesafio(t, p.us[0], p.id) != "encerrado" {
		t.Error("devolver_inscricoes: desafio deveria ficar encerrado")
	}
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência após devolução com taxa liberada: %d → %d", div0, dv)
	}
	ledgerConfere(t, todos(p)...)

	// débito × saque: 8 saques simultâneos com saldo → todos 409 debito_aberto
	depositar(t, p.us[0], deb-1) // abate quase tudo; sobra 1 de débito
	if debitoAberto(t, p.us[0].ID) != 1 || saldo(t, p.us[0]) != 0 {
		t.Fatalf("abatimento por depósito: débito %d saldo %d", debitoAberto(t, p.us[0].ID), saldo(t, p.us[0]))
	}
	depositar(t, p.us[0], 3001) // quita 1, saldo 3000
	if debitoAberto(t, p.us[0].ID) != 0 || saldo(t, p.us[0]) != 3000 {
		t.Fatalf("quitação: débito %d saldo %d", debitoAberto(t, p.us[0].ID), saldo(t, p.us[0]))
	}
	// novo débito: devedor com saldo tenta sacar em paralelo
	q := lancarPodioModCom(t, srv, cenarioComSaldo(t, 20000), 3)
	liberarPagamentos(t)
	pq := pendenteDe(t, q.id, q.us[0].ID)
	esperar(t, sacar(t, q.us[0], saldo(t, q.us[0])), 201)
	esperar(t, anular(t, sa, q.id, models.AnulacaoDevolverInscricoes, q.us[0].ID, 0), 200)
	if debitoAberto(t, q.us[0].ID) != pq-1000 {
		t.Fatalf("débito do 2º cenário: %d", debitoAberto(t, q.us[0].ID))
	}
	db.Exec("UPDATE usuarios SET saldo_centavos = saldo_centavos WHERE id = ?", q.us[0].ID)
	// dá saldo sem quitar: crédito via depósito abate primeiro → para ter saldo com débito, usamos um débito maior que o depósito? não: depósito sempre abate.
	// Então só confere que, com débito aberto e saldo 0, N saques simultâneos dão 409 (débito antes do saldo).
	var wg sync.WaitGroup
	sts := make([]resp, 8)
	for i := range sts {
		wg.Add(1)
		go func(i int) { defer wg.Done(); sts[i] = sacar(t, q.us[0], 1000) }(i)
	}
	wg.Wait()
	for i, r := range sts {
		if r.Status != 409 || r.Body["codigo"] != "debito_aberto" {
			t.Errorf("saque %d com débito: %d %v", i, r.Status, r.Body)
		}
	}
	ledgerConfere(t, todos(q)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência final: %d → %d", div0, dv)
	}
	_ = time.Now
}

// ─── 7. Concorrência: anulações, recurso público, evidências ─────────────────

func TestQA_R3_ConcorrenciaAnulacaoRecursoEvidencias(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	p := lancarPodioMod(t, servidorMod(0), 3)
	liberarPagamentos(t)
	var wg sync.WaitGroup
	rs := make([]resp, 6)
	for i := range rs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rs[i] = anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.us[0].ID, 0)
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, r := range rs {
		switch r.Status {
		case 200:
			ok++
		case 409:
		default:
			t.Errorf("anulação concorrente: %d %s", r.Status, r.Raw)
		}
	}
	if ok != 1 {
		t.Errorf("anulações 200: %d", ok)
	}
	for _, u := range p.us {
		var n int64
		db.Model(&models.Transacao{}).Where("id_usuario = ? AND id_desafio = ? AND tipo = 'estorno_desafio'", u.ID, p.id).Count(&n)
		if n != 1 {
			t.Errorf("devolução duplicada para %d: %d", u.ID, n)
		}
	}
	var nPun int64
	db.Model(&models.Punicao{}).Where("id_desafio = ? AND tipo = 'anulacao_resultado'", p.id).Count(&nPun)
	if nPun != 1 || contarAuditoria(t, "desafio_anular_resultado", "desafio", p.id) != 1 {
		t.Errorf("punições/auditorias da anulação: %d", nPun)
	}
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("divergência: %d → %d", div0, dv)
	}

	// recurso público do banido: 6 envios simultâneos → 1 recurso
	u := novoUsuario(t)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), sa, motivo("fraude qa")), 200)
	st := make([]int, 6)
	for i := range st {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st[i] = api(t, "POST", "/api/recursos/banimento", "", map[string]any{"email": u.Email, "senha": u.Senha,
				"texto": fmt.Sprintf("Peço revisão do banimento, envio número %d.", i)}).Status
		}(i)
	}
	wg.Wait()
	var nRec int64
	db.Model(&models.Recurso{}).Joins("JOIN punicoes p ON p.id = recursos.id_punicao").Where("p.id_usuario = ?", u.ID).Count(&nRec)
	c201 := 0
	for _, s := range st {
		if s == 201 {
			c201++
		} else if s != 200 && s != 409 {
			t.Errorf("recurso concorrente: %v", st)
		}
	}
	if nRec != 1 || c201 != 1 {
		t.Errorf("recursos criados: %d (201s: %d, %v)", nRec, c201, st)
	}

	// evidências: 6 uploads simultâneos → exatamente 3, ordens 1..3
	a, b := novoUsuario(t), novoUsuario(t)
	idDen := idDe(esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaOutro, 0)), 201))
	ev := fmt.Sprintf("/api/denuncias/%d/evidencias", idDen)
	for i := range st {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st[i] = enviarArquivo(t, servidor, ev, a.Token, "e.png", pngMinimo(i+1)).Status
		}(i)
	}
	wg.Wait()
	var ords []int
	db.Model(&models.DenunciaEvidencia{}).Where("id_denuncia = ?", idDen).Order("ordem").Pluck("ordem", &ords)
	if len(ords) != 3 || ords[0] != 1 || ords[2] != 3 {
		t.Errorf("evidências concorrentes: ordens %v, status %v", ords, st)
	}
}
