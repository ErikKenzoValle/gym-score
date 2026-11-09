package services

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"gymScore-backend/internal/models"
)

// ufsValidas são as 27 UFs aceitas (mesma lista do CHECK chk_academias_uf da migration 00020).
var ufsValidas = map[string]bool{
	"AC": true, "AL": true, "AP": true, "AM": true, "BA": true, "CE": true, "DF": true, "ES": true, "GO": true,
	"MA": true, "MT": true, "MS": true, "MG": true, "PA": true, "PB": true, "PR": true, "PE": true, "PI": true,
	"RJ": true, "RN": true, "RS": true, "RO": true, "RR": true, "SC": true, "SP": true, "SE": true, "TO": true,
}

// enderecoMax é o tamanho de academias.endereco (VARCHAR(500), migration 00020).
const enderecoMax = 500

// ErroEnderecoInvalido é o 422 do endereço recusado pelo banco (CHECK de CEP/UF, erro 3819) — a
// validação do service já barra antes; isto cobre o que escapar dela.
var ErroEnderecoInvalido = erro(http.StatusUnprocessableEntity,
	"Endereço inválido: o CEP deve ter 8 dígitos e a UF deve ser uma das 27 siglas (ex.: SP)")

// NormalizarCEP tira a máscara ("01310-100" → "01310100") e confere os 8 dígitos ("" se inválido).
func NormalizarCEP(cep string) string {
	cep = strings.TrimSpace(cep)
	if len(cep) == 9 && cep[5] == '-' {
		cep = cep[:5] + cep[6:]
	}
	if len(cep) != 8 {
		return ""
	}
	for _, r := range cep {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return cep
}

// enderecoNormalizado é o endereço validado, pronto para gravar.
type enderecoNormalizado struct {
	CEP, Logradouro, Numero, Complemento, Bairro, Cidade, UF string
}

// normalizarEndereco valida e normaliza o endereço estruturado (400 com mensagem clara se inválido).
func normalizarEndereco(r *models.EnderecoAcademiaRequest) (*enderecoNormalizado, error) {
	e := &enderecoNormalizado{
		CEP: NormalizarCEP(r.CEP), Logradouro: limpar(r.Logradouro), Numero: limpar(r.Numero),
		Complemento: limpar(r.Complemento), Bairro: limpar(r.Bairro), Cidade: limpar(r.Cidade),
		UF: strings.ToUpper(strings.TrimSpace(r.UF)),
	}
	switch {
	case e.CEP == "":
		return nil, erroRequisicao("CEP inválido: informe 8 dígitos (ex.: 01310-100)")
	case !ufsValidas[e.UF]:
		return nil, erroRequisicao("UF inválida: use a sigla do estado (ex.: SP)")
	case e.Numero == "":
		return nil, erroRequisicao("Informe o número do endereço (ou S/N)")
	case e.Logradouro == "":
		return nil, erroRequisicao("Informe o logradouro (rua, avenida...)")
	case e.Bairro == "":
		return nil, erroRequisicao("Informe o bairro")
	case e.Cidade == "":
		return nil, erroRequisicao("Informe a cidade")
	}
	for campo, max := range map[*string]int{&e.Logradouro: 150, &e.Numero: 20, &e.Complemento: 100, &e.Bairro: 100, &e.Cidade: 100} {
		if utf8.RuneCountInString(*campo) > max {
			return nil, erroRequisicao("Endereço com campo longo demais")
		}
	}
	return e, nil
}

// limpar tira espaços das pontas e colapsa espaços internos repetidos.
func limpar(s string) string { return strings.Join(strings.Fields(s), " ") }

// Texto monta o endereço formatado gravado em academias.endereco (compatibilidade com quem lê o texto):
// "Logradouro, Número - Complemento - Bairro, Cidade - UF, CEP 00000-000". Cabe em 500 caracteres com
// os tamanhos máximos dos campos (por garantia, corta no limite).
func (e *enderecoNormalizado) Texto() string {
	var b strings.Builder
	b.WriteString(e.Logradouro + ", " + e.Numero)
	if e.Complemento != "" {
		b.WriteString(" - " + e.Complemento)
	}
	b.WriteString(" - " + e.Bairro + ", " + e.Cidade + " - " + e.UF + ", CEP " + e.CEP[:5] + "-" + e.CEP[5:])
	t := b.String()
	if utf8.RuneCountInString(t) > enderecoMax {
		t = string([]rune(t)[:enderecoMax])
	}
	return t
}

// aplicar copia o endereço para o model.
func (e *enderecoNormalizado) aplicar(a *models.Academia) {
	a.Endereco = e.Texto()
	a.CEP, a.Logradouro, a.Numero, a.Bairro, a.Cidade, a.UF = &e.CEP, &e.Logradouro, &e.Numero, &e.Bairro, &e.Cidade, &e.UF
	a.Complemento = nil
	if e.Complemento != "" {
		a.Complemento = &e.Complemento
	}
}

// campos devolve o mapa de colunas para o UPDATE da edição.
func (e *enderecoNormalizado) campos() map[string]any {
	var compl any
	if e.Complemento != "" {
		compl = e.Complemento
	}
	return map[string]any{"endereco": e.Texto(), "cep": e.CEP, "logradouro": e.Logradouro, "numero": e.Numero,
		"complemento": compl, "bairro": e.Bairro, "cidade": e.Cidade, "uf": e.UF}
}

// AplicarEnderecoAcademia valida o endereço estruturado (mesma regra do cadastro) e o copia para o model,
// montando o texto de `endereco`. Usado fora do fluxo HTTP (ex.: CLI `seed-teste`).
func AplicarEnderecoAcademia(a *models.Academia, r *models.EnderecoAcademiaRequest) error {
	e, err := normalizarEndereco(r)
	if err != nil {
		return err
	}
	e.aplicar(a)
	return nil
}
