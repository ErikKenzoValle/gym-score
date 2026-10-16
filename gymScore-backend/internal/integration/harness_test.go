//go:build integration

// Package integration contém os testes de integração contra MySQL 8 real
// (build tag `integration`). Exigem TEST_DB_DSN apontando para um servidor
// DESCARTÁVEL (ex.: Docker local), sem nome de banco, por exemplo:
//
//	TEST_DB_DSN='root:senha@tcp(127.0.0.1:3307)/?parseTime=True&loc=Local' \
//	  go test -tags integration ./...
//
// O banco `gymscore_it` é recriado do zero a cada execução.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gymScore-backend/database"
	"gymScore-backend/internal/app"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const nomeBanco = "gymscore_it"

var uploadsDir = filepath.Join(os.TempDir(), fmt.Sprintf("gymscore_it_uploads_%d", os.Getpid()))

var (
	dsnAdmin        string // DSN sem banco (servidor de teste)
	db              *gorm.DB
	asaas           = &asaasFake{status: map[string]string{}}
	mailer          = &mailerFake{}
	servidor        *fiber.App // app padrão (webhook desativado, sem rate limit)
	servidorWebhook *fiber.App // mesma app com ASAAS_WEBHOOK_TOKEN definido
	cfgBase         *config.Config
)

// ─── Fakes ───────────────────────────────────────────────────────────────────

type asaasFake struct {
	mu     sync.Mutex
	n      int
	status map[string]string
	falhar bool

	// transferências (saque)
	transf        map[string]string // id -> status Asaas
	transfFalha   bool              // CriarTransferenciaPix devolve erro
	transfCriadas int
	transfN       int
}

func (a *asaasFake) CriarTransferenciaPix(valor float64, chave, tipo, desc string) (*models.AsaasTransferResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.transfFalha {
		return nil, fmt.Errorf("asaas: chave PIX inválida")
	}
	a.transfN++
	a.transfCriadas++
	id := fmt.Sprintf("tr_it_%d_%d", os.Getpid(), a.transfN)
	if a.transf == nil {
		a.transf = map[string]string{}
	}
	a.transf[id] = "PENDING"
	return &models.AsaasTransferResponse{ID: id, Status: "PENDING"}, nil
}
func (a *asaasFake) BuscarTransferencia(id string) (*models.AsaasTransferResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &models.AsaasTransferResponse{ID: id, Status: a.transf[id]}, nil
}
func (a *asaasFake) definirTransf(id, status string) {
	a.mu.Lock()
	a.transf[id] = status
	a.mu.Unlock()
}
func (a *asaasFake) modoFalhaTransf(v bool) {
	a.mu.Lock()
	a.transfFalha = v
	a.mu.Unlock()
}
func (a *asaasFake) criadas() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.transfCriadas
}

func (a *asaasFake) CriarCobrancaPix(_, _ string, valor float64, _ string) (*models.AsaasPaymentResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.n++
	id := fmt.Sprintf("pay_it_%d_%d", os.Getpid(), a.n)
	a.status[id] = "PENDING"
	return &models.AsaasPaymentResponse{ID: id, Value: valor, Status: "PENDING"}, nil
}
func (a *asaasFake) BuscarPixQrCode(string) (*models.AsaasPixQrCodeResponse, error) {
	return &models.AsaasPixQrCodeResponse{EncodedImage: "x", Payload: "p"}, nil
}
func (a *asaasFake) BuscarStatusPagamento(id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.falhar {
		return "", fmt.Errorf("asaas fora do ar")
	}
	return a.status[id], nil
}
func (a *asaasFake) SimularRecebimento(id string, _ float64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status[id] = "RECEIVED"
	return nil
}
func (a *asaasFake) pagar(id string) {
	a.mu.Lock()
	a.status[id] = "RECEIVED"
	a.mu.Unlock()
}

type mailerFake struct {
	mu     sync.Mutex
	envios []string // corpos dos e-mails
}

func (m *mailerFake) Enviar(para, _, corpo string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.envios = append(m.envios, para+"|"+corpo)
	return nil
}
func (m *mailerFake) ultimoPara(email string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.envios) - 1; i >= 0; i-- {
		if strings.HasPrefix(m.envios[i], email+"|") {
			return m.envios[i], true
		}
	}
	return "", false
}

