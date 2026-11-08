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
