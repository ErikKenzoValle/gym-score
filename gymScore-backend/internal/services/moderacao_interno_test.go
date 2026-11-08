package services

import (
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
)

func TestDetectarImagem_MagicBytes(t *testing.T) {
	casos := []struct {
		nome string
		b    []byte
		mime string
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0}, "image/jpeg"},
		{"png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 0}, "image/png"},
		{"webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), "image/webp"},
		{"riff sem WEBP (wav)", []byte("RIFF\x10\x00\x00\x00WAVEfmt "), ""},
		{"html disfarçado", []byte("<html><script>alert(1)</script>"), ""},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), ""},
		{"gif", []byte("GIF89a......"), ""},
		{"curto", []byte{0xFF, 0xD8}, ""},
		{"vazio", nil, ""},
	}
	for _, c := range casos {
		if got := DetectarImagem(c.b); got != c.mime {
			t.Errorf("%s: %q, esperava %q", c.nome, got, c.mime)
		}
	}
}

func TestJurisdicaoDaDenuncia(t *testing.T) {
	acad, dono := uint(5), uint(50)
	comAcademia := &repositories.DesafioDenuncia{ID: 1, IDAcademia: &acad, IDUsuarioAcademia: &dono}
	casos := []struct {
		nome       string
		categoria  string
		desafio    *repositories.DesafioDenuncia
		denunciado uint
		jur        string
		academia   *uint
	}{
		{"suborno em desafio de academia", models.DenunciaSubornoInstrutor, comAcademia, 7, models.JurisdicaoAcademia, &acad},
		{"denunciado é a própria academia", models.DenunciaSubornoInstrutor, comAcademia, dono, models.JurisdicaoAdmin, &acad},
		{"manipulação de pódio", models.DenunciaManipulacaoPodio, comAcademia, 7, models.JurisdicaoAdmin, nil},
		{"conduta sem desafio", models.DenunciaCondutaAbusiva, nil, 7, models.JurisdicaoAdmin, nil},
		{"suborno em desafio sem academia", models.DenunciaSubornoInstrutor, &repositories.DesafioDenuncia{ID: 2}, 7, models.JurisdicaoAdmin, nil},
	}
	for _, c := range casos {
		j, a := jurisdicaoDaDenuncia(c.categoria, c.desafio, c.denunciado)
		if j != c.jur || (a == nil) != (c.academia == nil) || (a != nil && *a != *c.academia) {
			t.Errorf("%s: %s %v", c.nome, j, a)
		}
	}
}

func TestTextoEntre(t *testing.T) {
	if _, err := textoEntre("  curto ", 20, 2000, "A descrição"); err == nil {
		t.Error("texto curto deveria falhar")
	}
	// conta runas, não bytes: 20 letras acentuadas passam
	if v, err := textoEntre("  "+strings.Repeat("é", 20)+"  ", 20, 2000, "A descrição"); err != nil || v != strings.Repeat("é", 20) {
		t.Errorf("runas: %q %v", v, err)
	}
	if _, err := textoEntre(strings.Repeat("a", 2001), 20, 2000, "A descrição"); err == nil {
		t.Error("texto longo deveria falhar")
	}
}

func TestValidarPunicaoAcademia(t *testing.T) {
	agora := time.Now()
	futuro, passado := agora.Add(time.Hour), agora.Add(-time.Hour)
	casos := []struct {
		p  models.PunicaoAcademiaRequest
		ok bool
	}{
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoAdvertencia}, true},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoAdvertencia, Fim: &futuro}, false},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoBloqueioAcademia}, true},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoBloqueioAcademia, Fim: &futuro}, true},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoBloqueioAcademia, Fim: &passado}, false},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoSuspensao, Fim: &futuro}, false},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoBanimento}, false},
		{models.PunicaoAcademiaRequest{Tipo: models.PunicaoAnulacaoResultado}, false},
		{models.PunicaoAcademiaRequest{Tipo: "xpto"}, false},
	}
	for _, c := range casos {
		if err := validarPunicaoAcademia(&c.p, agora); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.p, err)
		}
	}
}

func TestValidarPunicaoAdmin_SemAcademia(t *testing.T) {
	s := &moderacaoService{}
	agora := time.Now()
	futuro, passado := agora.Add(time.Hour), agora.Add(-time.Hour)
	acad := uint(1)
	casos := []struct {
		tipo   string
		acad   *uint
		fim    *time.Time
		ok     bool
		escopo string
	}{
		{models.PunicaoAdvertencia, nil, nil, true, models.EscopoGlobal},
		{models.PunicaoSuspensao, nil, &futuro, true, models.EscopoGlobal},
		{models.PunicaoSuspensao, nil, nil, false, ""},
		{models.PunicaoSuspensao, nil, &passado, false, ""},
		{models.PunicaoSuspensao, &acad, &futuro, false, ""},
		{models.PunicaoBanimento, nil, nil, true, models.EscopoGlobal},
		{models.PunicaoBanimento, nil, &futuro, false, ""},
		{models.PunicaoBloqueioAcademia, nil, nil, false, ""},
		{models.PunicaoAnulacaoResultado, nil, nil, false, ""},
		{"xpto", nil, nil, false, ""},
	}
	for _, c := range casos {
		p, err := s.validarPunicaoAdmin(c.tipo, c.acad, c.fim, agora)
		if (err == nil) != c.ok || (c.ok && p.Escopo != c.escopo) {
			t.Errorf("%s acad=%v fim=%v: %+v %v", c.tipo, c.acad, c.fim, p, err)
		}
	}
}

func TestPodeRecorrer(t *testing.T) {
	s := &moderacaoService{cfg: ModeracaoConfig{PrazoRecurso: 15 * 24 * time.Hour}}
	agora := time.Now()
	p := &models.Punicao{Inicio: agora.Add(-24 * time.Hour)}
	if !s.podeRecorrer(p, nil, agora) {
		t.Error("dentro do prazo, sem recurso: pode recorrer")
	}
	if s.podeRecorrer(p, &models.Recurso{}, agora) {
		t.Error("já tem recurso")
	}
	if s.podeRecorrer(&models.Punicao{Inicio: agora.Add(-16 * 24 * time.Hour)}, nil, agora) {
		t.Error("fora do prazo")
	}
	rev := agora
	if s.podeRecorrer(&models.Punicao{Inicio: agora, RevogadaEm: &rev}, nil, agora) {
		t.Error("revogada")
	}
}

func TestErrosModeracao_Codigos(t *testing.T) {
	fim := time.Now().Add(time.Hour)
	e := ErroContaSuspensa(&fim)
	if e.Status != 403 || e.Codigo != CodigoContaSuspensa || e.Dados == nil {
		t.Errorf("conta_suspensa: %+v", e)
	}
	b := ErroContaBanida(&models.BanimentoInfo{IDPunicao: 3, Motivo: "fraude", PodeRecorrer: true})
	if b.Status != 403 || b.Codigo != CodigoContaBanida || b.Dados.(*models.BanimentoInfo).IDPunicao != 3 {
		t.Errorf("conta_banida: %+v", b)
	}
	if ErroContaBanida(nil).Dados != nil {
		t.Error("sem info não deveria ter dados")
	}
}
