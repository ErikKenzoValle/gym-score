package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// Consultas do Nível GymScore e das estatísticas do atleta (rodada 6, U3/U5). Funções de pacote que
// recebem o *gorm.DB (ou a transação do chamador): o recálculo roda dentro das transações dos eventos
// (pódio, anulação, treino) e no job de reconciliação.

// desafioContaSQL: o desafio conta para o nível/estatísticas — encerrado e com o resultado NÃO anulado
// (anulação em devolver_inscricoes mantém o status encerrado; em relancar_podio o pódio é apagado e o
// relançamento limpa a anulação).
const desafioContaSQL = `d.status = 'encerrado' AND d.resultado_anulado_em IS NULL`

// ComponentesGymScore são as contagens que formam a pontuação do Nível GymScore de um usuário.
type ComponentesGymScore struct {
	Primeiros       int // pódios em 1º
	Segundos        int
	Terceiros       int
	OutrosPremiados int // 4º em diante
	Presencas       int // presença confirmada em desafio encerrado (com ou sem pódio)
	DiasTreino      int // dias distintos com treino registrado (treino_conclusoes)
}

// CarregarComponentesGymScore conta, numa consulta, os componentes da pontuação do usuário (todo o
// histórico). Dentro de uma transação, enxerga o que ela mesma gravou (ex.: o pódio recém-lançado).
func CarregarComponentesGymScore(db *gorm.DB, idUsuario uint) (ComponentesGymScore, error) {
	var c ComponentesGymScore
	err := db.Raw(`SELECT
		(SELECT COUNT(*) FROM podio_desafios pd JOIN desafios d ON d.id = pd.id_desafio
		  WHERE pd.id_participante = ? AND pd.posicao = 1 AND `+desafioContaSQL+`) AS primeiros,
		(SELECT COUNT(*) FROM podio_desafios pd JOIN desafios d ON d.id = pd.id_desafio
		  WHERE pd.id_participante = ? AND pd.posicao = 2 AND `+desafioContaSQL+`) AS segundos,
		(SELECT COUNT(*) FROM podio_desafios pd JOIN desafios d ON d.id = pd.id_desafio
		  WHERE pd.id_participante = ? AND pd.posicao = 3 AND `+desafioContaSQL+`) AS terceiros,
		(SELECT COUNT(*) FROM podio_desafios pd JOIN desafios d ON d.id = pd.id_desafio
		  WHERE pd.id_participante = ? AND pd.posicao > 3 AND `+desafioContaSQL+`) AS outros_premiados,
		(SELECT COUNT(*) FROM desafio_participantes p JOIN desafios d ON d.id = p.id_desafio
		  WHERE p.id_usuario = ? AND p.presente = 1 AND `+desafioContaSQL+`) AS presencas,
		(SELECT COUNT(DISTINCT DATE(tc.concluido_em)) FROM treino_conclusoes tc
		  WHERE tc.id_usuario = ? AND tc.concluido_em IS NOT NULL) AS dias_treino`,
		idUsuario, idUsuario, idUsuario, idUsuario, idUsuario, idUsuario).Scan(&c).Error
	return c, err
}

// GravarGymScore grava o cache do nível (os campos são somente leitura no model: UPDATE explícito).
func GravarGymScore(db *gorm.DB, idUsuario uint, pontuacao, nivel int, em time.Time) error {
	return db.Exec(`UPDATE usuarios SET pontuacao_gymscore = ?, nivel_gymscore = ?, gymscore_calculado_em = ? WHERE id = ?`,
		pontuacao, nivel, em, idUsuario).Error
}

// TravarUsuariosOrdenados trava (FOR UPDATE) as linhas dos usuários em ordem crescente de id — a mesma
// ordem dos créditos — para recalcular o nível com uma leitura que começa depois da trava.
func TravarUsuariosOrdenados(db *gorm.DB, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	var out []uint
	return db.Raw("SELECT id FROM usuarios WHERE id IN ? ORDER BY id FOR UPDATE", ids).Scan(&out).Error
}

