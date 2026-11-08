//go:build integration

package integration

import (
	"sync"
	"sync/atomic"
	"testing"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// ─── Ledger (repositório) ────────────────────────────────────────────────────

func TestLedger_DebitoInsuficienteNaoAlteraSaldoNemGravaLinha(t *testing.T) {
	u := usuarioComSaldo(t, 1000)
	repo := repositories.NovoCarteiraRepository(db)
	err := repo.Transacao(func(tx *gorm.DB) error {
		_, err := repo.Debitar(tx, repositories.Lancamento{IDUsuario: u.ID, Tipo: models.TipoEntradaDesafio, ValorCentavos: 1001})
		return err
	})
	if err != repositories.ErrSaldoInsuficiente {
		t.Fatalf("esperava ErrSaldoInsuficiente, obteve %v", err)
	}
	var linhas int64
	db.Model(&models.Transacao{}).Where("id_usuario = ? AND tipo = ?", u.ID, models.TipoEntradaDesafio).Count(&linhas)
	if linhas != 0 || saldo(t, u) != 1000 {
		t.Errorf("nada deveria mudar: linhas=%d saldo=%d", linhas, saldo(t, u))
	}
}

func TestLedger_ValoresInvalidosRejeitados(t *testing.T) {
	u := novoUsuario(t)
	repo := repositories.NovoCarteiraRepository(db)
	for _, v := range []int64{0, -5} {
		if _, err := repo.Creditar(db, repositories.Lancamento{IDUsuario: u.ID, Tipo: models.TipoPremioDesafio, ValorCentavos: v}); err == nil {
			t.Errorf("crédito de %d deveria falhar", v)
		}
		if _, err := repo.Debitar(db, repositories.Lancamento{IDUsuario: u.ID, Tipo: models.TipoEntradaDesafio, ValorCentavos: v}); err == nil {
			t.Errorf("débito de %d deveria falhar", v)
		}
	}
}

// 20 débitos paralelos de 100 contra saldo de 1000: exatamente 10 passam.
func TestLedger_ConcorrenciaDebitosNaoGeramSaldoNegativo(t *testing.T) {
	u := usuarioComSaldo(t, 1000)
	repo := repositories.NovoCarteiraRepository(db)
	var ok, recusados atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.Transacao(func(tx *gorm.DB) error {
				_, err := repo.Debitar(tx, repositories.Lancamento{IDUsuario: u.ID, Tipo: models.TipoEntradaDesafio, ValorCentavos: 100})
				return err
			})
			switch err {
			case nil:
				ok.Add(1)
			case repositories.ErrSaldoInsuficiente:
				recusados.Add(1)
			default:
				t.Errorf("erro inesperado: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 10 || recusados.Load() != 10 {
		t.Fatalf("esperava 10 débitos e 10 recusas, obteve %d e %d", ok.Load(), recusados.Load())
	}
	if saldo(t, u) != 0 {
		t.Errorf("saldo final deveria ser 0, obteve %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}

// Débito + crédito intercalados em paralelo: a soma do ledger sempre fecha com o saldo.
func TestLedger_ConcorrenciaMisturadaMantemInvariante(t *testing.T) {
	u := usuarioComSaldo(t, 5000)
	repo := repositories.NovoCarteiraRepository(db)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = repo.Transacao(func(tx *gorm.DB) error {
				if i%2 == 0 {
					_, err := repo.Creditar(tx, repositories.Lancamento{IDUsuario: u.ID, Tipo: models.TipoPremioDesafio, ValorCentavos: 70})
					return err
				}
				_, err := repo.Debitar(tx, repositories.Lancamento{IDUsuario: u.ID, Tipo: models.TipoEntradaDesafio, ValorCentavos: 130})
				return err
			})
		}(i)
	}
	wg.Wait()
	ledgerConfere(t, u)
}

// ─── PIX ─────────────────────────────────────────────────────────────────────

func TestPIX_ValidacaoDeEntrada(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 100, "cpf": cpfValido()}), 400)
	esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 1000, "cpf": "111.111.111-11"}), 400)
	esperar(t, api(t, "POST", "/api/pagamento/pix", "", map[string]any{"valor_centavos": 1000, "cpf": cpfValido()}), 401)
}

