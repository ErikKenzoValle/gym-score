package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
)

// Histórico do seed (rodada 6): dados para os gráficos de teia e o Nível GymScore das contas de teste.
//   - 6 desafios ENCERRADOS e GRATUITOS da academia de teste (sem ledger: Σ saldos = Σ ledger intacto),
//     de categorias variadas, nos últimos 180 dias, com a atleta (e às vezes a conta pendente) presentes
//     e no pódio;
//   - treinos registrados (treino_conclusoes) da atleta (pernas/abdominal/peito/costas, perfil corredora)
//     e do instrutor (membros superiores, perfil força).
// Idempotente: cada desafio é reconhecido pelo título + academia; os treinos pelo código do treino do seed
// (criado junto com as conclusões, na mesma transação). As datas são relativas ao dia da 1ª execução.

// historicoSeed descreve um desafio encerrado do histórico. podio: quem ficou em cada posição
// ("ana" | "paula"); presentes fora do pódio em extras.
type historicoSeed struct {
	titulo    string
	categoria string
	diasAtras int
	podio     []string
	extras    []string
}

var historicoDesafiosSeed = []historicoSeed{
	{"Histórico de teste: Corrida 10 km", "corrida", 150, []string{"ana", "paula"}, nil},
	{"Histórico de teste: Pedal 40 km", "ciclismo", 110, []string{"paula", "ana"}, nil},
	{"Histórico de teste: Circuito funcional", "funcional", 75, []string{"ana"}, nil},
	{"Histórico de teste: Travessia 1 km", "natacao", 45, []string{"ana"}, nil},
	{"Histórico de teste: Corrida 5 km noturna", "corrida", 20, []string{"ana"}, nil},
	{"Histórico de teste: WOD de sábado", "crossfit", 8, []string{"ana"}, nil},
}

// treinoSeed: código do treino (único), nome, a cada quantos dias treina e os grupos de cada sessão.
type treinoSeed struct {
	codigo    string
	nome      string
	intervalo int
	sessoes   [][]string
}

var (
	treinoAtletaSeed = treinoSeed{"SEEDANA6", "Treino de teste: corredora", 3, [][]string{
		{"pernas", "pernas", "abdominal", "abdominal"},
		{"pernas", "costas", "peito", "abdominal"},
		{"pernas", "pernas", "ombro", "abdominal", "costas"},
	}}
	treinoInstrutorSeed = treinoSeed{"SEEDIGOR6", "Treino de teste: força", 4, [][]string{
		{"peito", "peito", "triceps", "ombro", "abdominal"},
		{"costas", "costas", "biceps", "antebraco"},
		{"pernas", "pernas", "peito", "ombro", "triceps", "biceps"},
	}}
)

func (s *semeador) historico(acad *models.Academia, ana, paula, igor *models.Usuario, instr *models.Instrutor) error {
	pessoas := map[string]uint{"ana": ana.ID, "paula": paula.ID}
	for _, h := range historicoDesafiosSeed {
		criado, err := s.desafioHistorico(acad, instr, h, pessoas)
		if err != nil {
			return fmt.Errorf("histórico %q: %w", h.titulo, err)
		}
		if criado {
			s.res.HistoricoCriado++
		}
	}
	for _, c := range []struct {
		u *models.Usuario
		t treinoSeed
	}{{ana, treinoAtletaSeed}, {igor, treinoInstrutorSeed}} {
		criado, err := s.treinosHistorico(c.u.ID, c.t)
		if err != nil {
			return err
		}
		if criado {
			s.res.TreinosCriados++
		}
	}
	return nil
}

