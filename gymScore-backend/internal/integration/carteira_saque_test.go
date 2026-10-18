//go:build integration

package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/models"
)

func sacar(t *testing.T, u *user, centavos int64) resp {
	t.Helper()
	return api(t, "POST", "/api/carteira/saque", u.Token, map[string]any{
		"valor_centavos": centavos, "chave_pix": "teste@gymscore.test", "tipo_chave": "EMAIL",
	})
}

func idDe(r resp) uint { return uint(r.data()["id"].(float64)) }

func consultarSaque(t *testing.T, u *user, id uint) resp {
	t.Helper()
	return api(t, "GET", fmt.Sprintf("/api/carteira/saque/%d", id), u.Token, nil)
}

func TestCarteira_SaldoEExtratoPaginado(t *testing.T) {
	u := usuarioComSaldo(t, 5000)
	depositar(t, u, 1000)
	depositar(t, u, 2000)
	esperar(t, api(t, "POST", "/api/desafios", u.Token, map[string]any{"titulo": "Teste extrato", "valor_entrada_centavos": 500}), 201)

	s := esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 200).data()
	if s["saldo_centavos"].(float64) != 7500 || s["saldo"].(float64) != 75 {
		t.Errorf("saldo inesperado: %v", s)
	}
	p1 := esperar(t, api(t, "GET", "/api/carteira/extrato?limite=2&pagina=1", u.Token, nil), 200).data()
	p2 := esperar(t, api(t, "GET", "/api/carteira/extrato?limite=2&pagina=2", u.Token, nil), 200).data()
	if p1["total"].(float64) != 4 || len(p1["itens"].([]any)) != 2 || len(p2["itens"].([]any)) != 2 {
		t.Fatalf("paginação errada: %v / %v", p1, p2)
	}
	// mais recente primeiro: a entrada do desafio (-500)
	primeiro := p1["itens"].([]any)[0].(map[string]any)
	if primeiro["tipo"] != "entrada_desafio" || primeiro["valor_centavos"].(float64) != -500 {
		t.Errorf("extrato deveria começar pela entrada de desafio: %v", primeiro)
	}
	// extrato é só do dono
	outro := novoUsuario(t)
	if e := esperar(t, api(t, "GET", "/api/carteira/extrato", outro.Token, nil), 200).data(); e["total"].(float64) != 0 {
		t.Errorf("extrato de outro usuário deveria estar vazio: %v", e)
	}
	esperar(t, api(t, "GET", "/api/carteira", "", nil), 401)
}

func TestSaque_SucessoDebitaEConcluiPorPolling(t *testing.T) {
	u := usuarioComSaldo(t, 5000)
	r := esperar(t, sacar(t, u, 2000), 201)
	id := idDe(r)
	trID := r.data()["asaas_transfer_id"].(string)
	if r.data()["status"] != "pending" || saldo(t, u) != 3000 {
		t.Fatalf("saque deveria estar pending com saldo 3000: %v saldo=%d", r.data(), saldo(t, u))
	}
	ledgerConfere(t, u)

	if c := esperar(t, consultarSaque(t, u, id), 200); c.data()["status"] != "pending" {
		t.Errorf("enquanto o Asaas processa continua pending: %v", c.data())
	}
	asaas.definirTransf(trID, "DONE")
	for i := 0; i < 3; i++ {
		if c := esperar(t, consultarSaque(t, u, id), 200); c.data()["status"] != "concluido" {
			t.Fatalf("deveria concluir: %v", c.data())
		}
	}
	if saldo(t, u) != 3000 {
		t.Errorf("saldo não pode mudar ao concluir: %d", saldo(t, u))
	}
	if n := contarNotif(t, u, models.NotifSaqueConcluido); n != 1 {
		t.Errorf("deveria haver exatamente 1 notificação de saque concluído, há %d", n)
	}
	ledgerConfere(t, u)
}

func TestSaque_FalhaNoAsaasReembolsaNaHora(t *testing.T) {
	u := usuarioComSaldo(t, 5000)
	asaas.modoFalhaTransf(true)
	defer asaas.modoFalhaTransf(false)

	r := sacar(t, u, 2000)
	if r.Status != 502 {
		t.Fatalf("esperava 502, obteve %d: %s", r.Status, r.Raw)
	}
	if saldo(t, u) != 5000 {
		t.Fatalf("o valor deveria ter voltado ao saldo: %d", saldo(t, u))
	}
	var st string
	db.Model(&models.Transacao{}).Select("status").Where("id_usuario = ? AND tipo = 'saque'", u.ID).Scan(&st)
	if st != "failed" {
		t.Errorf("o saque deveria estar failed, está %q", st)
	}
	if n := contarNotif(t, u, models.NotifSaqueFalhou); n != 1 {
		t.Errorf("deveria notificar a falha uma vez, notificou %d", n)
	}
	ledgerConfere(t, u)
}

