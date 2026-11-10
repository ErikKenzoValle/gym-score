package services

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// ─── Filtros de entrada (rodada 6, U4) ───────────────────────────────────────

// PerfilElegibilidade são os dados do atleta que os filtros do desafio olham: o Nível GymScore e a data
// de nascimento (idade calculada na data do evento). Nivel 0 = desconhecido (tratado como 1).
type PerfilElegibilidade struct {
	Nivel          int
	DataNascimento *time.Time
}

// MsgSemDataNascimento é o bloqueio de quem não tem data de nascimento num desafio com filtro de idade.
const MsgSemDataNascimento = "Informe sua data de nascimento no perfil para entrar neste desafio"

// idadeEm devolve a idade (anos completos) de quem nasceu em nasc, no instante em (datas locais).
func idadeEm(nasc, em time.Time) int {
	nasc, em = nasc.In(time.Local), em.In(time.Local)
	anos := em.Year() - nasc.Year()
	if em.Month() < nasc.Month() || (em.Month() == nasc.Month() && em.Day() < nasc.Day()) {
		anos--
	}
	return anos
}

// MotivoFiltroEntrada devolve por que o atleta NÃO cumpre os requisitos de nível/idade do desafio ("" =
// cumpre). Idade na data do evento (sem data: agora). Mesmas mensagens do 409 de POST /entrar e de
// motivo_bloqueio_entrada. Ponto único usado por Entrar, pelo feed/detalhe e pela recomendação.
func MotivoFiltroEntrada(d *models.Desafio, p PerfilElegibilidade, agora time.Time) string {
	nivel := p.Nivel
	if nivel < models.NivelGymScoreMin {
		nivel = models.NivelGymScoreMin
	}
	if (d.NivelMin != nil && nivel < *d.NivelMin) || (d.NivelMax != nil && nivel > *d.NivelMax) {
		return fmt.Sprintf("Este desafio é para %s; você está no nível %d", faixaTexto(d.NivelMin, d.NivelMax, "nível"), nivel)
	}
	if d.IdadeMin == nil && d.IdadeMax == nil {
		return ""
	}
	if p.DataNascimento == nil {
		return MsgSemDataNascimento
	}
	quando := agora
	if d.DataEvento != nil {
		quando = *d.DataEvento
	}
	idade := idadeEm(*p.DataNascimento, quando)
	if (d.IdadeMin != nil && idade < *d.IdadeMin) || (d.IdadeMax != nil && idade > *d.IdadeMax) {
		return fmt.Sprintf("Este desafio é para %s na data do evento; você terá %d anos", faixaTexto(d.IdadeMin, d.IdadeMax, "idade"), idade)
	}
	return ""
}

// faixaTexto: "nível 3 a 6", "nível 3 ou mais", "nível até 6"; "idade de 18 a 40 anos", "idade mínima de 18 anos", "idade máxima de 40 anos".
func faixaTexto(min, max *int, tipo string) string {
	if tipo == "nível" {
		switch {
		case min != nil && max != nil:
			return fmt.Sprintf("nível %d a %d", *min, *max)
		case min != nil:
			return fmt.Sprintf("nível %d ou mais", *min)
		default:
			return fmt.Sprintf("nível até %d", *max)
		}
	}
	switch {
	case min != nil && max != nil:
		return fmt.Sprintf("idade de %d a %d anos", *min, *max)
	case min != nil:
		return fmt.Sprintf("idade mínima de %d anos", *min)
	default:
		return fmt.Sprintf("idade máxima de %d anos", *max)
	}
}

// elegibilidadeNaInscricao recalcula (e grava) o nível do atleta na transação do Entrar — com a linha do
// usuário já travada antes da primeira leitura, então o snapshot é fresco: "nível no momento da inscrição".
func elegibilidadeNaInscricao(tx *gorm.DB, userID uint, agora time.Time) (PerfilElegibilidade, error) {
	niveis, err := recalcularGymScoreTx(tx, []uint{userID}, agora)
	if err != nil {
		return PerfilElegibilidade{}, err
	}
	p, err := repositories.BuscarPerfilGymScore(tx, userID)
	if err != nil {
		return PerfilElegibilidade{}, err
	}
	out := PerfilElegibilidade{Nivel: niveis[userID]}
	if p != nil {
		out.DataNascimento = p.DataNascimento
	}
	return out, nil
}

// ─── Validações da criação/edição (U1, U2, U4) ───────────────────────────────

// textoDesafio apara e confere o tamanho (em caracteres) de resumo/regras.
func textoDesafio(campo, v string, min, max int) (string, error) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	if n < min || n > max {
		return "", erroRequisicao(fmt.Sprintf("%s é obrigatório e deve ter de %d a %d caracteres (tem %d)", campo, min, max, n))
	}
	return v, nil
}

// filtroOpcional: nil ou 0 = sem filtro (NULL).
func filtroOpcional(v *int) *int {
	if v == nil || *v == 0 {
		return nil
	}
	x := *v
	return &x
}

// validarFiltros confere os limites e min <= max dos filtros de nível e idade (já normalizados).
func validarFiltros(nivelMin, nivelMax, idadeMin, idadeMax *int) error {
	for _, v := range []*int{nivelMin, nivelMax} {
		if v != nil && (*v < models.NivelGymScoreMin || *v > models.NivelGymScoreMax) {
			return erroRequisicao("nivel_min/nivel_max devem estar entre 1 e 10 (0 = sem filtro)")
		}
	}
	for _, v := range []*int{idadeMin, idadeMax} {
		if v != nil && (*v < 0 || *v > models.IdadeFiltroMax) {
			return erroRequisicao("idade_min/idade_max devem estar entre 0 e 120 (0 = sem filtro)")
		}
	}
	if nivelMin != nil && nivelMax != nil && *nivelMin > *nivelMax {
		return erroRequisicao("nivel_min não pode ser maior que nivel_max")
	}
	if idadeMin != nil && idadeMax != nil && *idadeMin > *idadeMax {
		return erroRequisicao("idade_min não pode ser maior que idade_max")
	}
	return nil
}

// divisaoDoPedido devolve a divisão do prêmio do pedido validada, ou a padrão do tamanho do pódio se
// não veio.
func divisaoDoPedido(bp []int, posicoes int) ([]int, error) {
	if len(bp) == 0 {
		return DivisaoPadraoBP(posicoes), nil
	}
	if err := ValidarDivisaoBP(bp, posicoes); err != nil {
		return nil, err
	}
	return append([]int(nil), bp...), nil
}
