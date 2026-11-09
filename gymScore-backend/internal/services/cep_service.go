package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gymScore-backend/internal/models"
)

// CEPService consulta o endereço de um CEP (proxy do ViaCEP) para o preenchimento automático do
// endereço da academia. É só uma ajuda: o cadastro/edição nunca dependem dele (campos editáveis).
type CEPService interface {
	Buscar(ctx context.Context, cep string) (*models.CEPResponse, error)
}

// Configuração do proxy de CEP.
const (
	// ViaCEPURLPadrao é a base da API do ViaCEP (VIACEP_URL sobrescreve; testes usam httptest).
	ViaCEPURLPadrao = "https://viacep.com.br/ws"
	// cepTimeout: o usuário está esperando no formulário; passou disso, ele digita à mão.
	cepTimeout = 3 * time.Second
	// TTLs do cache em memória: CEP encontrado muda raramente; o não encontrado expira antes.
	cepTTLEncontrado    = 24 * time.Hour
	cepTTLNaoEncontrado = time.Hour
	// cepCacheMax limita a memória do cache (acima disso, limpa os expirados e, se preciso, tudo).
	cepCacheMax = 10000
	// cepCorpoMax limita a leitura da resposta do ViaCEP.
	cepCorpoMax = 64 * 1024
	// CodigoCEPIndisponivel: o ViaCEP não respondeu (timeout/erro); o front libera o preenchimento manual.
	CodigoCEPIndisponivel = "cep_indisponivel"
)

type cepCacheItem struct {
	resp   *models.CEPResponse // nil = CEP não encontrado
	expira time.Time
}

type cepService struct {
	base   string
	client *http.Client
	agora  func() time.Time

	mu    sync.Mutex
	cache map[string]cepCacheItem
}

// NovoCEPService cria o proxy do ViaCEP com timeout curto e cache em memória com TTL.
func NovoCEPService(baseURL string) CEPService {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = ViaCEPURLPadrao
	}
	return &cepService{base: base, client: &http.Client{Timeout: cepTimeout}, agora: time.Now,
		cache: map[string]cepCacheItem{}}
}

var (
	erroCEPNaoEncontrado = erro(http.StatusNotFound, "CEP não encontrado. Confira o número ou preencha o endereço manualmente")
)

func erroCEPIndisponivel() *ErroNegocio {
	e := erro(http.StatusServiceUnavailable, "Não foi possível consultar o CEP agora. Preencha o endereço manualmente")
	e.Codigo = CodigoCEPIndisponivel
	return e
}

// Buscar devolve o endereço do CEP (400 formato inválido, 404 não encontrado, 503 cep_indisponivel).
func (s *cepService) Buscar(ctx context.Context, cep string) (*models.CEPResponse, error) {
	cep = NormalizarCEP(cep)
	if cep == "" {
		return nil, erroRequisicao("CEP inválido: informe 8 dígitos (ex.: 01310-100)")
	}
	if item, ok := s.doCache(cep); ok {
		if item.resp == nil {
			return nil, erroCEPNaoEncontrado
		}
		r := *item.resp
		return &r, nil
	}
	resp, encontrado, err := s.consultar(ctx, cep)
	if err != nil {
		return nil, erroCEPIndisponivel()
	}
	if !encontrado {
		s.guardar(cep, nil, cepTTLNaoEncontrado)
		return nil, erroCEPNaoEncontrado
	}
	s.guardar(cep, resp, cepTTLEncontrado)
	r := *resp
	return &r, nil
}

func (s *cepService) doCache(cep string) (cepCacheItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.cache[cep]
	if !ok || !s.agora().Before(item.expira) {
		return cepCacheItem{}, false
	}
	return item, true
}

func (s *cepService) guardar(cep string, resp *models.CEPResponse, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agora := s.agora()
	if len(s.cache) >= cepCacheMax {
		for k, v := range s.cache {
			if !agora.Before(v.expira) {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= cepCacheMax {
			s.cache = map[string]cepCacheItem{}
		}
	}
	s.cache[cep] = cepCacheItem{resp: resp, expira: agora.Add(ttl)}
}

// viaCEPResposta é o JSON do ViaCEP; `erro` vem como true (ou "true") para CEP inexistente.
type viaCEPResposta struct {
	CEP        string `json:"cep"`
	Logradouro string `json:"logradouro"`
	Bairro     string `json:"bairro"`
	Localidade string `json:"localidade"`
	UF         string `json:"uf"`
	Erro       any    `json:"erro"`
}

// consultar chama o ViaCEP. encontrado=false para CEP inexistente (200 com `erro`, 400 ou 404); err para
// indisponibilidade (timeout, rede, 5xx, JSON inválido).
func (s *cepService) consultar(ctx context.Context, cep string) (*models.CEPResponse, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, cepTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/"+cep+"/json/", nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusNotFound:
		return nil, false, nil
	case res.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("viacep: HTTP %d", res.StatusCode)
	}
	var v viaCEPResposta
	if err := json.NewDecoder(io.LimitReader(res.Body, cepCorpoMax)).Decode(&v); err != nil {
		return nil, false, fmt.Errorf("viacep: resposta inválida: %w", err)
	}
	if e, ok := v.Erro.(bool); ok && e {
		return nil, false, nil
	}
	if e, ok := v.Erro.(string); ok && e == "true" {
		return nil, false, nil
	}
	uf := strings.ToUpper(strings.TrimSpace(v.UF))
	if !ufsValidas[uf] {
		return nil, false, fmt.Errorf("viacep: UF inesperada %q", v.UF)
	}
	return &models.CEPResponse{CEP: cep, Logradouro: cortar(limpar(v.Logradouro), 150), Bairro: cortar(limpar(v.Bairro), 100),
		Cidade: cortar(limpar(v.Localidade), 100), UF: uf}, true, nil
}

// cortar limita s a max caracteres (os dados externos nunca passam do tamanho das colunas).
func cortar(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