func TestSaque_StatusFailedOuCancelledReembolsaUmaVez(t *testing.T) {
	for _, statusAsaas := range []string{"FAILED", "CANCELLED"} {
		u := usuarioComSaldo(t, 5000)
		r := esperar(t, sacar(t, u, 3000), 201)
		id, trID := idDe(r), r.data()["asaas_transfer_id"].(string)
		if saldo(t, u) != 2000 {
			t.Fatalf("saldo após saque deveria ser 2000")
		}
		asaas.definirTransf(trID, statusAsaas)

		// 10 consultas simultâneas: só uma reembolsa
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); consultarSaque(t, u, id) }()
		}
		wg.Wait()
		if s := saldo(t, u); s != 5000 {
			t.Fatalf("%s: reembolso único esperado (5000), obteve %d", statusAsaas, s)
		}
		if c := consultarSaque(t, u, id); c.data()["status"] != "failed" {
			t.Errorf("%s: status deveria ser failed: %v", statusAsaas, c.data())
		}
		if n := contarNotif(t, u, models.NotifSaqueFalhou); n != 1 {
			t.Errorf("%s: 1 notificação esperada, houve %d", statusAsaas, n)
		}
		// um saque failed nunca pode ser concluído depois
		asaas.definirTransf(trID, "DONE")
		consultarSaque(t, u, id)
		if saldo(t, u) != 5000 {
			t.Errorf("saque failed não pode voltar a debitar")
		}
		ledgerConfere(t, u)
	}
}

func TestSaque_ValidacoesESaldo(t *testing.T) {
	u := usuarioComSaldo(t, 1000)
	antes := asaas.criadas()
	corpo := func(v int64, chave, tipo string) map[string]any {
		return map[string]any{"valor_centavos": v, "chave_pix": chave, "tipo_chave": tipo}
	}
	esperar(t, api(t, "POST", "/api/carteira/saque", u.Token, corpo(499, "a@b.com", "EMAIL")), 400) // mínimo R$ 5,00
	esperar(t, api(t, "POST", "/api/carteira/saque", u.Token, corpo(500, "a@b.com", "XYZ")), 400)
	esperar(t, api(t, "POST", "/api/carteira/saque", u.Token, corpo(500, "", "EMAIL")), 400)
	esperar(t, api(t, "POST", "/api/carteira/saque", u.Token, corpo(1_000_001, "a@b.com", "EMAIL")), 400)
	esperar(t, api(t, "POST", "/api/carteira/saque", "", corpo(500, "a@b.com", "EMAIL")), 401)
	esperar(t, sacar(t, u, 1001), 422) // saldo insuficiente
	if asaas.criadas() != antes {
		t.Error("nenhuma chamada ao Asaas deveria ter sido feita")
	}
	var n int64
	db.Model(&models.Transacao{}).Where("id_usuario = ? AND tipo = 'saque'", u.ID).Count(&n)
	if n != 0 || saldo(t, u) != 1000 {
		t.Errorf("nada deveria ter sido gravado: linhas=%d saldo=%d", n, saldo(t, u))
	}
}

func TestSaque_SoODonoConsulta(t *testing.T) {
	a, b := usuarioComSaldo(t, 3000), novoUsuario(t)
	id := idDe(esperar(t, sacar(t, a, 1000), 201))
	esperar(t, consultarSaque(t, b, id), 404)
	esperar(t, consultarSaque(t, a, id), 200)
	esperar(t, consultarSaque(t, a, 99999999), 404)
	// um depósito (id de ledger de outro tipo) não é saque
	var dep uint
	db.Model(&models.Transacao{}).Select("id").Where("id_usuario = ? AND tipo = 'deposito'", a.ID).Scan(&dep)
	esperar(t, consultarSaque(t, a, dep), 404)
}

// 5 saques de R$ 5,00 em paralelo contra R$ 10,00: no máximo 2 passam; saldo nunca negativo.
func TestSaque_ConcorrenciaNaoGeraSaldoNegativo(t *testing.T) {
	u := usuarioComSaldo(t, 1000)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sacar(t, u, 500).Status == 201 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 2 || saldo(t, u) != 0 {
		t.Fatalf("esperava 2 saques e saldo 0, obteve %d e %d", ok, saldo(t, u))
	}
	ledgerConfere(t, u)
}

// Débito feito mas transferência nunca criada (processo caiu): após o prazo o polling reembolsa.
func TestSaque_PendenteOrfaoEhReembolsadoPeloPolling(t *testing.T) {
	u := usuarioComSaldo(t, 2000)
	id := idDe(esperar(t, sacar(t, u, 1500), 201))
	db.Exec("UPDATE transacoes SET asaas_transfer_id = NULL WHERE id = ?", id)

	// ainda dentro do prazo: segue pending
	if c := esperar(t, consultarSaque(t, u, id), 200); c.data()["status"] != "pending" {
		t.Fatalf("dentro do prazo deveria seguir pending: %v", c.data())
	}
	db.Exec("UPDATE transacoes SET created_at = ? WHERE id = ?", time.Now().Add(-10*time.Minute), id)
	if c := esperar(t, consultarSaque(t, u, id), 200); c.data()["status"] != "failed" {
		t.Fatalf("órfão antigo deveria ser reembolsado: %v", c.data())
	}
	if saldo(t, u) != 2000 {
		t.Errorf("saldo deveria voltar a 2000: %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}
