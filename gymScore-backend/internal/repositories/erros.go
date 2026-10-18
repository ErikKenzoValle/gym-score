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
