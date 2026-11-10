package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// preloadPremiacao carrega a divisão do prêmio por posição (desafio_premio_posicoes, 00022) em ordem.
func preloadPremiacao(db *gorm.DB) *gorm.DB {
	return db.Preload("PremioPosicoes", func(q *gorm.DB) *gorm.DB { return q.Order("posicao ASC") })
}

// ListarDivisaoPremio devolve a divisão do prêmio do desafio em ordem de posição. Dentro das ações com o
// desafio travado é chamada DEPOIS das travas (ver comDesafioEUsuarioTravados): não é leitura de trava.
func (r *DesafioRepository) ListarDivisaoPremio(idDesafio uint) ([]models.DesafioPremioPosicao, error) {
	var lista []models.DesafioPremioPosicao
	err := r.db.Where("id_desafio = ?", idDesafio).Order("posicao ASC").Find(&lista).Error
	return lista, err
}

// SubstituirDivisaoPremio grava a divisão do prêmio (pctBP[i] = posição i+1) apagando a anterior, na
// transação do chamador (com o desafio travado). A validação (soma 10000, não crescente, uma fatia por
// posição) é do serviço; o banco só confere posição 1..100 e fatia 1..10000.
func (r *DesafioRepository) SubstituirDivisaoPremio(idDesafio uint, pctBP []int) error {
	if err := r.db.Where("id_desafio = ?", idDesafio).Delete(&models.DesafioPremioPosicao{}).Error; err != nil {
		return err
	}
	if len(pctBP) == 0 {
		return nil
	}
	linhas := make([]models.DesafioPremioPosicao, len(pctBP))
	for i, v := range pctBP {
		linhas[i] = models.DesafioPremioPosicao{IDDesafio: idDesafio, Posicao: i + 1, PctBP: v}
	}
	return r.db.Create(&linhas).Error
}

// InscritosTravados devolve os inscritos do desafio com leitura de trava (FOR SHARE): só serve com o
// desafio já travado — as inscrições dele só mudam sob a mesma trava, então não há espera aqui.
func (r *DesafioRepository) InscritosTravados(idDesafio uint) ([]uint, error) {
	var ids []uint
	err := r.db.Raw("SELECT id_usuario FROM desafio_participantes WHERE id_desafio = ? ORDER BY id_usuario FOR SHARE", idDesafio).Scan(&ids).Error
	return ids, err
}

// ChoqueDeInscritos conta os inscritos do desafio que também estão inscritos em OUTRO desafio aberto ou
// em andamento da mesma academia com intervalo [início, fim) sobreposto a [inicio, fim) (legados sem
// fim não chocam) e devolve o título de um desses desafios (o de início mais cedo).
func (r *DesafioRepository) ChoqueDeInscritos(idDesafio, idAcademia uint, inicio, fim time.Time) (int64, string, error) {
	var out []struct {
		Atletas int64
		Titulo  *string
	}
	err := r.db.Raw(`SELECT COUNT(DISTINCT p.id_usuario) AS atletas,
			(SELECT d2.titulo FROM desafio_participantes p2 JOIN desafios d2 ON d2.id = p2.id_desafio
			  JOIN desafio_participantes p3 ON p3.id_usuario = p2.id_usuario AND p3.id_desafio = ?
			  WHERE d2.id <> ? AND d2.id_academia = ? AND d2.status IN ('aberto','em_andamento')
			    AND d2.data_evento IS NOT NULL AND d2.data_fim_evento IS NOT NULL
			    AND d2.data_evento < ? AND d2.data_fim_evento > ?
			  ORDER BY d2.data_evento, d2.id LIMIT 1) AS titulo
		FROM desafio_participantes p
		JOIN desafio_participantes o ON o.id_usuario = p.id_usuario AND o.id_desafio <> p.id_desafio
		JOIN desafios d ON d.id = o.id_desafio
		WHERE p.id_desafio = ? AND d.id_academia = ? AND d.status IN ('aberto','em_andamento')
		  AND d.data_evento IS NOT NULL AND d.data_fim_evento IS NOT NULL
		  AND d.data_evento < ? AND d.data_fim_evento > ?`,
		idDesafio, idDesafio, idAcademia, fim, inicio, idDesafio, idAcademia, fim, inicio).Scan(&out).Error
	if err != nil || len(out) == 0 {
		return 0, "", err
	}
	titulo := ""
	if out[0].Titulo != nil {
		titulo = *out[0].Titulo
	}
	return out[0].Atletas, titulo, nil
}