// ─── Setup ───────────────────────────────────────────────────────────────────

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		log.Fatal("TEST_DB_DSN não definido: os testes de integração exigem um MySQL descartável (Docker local)")
	}
	dsnAdmin = dsn
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		log.Fatalf("conexão com o MySQL de teste: %v", err)
	}
	admin.Exec("DROP DATABASE IF EXISTS " + nomeBanco)
	if err := admin.Exec("CREATE DATABASE " + nomeBanco + " CHARACTER SET utf8mb4").Error; err != nil {
		log.Fatalf("criar banco: %v", err)
	}
	// DSN termina em "/?params": injeta o nome do banco
	dsnBanco := strings.Replace(dsn, "/?", "/"+nomeBanco+"?", 1)
	// igual à produção: sessão MySQL no mesmo fuso do Go (config.ConnectDB)
	dsnBanco = config.AplicarFusoDSN(dsnBanco, time.Local)
	db, err = gorm.Open(mysql.Open(dsnBanco), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		log.Fatalf("conexão com %s: %v", nomeBanco, err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(100)
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	cfgBase = &config.Config{
		AppEnv: "development", JWTSecret: "segredo-de-teste-integracao", AppBaseURL: "http://localhost:3000",
		AsaasEnv: "sandbox", FrontendPath: os.TempDir(), RateLimitMax: 0, UploadsDir: uploadsDir,
	}
	servidor = novoServidor(cfgBase)
	cfgWH := *cfgBase
	cfgWH.AsaasWebhookToken = "token-webhook-de-teste"
	servidorWebhook = novoServidor(&cfgWH)
	codigo := m.Run()
	_ = os.RemoveAll(uploadsDir)
	os.Exit(codigo)
}

func novoServidor(cfg *config.Config) *fiber.App {
	return app.Construir(cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
}

// ─── Helpers HTTP ────────────────────────────────────────────────────────────

type resp struct {
	Status int
	Body   map[string]any
	Raw    string
}

func (r resp) data() map[string]any {
	d, _ := r.Body["data"].(map[string]any)
	return d
}

func chamar(t *testing.T, srv *fiber.App, metodo, caminho, token string, corpo any) resp {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func api(t *testing.T, metodo, caminho, token string, corpo any) resp {
	return chamar(t, servidor, metodo, caminho, token, corpo)
}

func esperar(t *testing.T, r resp, status int) resp {
	t.Helper()
	if r.Status != status {
		t.Fatalf("esperava HTTP %d, obteve %d: %s", status, r.Status, r.Raw)
	}
	return r
}

// ─── Usuários de teste ───────────────────────────────────────────────────────

var seq atomic.Int64

// cpfValido gera um CPF válido (com dígitos verificadores) no formato 000.000.000-00.
func cpfValido() string {
	for {
		d := make([]int, 11)
		for i := 0; i < 9; i++ {
			d[i] = rand.Intn(10)
		}
		iguais := true
		for i := 1; i < 9; i++ {
			if d[i] != d[0] {
				iguais = false
			}
		}
		if iguais {
			continue
		}
		for j := 9; j < 11; j++ {
			soma := 0
			for i := 0; i < j; i++ {
				soma += d[i] * (j + 1 - i)
			}
			r := (soma * 10) % 11
			if r == 10 {
				r = 0
			}
			d[j] = r
		}
		return fmt.Sprintf("%d%d%d.%d%d%d.%d%d%d-%d%d", d[0], d[1], d[2], d[3], d[4], d[5], d[6], d[7], d[8], d[9], d[10])
	}
}

type user struct {
	ID    uint
	Email string
	Senha string
	Token string
	Nome  string
}

const senhaPadrao = "senha-forte-123"

func novoUsuario(t *testing.T) *user {
	t.Helper()
	n := seq.Add(1)
	u := &user{Email: fmt.Sprintf("qa+it%d_%d@gymscore.test", os.Getpid(), n), Senha: senhaPadrao, Nome: fmt.Sprintf("Teste%d", n)}
	esperar(t, api(t, "POST", "/api/usuarios", "", map[string]any{
		"nome": u.Nome, "sobrenome": "Integracao", "cpf": cpfValido(), "email": u.Email,
		"senha": u.Senha, "data_nascimento": "1995-03-15", "genero": "M",
	}), 201)
	login := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	u.Token = login.data()["token"].(string)
	u.ID = uint(login.data()["usuario"].(map[string]any)["id"].(float64))
	return u
}

// usuarioComSaldo cria o usuário e deposita via fluxo PIX real (ledger consistente).
func usuarioComSaldo(t *testing.T, centavos int64) *user {
	t.Helper()
	u := novoUsuario(t)
	depositar(t, u, centavos)
	return u
}

func depositar(t *testing.T, u *user, centavos int64) string {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{
		"valor_centavos": centavos, "cpf": cpfValido(),
	}), 200)
	id := r.data()["asaas_payment_id"].(string)
	asaas.pagar(id)
	esperar(t, api(t, "GET", "/api/pagamento/pix/"+id, u.Token, nil), 200)
	return id
}

func saldo(t *testing.T, u *user) int64 {
	t.Helper()
	r := esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 200)
	return int64(r.data()["saldo_centavos"].(float64))
}

// ledgerConfere verifica a invariante: soma do ledger efetivo == saldo.
func ledgerConfere(t *testing.T, us ...*user) {
	t.Helper()
	for _, u := range us {
		var soma int64
		db.Model(&models.Transacao{}).Select("COALESCE(SUM(valor_centavos),0)").
			Where("id_usuario = ? AND (status IN ? OR (tipo = 'saque' AND status = 'pending'))", u.ID, []string{"received", "concluido"}).Scan(&soma)
		if s := saldo(t, u); s != soma {
			t.Errorf("usuário %d: saldo %d != soma do ledger %d", u.ID, s, soma)
		}
	}
}

func amigos(t *testing.T, a, b *user) {
	t.Helper()
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
}

// chamarComHeader chama o webhook enviando o header asaas-access-token.
func chamarComHeader(t *testing.T, srv *fiber.App, caminho, tokenWebhook string, corpo any) resp {
	t.Helper()
	b, _ := json.Marshal(corpo)
	req := httptest.NewRequest(http.MethodPost, caminho, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if tokenWebhook != "" {
		req.Header.Set("asaas-access-token", tokenWebhook)
	}
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatalf("webhook: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func jsonBytes(v any) ([]byte, error) { return json.Marshal(v) }

// doReq faz uma requisição JSON sem token e devolve a resposta crua (para inspecionar headers).
func doReq(t *testing.T, srv *fiber.App, metodo, caminho string, corpo []byte) *http.Response {
	t.Helper()
	req := httptest.NewRequest(metodo, caminho, bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// novoServidorEm monta a aplicação sobre outro banco (ex.: banco migrado do schema antigo).
func novoServidorEm(d *gorm.DB) *fiber.App {
	return app.Construir(cfgBase, d, app.Dependencias{Asaas: asaas, Mailer: mailer})
}

// httpCookie é um atalho para montar cookies nos testes.
type httpCookie struct{ Name, Value string }

func (c httpCookie) c() *http.Cookie { return &http.Cookie{Name: c.Name, Value: c.Value} }