// UsuariosParaReconciliarGymScore devolve até `limite` usuários (contas de atleta) cujo nível precisa
// ser recalculado pelo job: primeiro os nunca calculados (NULL), depois os calculados antes de `antesDe`,
// do mais antigo para o mais novo (índice idx_usuarios_gymscore_calculado da 00022).
func UsuariosParaReconciliarGymScore(db *gorm.DB, antesDe time.Time, limite int) ([]uint, error) {
	var ids []uint
	err := db.Raw(`SELECT id FROM usuarios
		WHERE perfil = 'atleta' AND (gymscore_calculado_em IS NULL OR gymscore_calculado_em < ?)
		ORDER BY gymscore_calculado_em IS NOT NULL, gymscore_calculado_em, id
		LIMIT ?`, antesDe, limite).Scan(&ids).Error
	return ids, err
}

// TreinoNaJanela é a quantidade de exercícios concluídos de um grupo muscular num dia.
type TreinoNaJanela struct {
	GrupoMuscular string
	Dia           time.Time
	N             int
}

// TreinosNaJanela agrega os exercícios concluídos do usuário por grupo muscular e dia desde `desde`.
func TreinosNaJanela(db *gorm.DB, idUsuario uint, desde time.Time) ([]TreinoNaJanela, error) {
	var out []TreinoNaJanela
	err := db.Raw(`SELECT LOWER(grupo_muscular) AS grupo_muscular, DATE(concluido_em) AS dia, COUNT(*) AS n
		FROM treino_conclusoes WHERE id_usuario = ? AND concluido_em >= ?
		GROUP BY LOWER(grupo_muscular), DATE(concluido_em)`, idUsuario, desde).Scan(&out).Error
	return out, err
}

// DesafioNaJanela é uma participação com presença confirmada num desafio que conta (encerrado, não
// anulado), com a posição no pódio (nil = presente fora do pódio).
type DesafioNaJanela struct {
	IDDesafio uint
	Categoria string
	Quando    time.Time
	Posicao   *int
}

// DesafiosNaJanela devolve as participações com presença do usuário em desafios que contam, cujo evento
// (ou encerramento, em legados sem data) é desde `desde`.
func DesafiosNaJanela(db *gorm.DB, idUsuario uint, desde time.Time) ([]DesafioNaJanela, error) {
	var out []DesafioNaJanela
	err := db.Raw(`SELECT d.id AS id_desafio, d.categoria, COALESCE(d.data_evento, d.data_encerramento, d.criado_em) AS quando,
			pd.posicao
		FROM desafio_participantes p
		JOIN desafios d ON d.id = p.id_desafio
		LEFT JOIN podio_desafios pd ON pd.id_desafio = d.id AND pd.id_participante = p.id_usuario
		WHERE p.id_usuario = ? AND p.presente = 1 AND `+desafioContaSQL+`
		  AND COALESCE(d.data_evento, d.data_encerramento, d.criado_em) >= ?
		ORDER BY quando`, idUsuario, desde).Scan(&out).Error
	return out, err
}

// PerfilGymScore são os dados do usuário que o nível/estatísticas/filtros precisam.
type PerfilGymScore struct {
	ID                  uint
	Perfil              string
	DataNascimento      *time.Time
	PontuacaoGymScore   int
	NivelGymScore       int
	GymScoreCalculadoEm *time.Time
}

// BuscarPerfilGymScore lê o perfil (nil se não existir).
func BuscarPerfilGymScore(db *gorm.DB, idUsuario uint) (*PerfilGymScore, error) {
	var out []PerfilGymScore
	err := db.Model(&models.Usuario{}).
		Select("id, perfil, data_nascimento, pontuacao_gymscore, nivel_gymscore, gymscore_calculado_em").
		Where("id = ?", idUsuario).Limit(1).Scan(&out).Error
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return &out[0], nil
}
