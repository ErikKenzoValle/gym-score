package repositories

import (
	"errors"

	mysqldrv "github.com/go-sql-driver/mysql"
)

// EhDuplicata detecta o erro 1062 (violação de chave única) do MySQL.
func EhDuplicata(err error) bool {
	var me *mysqldrv.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// EhDeadlock detecta o erro 1213 (deadlock; a transação foi desfeita pelo MySQL e pode ser repetida).
// Ocorre raramente entre um crédito (que trava a faixa de usuario_debitos do credor) e uma anulação que
// cria débito para outro usuário na mesma faixa do índice.
func EhDeadlock(err error) bool {
	var me *mysqldrv.MySQLError
	return errors.As(err, &me) && me.Number == 1213
}

// EhViolacaoCheck detecta o erro 3819 (CHECK constraint violada) do MySQL — ex.: CEP/UF fora do formato
// em `academias` (migration 00020). O service traduz para 422 com mensagem clara.
func EhViolacaoCheck(err error) bool {
	var me *mysqldrv.MySQLError
	return errors.As(err, &me) && me.Number == 3819
}
