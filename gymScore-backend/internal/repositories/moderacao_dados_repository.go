package repositories

import (
	"fmt"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ModeracaoEventoRepository grava e lê a linha do tempo append-only (`moderacao_eventos`). Registrar
// roda na MESMA transação da ação (como admin_auditoria); não há UPDATE nem DELETE.
type ModeracaoEventoRepository interface {
	// Registrar insere o evento. O CHECK chk_eventos_ator exige: admin ⇒ IDAdmin; usuario/academia ⇒
	// IDAtor; sistema ⇒ nenhum.
	Registrar(tx *gorm.DB, e *models.ModeracaoEvento) error
	// PorEntidade / PorDesafio devolvem a linha do tempo em ordem cronológica (id ASC), paginada.
	PorEntidade(entidade string, idEntidade uint, pagina, limite int) ([]models.ModeracaoEvento, int64, error)
	PorDesafio(idDesafio uint, pagina, limite int) ([]models.ModeracaoEvento, int64, error)
}

type moderacaoEventoRepository struct{ db *gorm.DB }

// NovoModeracaoEventoRepository cria o repositório da linha do tempo da moderação.
func NovoModeracaoEventoRepository(db *gorm.DB) ModeracaoEventoRepository {
	return &moderacaoEventoRepository{db}
}

func (r *moderacaoEventoRepository) Registrar(tx *gorm.DB, e *models.ModeracaoEvento) error {
	return dbOuTx(r.db, tx).Create(e).Error
}

func (r *moderacaoEventoRepository) PorEntidade(entidade string, idEntidade uint, pagina, limite int) ([]models.ModeracaoEvento, int64, error) {
	q := r.db.Model(&models.ModeracaoEvento{}).Where("entidade = ? AND id_entidade = ?", entidade, idEntidade)
	return paginarContando[models.ModeracaoEvento](q, pagina, limite, "id ASC", nil)
}

func (r *moderacaoEventoRepository) PorDesafio(idDesafio uint, pagina, limite int) ([]models.ModeracaoEvento, int64, error) {
	q := r.db.Model(&models.ModeracaoEvento{}).Where("id_desafio = ?", idDesafio)
	return paginarContando[models.ModeracaoEvento](q, pagina, limite, "id ASC", nil)
}

// DesafioModeracaoRepository grava as colunas de pódio/anulação de `desafios` (00013) por UPDATE
// condicional. O CAS em_andamento -> encerrado do pódio continua no DesafioRepository (passar
// podio_lancado_por/podio_lancado_em no mapa de extras).
type DesafioModeracaoRepository interface {
	// AnularResultado: encerrado e sem anulação vigente -> resultado_anulado_em/modo. false se não cabia.
	AnularResultado(tx *gorm.DB, idDesafio uint, modo string, em time.Time) (bool, error)
	// ReabrirParaRelancamento (Q3, modo relancar_podio): encerrado -> em_andamento, limpa
	// podio_lancado_por/em. A marca de anulação fica (o job de expiração deve ignorar esses desafios até o
	// relançamento; a carência conta de resultado_anulado_em). false se não estava anulado em relancar_podio.
	ReabrirParaRelancamento(tx *gorm.DB, idDesafio uint) (bool, error)
	// LimparAnulacaoNoRelancamento apaga a marca de anulação quando o novo pódio é lançado (histórico
	// continua em punicoes/moderacao_eventos). false se o desafio não estava anulado.
	LimparAnulacaoNoRelancamento(tx *gorm.DB, idDesafio uint) (bool, error)
	// RemoverPodio apaga as posições do pódio e zera prêmio/vencedor dos participantes (relancar_podio).
	RemoverPodio(tx *gorm.DB, idDesafio uint) error
	// DecrementarPontos desfaz pontos do pódio anulado (piso 0).
	DecrementarPontos(tx *gorm.DB, idUsuario uint, pontos int) error
	// RegistrarEstornoTaxa grava o estorno da taxa da plataforma (Q6; valor positivo informado, gravado
	// NEGATIVO com origem estorno_taxa). 1062 = já estornada para o desafio.
	RegistrarEstornoTaxa(tx *gorm.DB, idDesafio uint, valor int64) error
}

type desafioModeracaoRepository struct{ db *gorm.DB }

// NovoDesafioModeracaoRepository cria o repositório de pódio/anulação de desafios.
func NovoDesafioModeracaoRepository(db *gorm.DB) DesafioModeracaoRepository {
	return &desafioModeracaoRepository{db}
}

func (r *desafioModeracaoRepository) AnularResultado(tx *gorm.DB, idDesafio uint, modo string, em time.Time) (bool, error) {
	if modo != models.AnulacaoRelancarPodio && modo != models.AnulacaoDevolverInscricoes {
		return false, gorm.ErrInvalidValue
	}
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		`UPDATE desafios SET resultado_anulado_em = ?, resultado_anulacao_modo = ?
		  WHERE id = ? AND status = ? AND resultado_anulado_em IS NULL`,
		em, modo, idDesafio, models.StatusEncerrado))
}

func (r *desafioModeracaoRepository) ReabrirParaRelancamento(tx *gorm.DB, idDesafio uint) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		`UPDATE desafios SET status = ?, podio_lancado_por = NULL, podio_lancado_em = NULL
		  WHERE id = ? AND status = ? AND resultado_anulacao_modo = ?`,
		models.StatusEmAndamento, idDesafio, models.StatusEncerrado, models.AnulacaoRelancarPodio))
}

func (r *desafioModeracaoRepository) LimparAnulacaoNoRelancamento(tx *gorm.DB, idDesafio uint) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		`UPDATE desafios SET resultado_anulado_em = NULL, resultado_anulacao_modo = NULL
		  WHERE id = ? AND resultado_anulacao_modo = ?`, idDesafio, models.AnulacaoRelancarPodio))
}

func (r *desafioModeracaoRepository) DecrementarPontos(tx *gorm.DB, idUsuario uint, pontos int) error {
	if pontos <= 0 {
		return nil
	}
	return dbOuTx(r.db, tx).Exec("UPDATE usuarios SET pontos = GREATEST(pontos - ?, 0) WHERE id = ?", pontos, idUsuario).Error
}

func (r *desafioModeracaoRepository) RegistrarEstornoTaxa(tx *gorm.DB, idDesafio uint, valor int64) error {
	if valor <= 0 {
		return fmt.Errorf("valor do estorno de taxa deve ser positivo")
	}
	return dbOuTx(r.db, tx).Create(&models.PlataformaReceita{
		Origem: models.ReceitaEstornoTaxa, ValorCentavos: -valor, IDDesafio: &idDesafio,
	}).Error
}
