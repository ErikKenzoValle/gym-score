package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// PontosPorCheckin é o peso de um check-in de desafio no ranking (exercício concluído vale 1).
const PontosPorCheckin = 3

// RankingRepository calcula o ranking entre amigos.
type RankingRepository interface {
	Amigos(idUsuario uint, desde time.Time) ([]models.RankingItem, error)
}

type rankingRepository struct{ db *gorm.DB }

func NovoRankingRepository(db *gorm.DB) RankingRepository { return &rankingRepository{db} }

// Amigos devolve o usuário + amigos aceitos com exercícios, check-ins e pontos desde `desde`,
// ordenado por pontos desc (desempate por nome). Posição segue a regra de competição (empate
// compartilha posição). Tudo em uma única consulta.
func (r *rankingRepository) Amigos(idUsuario uint, desde time.Time) ([]models.RankingItem, error) {
	const sqlRanking = `
SELECT u.id AS id, u.nome AS nome, u.sobrenome AS sobrenome,
  (SELECT COUNT(*) FROM treino_conclusoes tc WHERE tc.id_usuario = u.id AND tc.concluido_em >= ?) AS exercicios,
  (SELECT COUNT(*) FROM desafio_checkins ck WHERE ck.id_usuario = u.id AND ck.criado_em >= ?) AS checkins
FROM usuarios u
WHERE u.id = ? OR u.id IN (
  SELECT CASE WHEN a.id_usuario = ? THEN a.id_amigo ELSE a.id_usuario END
  FROM amizades a
  WHERE a.status = 'aceita' AND (a.id_usuario = ? OR a.id_amigo = ?))`
	var itens []models.RankingItem
	if err := r.db.Raw(sqlRanking, desde, desde, idUsuario, idUsuario, idUsuario, idUsuario).Scan(&itens).Error; err != nil {
		return nil, err
	}
	for i := range itens {
		itens[i].Pontos = itens[i].Exercicios + PontosPorCheckin*itens[i].Checkins
		itens[i].Voce = itens[i].ID == idUsuario
	}
	sortRanking(itens)
	return itens, nil
}

func sortRanking(itens []models.RankingItem) {
	// insertion sort estável: pontos desc, nome asc, id asc (listas pequenas)
	for i := 1; i < len(itens); i++ {
		for j := i; j > 0 && antes(itens[j], itens[j-1]); j-- {
			itens[j], itens[j-1] = itens[j-1], itens[j]
		}
	}
	for i := range itens {
		if i > 0 && itens[i].Pontos == itens[i-1].Pontos {
			itens[i].Posicao = itens[i-1].Posicao
		} else {
			itens[i].Posicao = i + 1
		}
	}
}

func antes(a, b models.RankingItem) bool {
	if a.Pontos != b.Pontos {
		return a.Pontos > b.Pontos
	}
	if a.Nome != b.Nome {
		return a.Nome < b.Nome
	}
	return a.ID < b.ID
}
