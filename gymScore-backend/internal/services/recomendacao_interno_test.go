package services

import (
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
)

type listaFixaDesafios []models.Desafio

func (l listaFixaDesafios) Listar(repositories.FiltroDesafios) ([]models.Desafio, int64, error) {
	return append([]models.Desafio(nil), l...), int64(len(l)), nil
}
func (l listaFixaDesafios) ListarHistoricoUsuario(uint) ([]models.Desafio, error) { return nil, nil }

// A6-1: com o contexto do serviço de desafios (ComRegrasDeEntrada), a recomendação aplica o MESMO
// pode_entrar do feed — conflito de interesse e choque de horário excluem o desafio, sem regra duplicada.
func TestRecomendar_DescartaConflitoDeInteresseEChoqueDeHorario(t *testing.T) {
	agora := time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)
	ini := agora.Add(48 * time.Hour)
	fim := ini.Add(2 * time.Hour)
	acad, outraAcad := uint(7), uint(8)
	ds := listaFixaDesafios{
		{ID: 1, Status: models.StatusAberto, Titulo: "Na equipe", Categoria: "corrida", Vagas: 10, IDAcademia: &outraAcad},
		{ID: 2, Status: models.StatusAberto, Titulo: "Academia vinculada", Categoria: "corrida", Vagas: 10, IDAcademia: &acad},
		{ID: 3, Status: models.StatusAberto, Titulo: "Candidatura ativa", Categoria: "corrida", Vagas: 10, IDAcademia: &outraAcad},
		{ID: 4, Status: models.StatusAberto, Titulo: "Choca com inscrição", Categoria: "corrida", Vagas: 10,
			IDAcademia: &outraAcad, DataEvento: &ini, DataFimEvento: &fim},
		{ID: 5, Status: models.StatusAberto, Titulo: "Livre", Categoria: "corrida", Vagas: 10, IDAcademia: &outraAcad},
	}
	ctxFeed := &contextoUsuario{ID: 42, Perfil: models.ModoAtleta,
		conflito: &conflitoAtleta{InstrutorID: 3, Aprovado: true, AcademiaID: acad,
			Equipe: map[uint]bool{1: true}, Candidatura: map[uint]bool{3: true}},
		inscricoes: []repositories.InscricaoNoHorario{{IDDesafio: 99, Titulo: "Outro", IDAcademia: outraAcad,
			DataEvento: ini.Add(-time.Hour), DataFimEvento: ini.Add(time.Hour)}},
		elegibilidade: &PerfilElegibilidade{Nivel: 3},
	}
	s := NovoRecomendacaoService(ds, nil, nil).(*recomendacaoService)
	s.agora = func() time.Time { return agora }
	s.contexto = func(uint) (*contextoUsuario, error) { return ctxFeed, nil }

	resp, err := s.Recomendar(42)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 1 || resp[0].Desafio.ID != 5 {
		ids := []uint{}
		for _, r := range resp {
			ids = append(ids, r.Desafio.ID)
		}
		t.Fatalf("esperava só o desafio 5 (os demais têm conflito/choque), obteve %v", ids)
	}
	if !resp[0].Desafio.PodeEntrar {
		t.Error("recomendado deveria sair com pode_entrar=true")
	}
}

func TestLimparRazaoIA(t *testing.T) {
	casos := []struct {
		in string
		ok bool
	}{
		{"Sua Resistência em 70/100 combina com esta corrida.", true},
		{"  Linha\nquebrada\tcom  espaços extras aqui.  ", true},
		{"<b>negrito</b> não pode passar aqui", false},
		{"**markdown** também não pode passar", false},
		{"Link www.exemplo.com não pode passar", false},
		{"curta", false},
		{"Entidade &lt;script&gt; codificada também não", false},
	}
	for _, c := range casos {
		got, ok := limparRazaoIA(c.in)
		if ok != c.ok {
			t.Errorf("%q: ok=%v, esperava %v (%q)", c.in, ok, c.ok, got)
		}
	}
	if got, _ := limparRazaoIA("  Linha\nquebrada\tcom  espaços extras aqui.  "); got != "Linha quebrada com espaços extras aqui." {
		t.Errorf("normalização de espaços: %q", got)
	}
}