func TestPIX_DepositoCreditaUmaUnicaVez(t *testing.T) {
	u := novoUsuario(t)
	id := depositar(t, u, 2500)
	for i := 0; i < 3; i++ {
		r := esperar(t, api(t, "GET", "/api/pagamento/pix/"+id, u.Token, nil), 200)
		if r.data()["status"] != "received" {
			t.Fatalf("status deveria ser received: %v", r.data())
		}
	}
	if saldo(t, u) != 2500 {
		t.Errorf("saldo deveria ser 2500, obteve %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}

func TestPIX_ConsultasParalelasCreditamUmaVez(t *testing.T) {
	u := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 4000, "cpf": cpfValido()}), 200)
	id := r.data()["asaas_payment_id"].(string)
	asaas.pagar(id)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); api(t, "GET", "/api/pagamento/pix/"+id, u.Token, nil) }()
	}
	wg.Wait()
	if saldo(t, u) != 4000 {
		t.Fatalf("saldo deveria ser 4000 (crédito único), obteve %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}

func TestPIX_SoODonoConsultaESimula(t *testing.T) {
	dono := novoUsuario(t)
	outro := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", dono.Token, map[string]any{"valor_centavos": 1000, "cpf": cpfValido()}), 200)
	id := r.data()["asaas_payment_id"].(string)

	esperar(t, api(t, "GET", "/api/pagamento/pix/"+id, outro.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/pagamento/pix/"+id+"/simular", outro.Token, nil), 404)
	if saldo(t, outro) != 0 || saldo(t, dono) != 0 {
		t.Error("nenhum saldo pode mudar por tentativa de outro usuário")
	}
	// o dono simula e recebe
	esperar(t, api(t, "POST", "/api/pagamento/pix/"+id+"/simular", dono.Token, nil), 200)
	if saldo(t, dono) != 1000 {
		t.Errorf("dono deveria ter 1000, obteve %d", saldo(t, dono))
	}
}

func TestPIX_FalhaDoAsaasNoPollingNaoCredita(t *testing.T) {
	u := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 1000, "cpf": cpfValido()}), 200)
	id := r.data()["asaas_payment_id"].(string)
	asaas.pagar(id)
	asaas.mu.Lock()
	asaas.falhar = true
	asaas.mu.Unlock()
	defer func() { asaas.mu.Lock(); asaas.falhar = false; asaas.mu.Unlock() }()
	c := esperar(t, api(t, "GET", "/api/pagamento/pix/"+id, u.Token, nil), 200)
	if c.data()["status"] != "pending" || saldo(t, u) != 0 {
		t.Errorf("Asaas fora do ar: deve continuar pending sem crédito (%v)", c.data())
	}
}

// ─── Webhook ─────────────────────────────────────────────────────────────────

func webhook(t *testing.T, token, evento, paymentID string) resp {
	t.Helper()
	return chamarComHeader(t, servidorWebhook, "/api/webhooks/asaas", token,
		map[string]any{"event": evento, "payment": map[string]any{"id": paymentID}})
}

func TestWebhook_DesativadoSemToken(t *testing.T) {
	// servidor padrão tem ASAAS_WEBHOOK_TOKEN vazio
	r := chamarComHeader(t, servidor, "/api/webhooks/asaas", "qualquer", map[string]any{"event": "PAYMENT_RECEIVED", "payment": map[string]any{"id": "x"}})
	if r.Status != 404 {
		t.Fatalf("webhook sem token configurado deve responder 404, obteve %d", r.Status)
	}
}

func TestWebhook_TokenErradoOuAusente(t *testing.T) {
	u := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 1000, "cpf": cpfValido()}), 200)
	id := r.data()["asaas_payment_id"].(string)

	esperar(t, webhook(t, "", "PAYMENT_RECEIVED", id), 401)
	esperar(t, webhook(t, "errado", "PAYMENT_RECEIVED", id), 401)
	if saldo(t, u) != 0 {
		t.Error("webhook sem token válido não pode creditar")
	}
}

func TestWebhook_RecebidoEhIdempotenteEEstornoDebita(t *testing.T) {
	u := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 3000, "cpf": cpfValido()}), 200)
	id := r.data()["asaas_payment_id"].(string)

	// evento duplicado + confirmado + polling: um único crédito
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_RECEIVED", id), 200)
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_RECEIVED", id), 200)
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_CONFIRMED", id), 200)
	asaas.pagar(id)
	esperar(t, api(t, "GET", "/api/pagamento/pix/"+id, u.Token, nil), 200)
	if saldo(t, u) != 3000 {
		t.Fatalf("crédito único esperado (3000), obteve %d", saldo(t, u))
	}

	// estorno: debita e não repete
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_REFUNDED", id), 200)
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_REFUNDED", id), 200)
	if saldo(t, u) != 0 {
		t.Errorf("estorno deveria zerar o saldo, obteve %d", saldo(t, u))
	}
	ledgerConfere(t, u)

	// evento para pagamento desconhecido não quebra
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_RECEIVED", "pay_inexistente"), 200)
}

func TestWebhook_EstornoComSaldoInsuficienteNaoDeixaNegativo(t *testing.T) {
	u := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 3000, "cpf": cpfValido()}), 200)
	id := r.data()["asaas_payment_id"].(string)
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_RECEIVED", id), 200)

	// o usuário gasta quase tudo numa entrada de desafio
	esperar(t, acao(t, u, criarDesafio(t, novoCenario(t), 2500, 4), "entrar", nil), 200)
	if saldo(t, u) != 500 {
		t.Fatalf("saldo esperado 500, obteve %d", saldo(t, u))
	}
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_REFUNDED", id), 200)
	if saldo(t, u) != 500 {
		t.Errorf("saldo não pode mudar nem ficar negativo: %d", saldo(t, u))
	}
	var status string
	db.Model(&models.Transacao{}).Select("status").Where("asaas_payment_id = ?", id).Scan(&status)
	if status != "received" {
		t.Errorf("sem débito possível o depósito permanece 'received' (consistente com o saldo), obteve %s", status)
	}
	ledgerConfere(t, u)
}