func codigoIngressoSeed() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// desafioHistorico cria (uma vez) um desafio encerrado gratuito com presença e pódio.
func (s *semeador) desafioHistorico(acad *models.Academia, instr *models.Instrutor, h historicoSeed, pessoas map[string]uint) (bool, error) {
	var n int64
	if err := s.tx.Model(&models.Desafio{}).Where("id_academia = ? AND titulo = ?", acad.ID, h.titulo).Count(&n).Error; err != nil || n > 0 {
		return false, err
	}
	base := s.agora.AddDate(0, 0, -h.diasAtras)
	inicio := time.Date(base.Year(), base.Month(), base.Day(), 8, 0, 0, 0, time.Local)
	fim := inicio.Add(duracaoEventoSeed)
	resumo := "Desafio encerrado fictício (histórico do seed-teste) para alimentar os gráficos do perfil."
	regras := regrasSeed
	d := models.Desafio{Titulo: h.titulo, Resumo: &resumo, Regras: &regras, Categoria: h.categoria,
		ValorEntradaCentavos: 0, DataEvento: &inicio, DataFimEvento: &fim, DataEncerramento: &fim,
		Status: models.StatusEncerrado, Vagas: 20, PosicoesPodio: len(h.podio), IDCriador: acad.IDUsuario,
		IDAcademia: &acad.ID, IDInstrutor: &instr.ID, DividirPote: true, InstrutorModoPagamento: models.InstrutorFixo,
		InstrutoresVagas: 1, InstrutorSelecao: models.SelecaoDesignado}
	if err := services.AplicarEnderecoDesafio(&d, enderecoDaAcademia(acad)); err != nil {
		return false, err
	}
	repo := repositories.NovoDesafioRepository(s.tx)
	if err := repo.Criar(&d); err != nil {
		return false, err
	}
	if err := s.garantirTextoEDivisao(&d, nil); err != nil {
		return false, err
	}
	if err := repo.AdicionarMembro(&models.DesafioInstrutor{IDDesafio: d.ID, IDInstrutor: instr.ID,
		Origem: models.EquipeOrigemIndicado, Responsavel: true}); err != nil {
		return false, err
	}
	presenca := inicio.Add(-15 * time.Minute)
	modo := models.PresencaManual
	marcar := func(quem string, posicao int) error {
		codigo, err := codigoIngressoSeed()
		if err != nil {
			return err
		}
		p := &models.DesafioParticipante{IDDesafio: d.ID, IDUsuario: pessoas[quem], CodigoIngresso: codigo,
			Presente: true, PresenteEm: &presenca, PresencaMarcadaPor: &instr.IDUsuario, PresencaModo: &modo,
			Vencedor: posicao == 1}
		if err := repo.AdicionarParticipante(p); err != nil {
			return err
		}
		if posicao == 0 {
			return nil
		}
		return repo.InserirPodio(&models.PodioDesafio{IDDesafio: d.ID, Posicao: posicao, IDParticipante: pessoas[quem],
			Marca: fmt.Sprintf("%dº lugar (seed)", posicao)})
	}
	for i, quem := range h.podio {
		if err := marcar(quem, i+1); err != nil {
			return false, err
		}
	}
	for _, quem := range h.extras {
		if err := marcar(quem, 0); err != nil {
			return false, err
		}
	}
	return true, nil
}

// treinosHistorico cria (uma vez) o treino do seed do usuário e as conclusões dos últimos 180 dias.
func (s *semeador) treinosHistorico(idUsuario uint, t treinoSeed) (bool, error) {
	var n int64
	if err := s.tx.Model(&models.Treino{}).Where("codigo = ?", t.codigo).Count(&n).Error; err != nil || n > 0 {
		return false, err
	}
	treino := models.Treino{IDUsuario: idUsuario, Nome: t.nome, Descricao: "Criado pelo seed-teste", Codigo: t.codigo}
	for i, g := range t.sessoes[0] {
		treino.Exercicios = append(treino.Exercicios, models.TreinoExercicio{Nome: "Exercício de " + g, GrupoMuscular: g,
			Ordem: i + 1, Series: 3, Repeticoes: 12})
	}
	if err := s.tx.Create(&treino).Error; err != nil {
		return false, err
	}
	var conclusoes []models.TreinoConclusao
	sessao := 0
	for dias := 178; dias >= 1; dias -= t.intervalo {
		// Mais treino recente (constância crescente): no 1º trimestre pula uma sessão a cada três.
		if dias > 90 && sessao%3 == 2 {
			sessao++
			continue
		}
		base := s.agora.AddDate(0, 0, -dias)
		quando := time.Date(base.Year(), base.Month(), base.Day(), 18, 30, 0, 0, time.Local)
		for _, g := range t.sessoes[sessao%len(t.sessoes)] {
			conclusoes = append(conclusoes, models.TreinoConclusao{IDUsuario: idUsuario, IDTreino: treino.ID,
				GrupoMuscular: g, ConcluidoEm: quando})
		}
		sessao++
	}
	return true, s.tx.CreateInBatches(conclusoes, 200).Error
}
