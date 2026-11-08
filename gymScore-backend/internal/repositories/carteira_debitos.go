package repositories

import (
	"encoding/json"
	"fmt"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// Débitos de usuário na carteira (moderação, Q5). Regras:
//   - todo crédito efetivo (Creditar, depósito confirmado, reembolso de saque) abate os débitos abertos do
//     usuário na MESMA transação do crédito, do mais antigo para o mais novo, até o valor creditado;
//   - débito aberto bloqueia só o saque (checado dentro da transação do saque, com o usuário travado);
//   - ordem de travas em todos os caminhos: linha do usuário ANTES de usuario_debitos (o crédito já
//     travou o usuário no UPDATE do saldo; a anulação trava o usuário antes de criar o débito).

// Ações da linha do tempo gravadas pelo abatimento (ator sistema).
const (
	EventoDebitoAbatido = "debito_abatido"
	EventoDebitoQuitado = "debito_quitado"
)

// ErroDebitoAberto: saque recusado porque o usuário tem débito aberto (valor total devido).
type ErroDebitoAberto struct{ TotalCentavos int64 }

func (e *ErroDebitoAberto) Error() string {
	return fmt.Sprintf("saque bloqueado: débito aberto de %d centavos", e.TotalCentavos)
}

// SaldoTravado lê o saldo com SELECT ... FOR UPDATE (trava a linha do usuário até o fim da transação).
func (r *CarteiraRepository) SaldoTravado(tx *gorm.DB, idUsuario uint) (int64, error) {
	var linhas []int64
	if err := tx.Raw("SELECT saldo_centavos FROM usuarios WHERE id = ? FOR UPDATE", idUsuario).Scan(&linhas).Error; err != nil {
		return 0, err
	}
	if len(linhas) == 0 {
		return 0, fmt.Errorf("usuário %d não encontrado", idUsuario)
	}
	return linhas[0], nil
}

// TravarUsuarios trava as linhas dos usuários em ordem crescente de id (operações com vários saldos
// travam todos de uma vez, na mesma ordem em qualquer caminho, para não haver deadlock entre elas).
func (r *CarteiraRepository) TravarUsuarios(tx *gorm.DB, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	var travados []uint
	return tx.Raw("SELECT id FROM usuarios WHERE id IN ? ORDER BY id FOR UPDATE", ids).Scan(&travados).Error
}

// AbaterDebitosDoCredito aplica o Q5 a um crédito de `valor` que acabou de entrar no saldo do usuário:
// abate os débitos abertos (abatimento_debito no ledger), grava debito_abatido/debito_quitado na linha do
// tempo e as notificações ao devedor — tudo na transação `tx` do crédito (se ela desfizer, nada fica).
func (r *CarteiraRepository) AbaterDebitosDoCredito(tx *gorm.DB, idUsuario uint, valor int64) ([]Abatimento, error) {
	if valor <= 0 {
		return nil, nil
	}
	agora := time.Now().Truncate(time.Millisecond)
	abs, err := NovoDebitoRepository(r.db).AbaterDebitos(tx, r, idUsuario, valor, agora)
	if err != nil || len(abs) == 0 {
		return abs, err
	}
	eventos := NovoModeracaoEventoRepository(r.db)
	for _, a := range abs {
		var restante int64
		if err := tx.Raw("SELECT saldo_restante_centavos FROM usuario_debitos WHERE id = ?", a.IDDebito).Scan(&restante).Error; err != nil {
			return abs, err
		}
		dados, _ := json.Marshal(map[string]any{
			"depois": map[string]any{"abatido_centavos": a.Valor, "saldo_restante_centavos": restante, "id_transacao": a.IDTransacao},
		})
		idDesafio := a.IDDesafio
		acao := EventoDebitoAbatido
		if a.Quitado {
			acao = EventoDebitoQuitado
		}
		if err := eventos.Registrar(tx, &models.ModeracaoEvento{AtorPapel: models.AtorSistema, Acao: acao,
			Entidade: models.EntidadeDebito, IDEntidade: a.IDDebito, IDDesafio: &idDesafio, Dados: dados}); err != nil {
			return abs, err
		}
		n := models.Notificacao{IDUsuario: idUsuario, Tipo: models.NotifDebitoAbatido, Titulo: "Débito abatido",
			Mensagem: fmt.Sprintf("%s de um crédito na sua carteira abateram o débito #%d. Restam %s.",
				ReaisBR(a.Valor), a.IDDebito, ReaisBR(restante))}
		if a.Quitado {
			n.Tipo, n.Titulo = models.NotifDebitoQuitado, "Débito quitado"
			n.Mensagem = fmt.Sprintf("O débito #%d foi quitado (%s abatidos de um crédito). O saque está liberado se não houver outro débito.",
				a.IDDebito, ReaisBR(a.Valor))
		}
		link := "/carteira"
		n.Link = &link
		if err := tx.Create(&n).Error; err != nil {
			return abs, err
		}
	}
	return abs, nil
}

// ReaisBR formata centavos como "R$ 1.234,56" (textos de notificação gravados na camada de dados).
func ReaisBR(c int64) string {
	sinal := ""
	if c < 0 {
		sinal, c = "-", -c
	}
	inteiro, frac := c/100, c%100
	s := fmt.Sprintf("%d", inteiro)
	out := ""
	for i, d := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += "."
		}
		out += string(d)
	}
	return fmt.Sprintf("%sR$ %s,%02d", sinal, out, frac)
}
