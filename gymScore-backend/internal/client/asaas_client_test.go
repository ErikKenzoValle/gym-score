package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gymScore-backend/internal/config"
)

func clienteDeTeste(url string, timeout time.Duration) *AsaasClient {
	c := NewAsaasClient(&config.Config{AsaasBaseURL: url, AsaasAPIKey: "chave-teste"})
	c.httpClient.Timeout = timeout
	return c
}

func TestValorReais_SemFloat(t *testing.T) {
	casos := map[int64]string{0: "0.00", 5: "0.05", 100: "1.00", 2005: "20.05", 1_000_000: "10000.00", 1999_99: "1999.99", -150: "-1.50"}
	for c, esperado := range casos {
		if got := string(ValorReais(c)); got != esperado {
			t.Errorf("ValorReais(%d) = %s, esperado %s", c, got, esperado)
		}
	}
}

// O POST /v3/transfers leva o valor decimal exato e a externalReference do saque.
func TestCriarTransferenciaPix_CorpoComExternalReference(t *testing.T) {
	var corpo map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v3/transfers" || r.Header.Get("access_token") != "chave-teste" {
			t.Errorf("requisição inesperada: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &corpo)
		_, _ = w.Write([]byte(`{"id":"tr_1","status":"PENDING"}`))
	}))
	defer srv.Close()

	tr, err := clienteDeTeste(srv.URL, time.Second).CriarTransferenciaPix(2005, "a@b.com", "EMAIL", "Saque #7", "saque-7")
	if err != nil || tr.ID != "tr_1" {
		t.Fatalf("esperava sucesso: %v %v", tr, err)
	}
	if string(corpo["value"]) != "20.05" {
		t.Errorf("value deveria ser o decimal exato 20.05, veio %s", corpo["value"])
	}
	if string(corpo["externalReference"]) != `"saque-7"` {
		t.Errorf("externalReference ausente/errada: %s", corpo["externalReference"])
	}
}

// Só 4xx de validação é definitivo; timeout, 5xx, 429 e respostas 2xx ilegíveis são ambíguos.
func TestCriarTransferenciaPix_ClassificacaoDeErros(t *testing.T) {
	casos := []struct {
		nome       string
		status     int
		corpo      string
		demora     time.Duration
		definitivo bool
	}{
		{"400 validação", 400, `{"errors":[{"code":"invalid_pixAddressKey"}]}`, 0, true},
		{"401 chave", 401, `{}`, 0, true},
		{"422", 422, `{}`, 0, true},
		{"500", 500, `erro`, 0, false},
		{"502 gateway", 502, `bad gateway`, 0, false},
		{"504", 504, ``, 0, false},
		{"429", 429, `{}`, 0, false},
		{"408", 408, `{}`, 0, false},
		{"409", 409, `{}`, 0, false},
		{"200 ilegível", 200, `<html>`, 0, false},
		{"200 sem id", 200, `{"status":"PENDING"}`, 0, false},
		{"timeout", 200, `{"id":"tr_tarde","status":"PENDING"}`, 300 * time.Millisecond, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(c.demora)
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.corpo))
			}))
			defer srv.Close()
			_, err := clienteDeTeste(srv.URL, 100*time.Millisecond).CriarTransferenciaPix(1000, "a@b.com", "EMAIL", "x", "saque-1")
			if err == nil {
				t.Fatal("esperava erro")
			}
			if got := ErroDefinitivo(err); got != c.definitivo {
				t.Errorf("ErroDefinitivo = %v, esperado %v (%v)", got, c.definitivo, err)
			}
		})
	}
}

func TestCriarTransferenciaPix_ConexaoRecusadaEhAmbigua(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // ninguém escutando
	_, err := clienteDeTeste(url, time.Second).CriarTransferenciaPix(1000, "a@b.com", "EMAIL", "x", "saque-1")
	if err == nil || ErroDefinitivo(err) {
		t.Fatalf("erro de rede deve ser ambíguo: %v", err)
	}
}

func TestCriarCobrancaPix_ValorExatoEReferencia(t *testing.T) {
	var corpo map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v3/customers":
			_, _ = w.Write([]byte(`{"data":[{"id":"cus_1"}]}`))
		case r.Method == "POST" && r.URL.Path == "/v3/payments":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &corpo)
			_, _ = w.Write([]byte(`{"id":"pay_1","status":"PENDING"}`))
		default:
			t.Errorf("rota inesperada %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	if _, err := clienteDeTeste(srv.URL, time.Second).CriarCobrancaPix("A B", "123.456.789-09", 1010, "dep", "deposito-9"); err != nil {
		t.Fatal(err)
	}
	if string(corpo["value"]) != "10.10" || string(corpo["externalReference"]) != `"deposito-9"` {
		t.Errorf("corpo inesperado: value=%s ref=%s", corpo["value"], corpo["externalReference"])
	}
}
