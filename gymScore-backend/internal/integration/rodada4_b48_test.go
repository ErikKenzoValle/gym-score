//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
)

// desafioDoFeed procura o desafio no feed filtrado pelo título (mesmo montarResposta do detalhe).
func desafioDoFeed(t *testing.T, u *user, titulo string, id uint) map[string]any {
	t.Helper()
	r := esperar(t, api(t, "GET", "/api/desafios?limite=100&q="+strings.ReplaceAll(titulo, " ", "%20"), u.Token, nil), 200)
	for _, it := range itensDe(t, r) {
		if uint(it["id"].(float64)) == id {
			return it
		}
	}
	t.Fatalf("desafio %d fora do feed", id)
	return nil
}

func TestB48_PodeEntrarConsideraConflitoDeInteresse(t *testing.T) {
	c := novoCenario(t) // c.I vinculado (aprovado) à academia de c
	corpo := corpoDesafio(c, 0, 5, 1)
	tituloA := fmt.Sprintf("Conflito B48 zz%d", tituloSeq.Add(1))
	corpo["titulo"] = tituloA
	idA := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpo), 201))
	b := novoCenario(t)
	idB := criarDesafio(t, b, 0, 5)
	modoAtleta(t, c.I)

	confere := func(u *user, id uint, pode bool, motivo string) {
		t.Helper()
		d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200).data()
		m, _ := d["motivo_bloqueio_entrada"].(string)
		if d["pode_entrar"] != pode || !strings.Contains(m, motivo) || (motivo == "" && m != "") {
			t.Errorf("desafio %d: pode_entrar=%v motivo=%q (esperava %v %q)", id, d["pode_entrar"], m, pode, motivo)
		}
		// e a previsão confere com o POST /entrar (409 com a MESMA mensagem)
		if !pode && motivo != "" {
			r := esperar(t, acao(t, u, id, "entrar", nil), 409)
			if !strings.Contains(r.Raw, m) {
				t.Errorf("409 com outra mensagem: %s", r.Raw)
			}
		}
	}
	// c.I é o designado de idA; v é outro instrutor aprovado vinculado à mesma academia
	confere(c.I, idA, false, "equipe de instrutores")
	v, _ := novoInstrutorDe(t, c.AcademiaID)
	modoAtleta(t, v)
	confere(v, idA, false, "vinculado à academia")
	if it := desafioDoFeed(t, v, tituloA, idA); it["pode_entrar"] != false || it["motivo_bloqueio_entrada"] == nil {
		t.Errorf("feed sem o bloqueio: %v", it)
	}
	// outra academia: pode entrar, sem motivo
	confere(c.I, idB, true, "")
	// atleta comum: sem motivo
	at := novoUsuario(t)
	confere(at, idA, true, "")

	// designado (vaga aberta escolhida) e candidatura pendente
	auto, _ := instrutorAprovado(t, 0)
	idV := desafioVagaAberta(t, b)
	modoInstrutor(t, auto)
	cid := uint(esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", idV), auto.Token,
		map[string]any{"lance_centavos": 0}), 201).data()["id"].(float64))
	modoAtleta(t, auto)
	confere(auto, idV, false, "candidatou")
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", idV, cid), b.A.Token, nil), 200)
	confere(auto, idV, false, "equipe de instrutores")

	// pedido PENDENTE vinculado não bloqueia (regra: só o designado bloqueia o pendente)
	pend := novoUsuario(t)
	db.Exec("INSERT INTO instrutores (id_usuario, cref, status, id_academia) VALUES (?, '000123-G/SP', 'pendente', ?)", pend.ID, c.AcademiaID)
	confere(pend, idA, true, "")

	// modo instrutor: pode_entrar false e sem motivo de conflito (o motivo é o modo)
	modoInstrutor(t, c.I)
	d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", idA), c.I.Token, nil), 200).data()
	if d["pode_entrar"] != false || d["motivo_bloqueio_entrada"] != nil {
		t.Errorf("modo instrutor: %v %v", d["pode_entrar"], d["motivo_bloqueio_entrada"])
	}
}

func TestB48_AdminAcademiasTrazEnderecoEstruturado(t *testing.T) {
	_, aid := novaAcademia(t)
	db.Exec("UPDATE academias SET status = 'pendente' WHERE id = ?", aid)
	a := novoAdminDB(t, "admin")
	var achou map[string]any
	for pag := 1; pag <= 50 && achou == nil; pag++ {
		r := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/academias?status=pendente&limite=100&pagina=%d", pag), a, nil), 200)
		itens := itensDe(t, r)
		for _, it := range itens {
			if uint(it["id"].(float64)) == aid {
				achou = it
			}
		}
		if len(itens) < 100 {
			break
		}
	}
	if achou == nil {
		t.Fatal("academia pendente fora da lista do admin")
	}
	for _, k := range []string{"endereco", "cep", "logradouro", "numero", "bairro", "cidade", "uf"} {
		if s, _ := achou[k].(string); s == "" {
			t.Errorf("lista do admin sem %q: %v", k, achou)
		}
	}
	if achou["cep"] == nil || len(achou["cep"].(string)) != 8 {
		t.Errorf("cep: %v", achou["cep"])
	}
	// aprovar devolve o mesmo DTO com endereço
	r := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/academias/%d/aprovar", aid), a, motivo("endereço conferido")), 200).data()
	if r["uf"] == nil || r["cidade"] == nil {
		t.Errorf("aprovar sem endereço: %v", r)
	}
	// academia antiga (só texto): estruturados omitidos, texto presente
	_, antiga := novaAcademia(t)
	db.Exec("UPDATE academias SET cep=NULL, logradouro=NULL, numero=NULL, complemento=NULL, bairro=NULL, cidade=NULL, uf=NULL, endereco='Rua Velha, 1', status='pendente' WHERE id = ?", antiga)
	r = esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/academias/%d/rejeitar", antiga), a, motivo("teste de academia antiga")), 200).data()
	if r["endereco"] != "Rua Velha, 1" || r["cep"] != nil {
		t.Errorf("academia antiga: %v", r)
	}
}
