// Package database embute as migrations goose e as executa no boot da aplicação.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var arquivos embed.FS

// Migrar aplica todas as migrations pendentes (goose "up") usando os SQLs embutidos.
func Migrar(ctx context.Context, db *sql.DB) error {
	return migrar(ctx, db, -1)
}

// MigrarAte aplica as migrations até a versão informada (usado para testar a conversão de
// dados legados: baseline -> dados antigos -> demais migrations).
func MigrarAte(ctx context.Context, db *sql.DB, versao int64) error {
	return migrar(ctx, db, versao)
}

func migrar(ctx context.Context, db *sql.DB, ate int64) error {
	sub, err := fs.Sub(arquivos, "migrations")
	if err != nil {
		return fmt.Errorf("migrations embutidas: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectMySQL, db, sub)
	if err != nil {
		return fmt.Errorf("criar provider goose: %w", err)
	}
	var errUp error
	if ate >= 0 {
		_, errUp = provider.UpTo(ctx, ate)
	} else {
		_, errUp = provider.Up(ctx)
	}
	if err := errUp; err != nil {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}
