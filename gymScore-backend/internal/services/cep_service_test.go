package services_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"gymScore-backend/internal/services"
)

func TestNormalizarCEP(t *testing.T) {
	for in, out := range map[string]string{
		"01310100": "01310100", "01310-100": "01310100", " 01310-100 ": "01310100",
		"0131010": "", "013101000": "", "0131-0100": "", "abcdefgh": "", "": "", "01310_100": "",
	} {
		if got := services.NormalizarCEP(in); got != out {
			t.Errorf("NormalizarCEP(%q) = %q, esperava %q", in, got, out)
		}
	}
}

func TestCEPService_CacheConcorrenteEErros(t *testing.T) {
	var chamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas.Add(1)
		switch r.URL.Path {
		case "/ws/01001000/json/":
			_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"Praça da Sé","bairro":"Sé","localidade":"São Paulo","uf":"sp"}`))
		case "/ws/22222222/json/":
			_, _ = w.Write([]byte(`{"cep":"x","uf":"ZZ"}`)) // UF inesperada = resposta não confiável
		default:
			_, _ = w.Write([]byte(`{"erro":true}`))
		}
	}))
	defer srv.Close()
	svc := services.NovoCEPService(srv.URL + "/ws/")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := svc.Buscar(context.Background(), "01001-000")
			if err != nil || r.UF != "SP" || r.Cidade != "São Paulo" || r.CEP != "01001000" {
				t.Errorf("busca: %+v %v", r, err)
			}
		}()
	}
	wg.Wait()
	if _, err := svc.Buscar(context.Background(), "01001000"); err != nil || chamadas.Load() > 20 {
		t.Errorf("cache: %v %d", err, chamadas.Load())
	}
	n := chamadas.Load()
	if _, err := svc.Buscar(context.Background(), "01001000"); err != nil || chamadas.Load() != n {
		t.Errorf("a consulta repetida deveria vir do cache")
	}
	if _, err := svc.Buscar(context.Background(), "33333333"); err == nil || statusDe(t, err) != http.StatusNotFound {
		t.Errorf("não encontrado: %v", err)
	}
	if _, err := svc.Buscar(context.Background(), "22222222"); err == nil || statusDe(t, err) != http.StatusServiceUnavailable {
		t.Errorf("resposta com UF inválida deveria ser indisponível: %v", err)
	}
	if _, err := svc.Buscar(context.Background(), "12"); err == nil || statusDe(t, err) != http.StatusBadRequest {
		t.Errorf("formato inválido: %v", err)
	}
	// Servidor fora do ar.
	srv.Close()
	if _, err := services.NovoCEPService(srv.URL).Buscar(context.Background(), "04538132"); err == nil ||
		statusDe(t, err) != http.StatusServiceUnavailable {
		t.Errorf("fora do ar: %v", err)
	}
}
