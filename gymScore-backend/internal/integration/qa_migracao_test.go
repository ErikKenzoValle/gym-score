//go:build integration

package integration

// Testes do QA (rodada admin): migrations 00013–00019 sobre um banco "legado" (parado na 00012 com
// dados realistas), Down até 00012 e Up de novo, e sondas de causas prováveis de falha num banco
// remoto (dados que violam os CHECK/UNIQUE da 00016, versões fora de ordem no goose_db_version).
// Rodada 3: a moderação da Sprint 7 virou a 00013 e as nossas 00013–00017 passaram a 00014–00018.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/repositories"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func providerQA(t *testing.T, sqlDB *sql.DB) *goose.Provider {
	t.Helper()
	p, err := goose.NewProvider(goose.DialectMySQL, sqlDB, os.DirFS("../../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// bancoQA cria um banco vazio e aplica as migrations até `versao`.
func bancoQA(t *testing.T, sufixo string, versao int64) (*gorm.DB, *sql.DB) {
	t.Helper()
	nome := nomeBanco + "_qa_" + sufixo
	admin, err := gorm.Open(mysql.Open(dsnAdmin), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	admin.Exec("DROP DATABASE IF EXISTS " + nome)
	if err := admin.Exec("CREATE DATABASE " + nome + " CHARACTER SET utf8mb4").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP DATABASE IF EXISTS " + nome) })
	dbm, err := gorm.Open(mysql.Open(config.AplicarFusoDSN(strings.Replace(dsnAdmin, "/?", "/"+nome+"?", 1), time.Local)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := dbm.DB()
	if _, err := providerQA(t, sqlDB).UpTo(context.Background(), versao); err != nil {
		t.Fatalf("UpTo %d: %v", versao, err)
	}
	return dbm, sqlDB
}

func execQA(t *testing.T, d *gorm.DB, q string, args ...any) int64 {
	t.Helper()
	res := d.Exec(q, args...)
	if res.Error != nil {
		t.Fatalf("%s: %v", q, res.Error)
	}
	var id int64
	d.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	return id
}

func usuarioLegado(t *testing.T, d *gorm.DB, perfil string) uint {
	n := seq.Add(1)
	return uint(execQA(t, d, `INSERT INTO usuarios (nome,sobrenome,cpf,email,senha,perfil,criado_em,atualizado_em)
		VALUES (?, 'Legado', ?, ?, 'x', ?, NOW(3), NOW(3))`, fmt.Sprintf("QA%d", n), cpfValido(),
		fmt.Sprintf("qa.leg%d_%d@gymscore.test", n, time.Now().UnixNano()), perfil))
}

func lanc(t *testing.T, d *gorm.DB, u uint, tipo string, v int64, desafio *uint, status, desc string) uint {
	return uint(execQA(t, d, `INSERT INTO transacoes (id_usuario,tipo,valor_centavos,id_desafio,status,descricao,created_at,updated_at)
		VALUES (?,?,?,?,?,?,NOW(3),NOW(3))`, u, tipo, v, desafio, status, desc))
}

// sincronizarSaldos põe saldo_centavos = soma efetiva do ledger (o estado que a aplicação mantém).
func sincronizarSaldos(t *testing.T, d *gorm.DB) {
	execQA(t, d, `UPDATE usuarios u SET saldo_centavos = (SELECT COALESCE(SUM(valor_centavos),0) FROM transacoes t
		WHERE t.id_usuario = u.id AND (t.status IN ('received','concluido') OR (t.tipo='saque' AND t.status='pending')))`)
}

func conferirInvariante(t *testing.T, d *gorm.DB, etapa string) {
	t.Helper()
	var div []struct {
		ID    uint
		Saldo int64
		Soma  int64
	}
	d.Raw(`SELECT u.id, u.saldo_centavos AS saldo, COALESCE((SELECT SUM(valor_centavos) FROM transacoes t
		WHERE t.id_usuario = u.id AND (t.status IN ('received','concluido') OR (t.tipo='saque' AND t.status='pending'))),0) AS soma
		FROM usuarios u HAVING saldo <> soma`).Scan(&div)
	if len(div) > 0 {
		t.Errorf("%s: saldo != ledger: %+v", etapa, div)
	}
	var s1, s2 int64
	d.Raw("SELECT COALESCE(SUM(saldo_centavos),0) FROM usuarios").Scan(&s1)
	d.Raw("SELECT COALESCE(SUM(valor_centavos),0) FROM transacoes WHERE status IN ('received','concluido') OR (tipo='saque' AND status='pending')").Scan(&s2)
	if s1 != s2 {
		t.Errorf("%s: Σ saldos %d != Σ ledger %d", etapa, s1, s2)
	}
}

type legadoQA struct {
	academia, instrutor, x, y       uint
	dAberto, dAndamento, dEncerrado uint
	saqueConcl, saquePend, tarifaC  uint
	tarifaP, tarifaTextoDiferente   uint
}

func semearLegado(t *testing.T, d *gorm.DB) legadoQA {
	var L legadoQA
	L.academia = usuarioLegado(t, d, "academia")
	L.instrutor = usuarioLegado(t, d, "instrutor")
	L.x = usuarioLegado(t, d, "atleta")
	L.y = usuarioLegado(t, d, "atleta")
	idAc := uint(execQA(t, d, `INSERT INTO academias (id_usuario,razao_social,nome_fantasia,cnpj,criado_em,atualizado_em) VALUES (?,?,?,?,NOW(3),NOW(3))`,
		L.academia, "Acad Legada", "Acad Legada", cnpjUnico()))
	idIn := uint(execQA(t, d, `INSERT INTO instrutores (id_usuario,cref,id_academia,criado_em,atualizado_em) VALUES (?,?,?,NOW(3),NOW(3))`,
		L.instrutor, "123-G/SP", idAc))
	for _, u := range []uint{L.academia, L.x, L.y} {
		lanc(t, d, u, "deposito", 10000, nil, "received", "")
	}
	lanc(t, d, L.y, "deposito", 5000, nil, "pending", "")
	lanc(t, d, L.y, "deposito", 3000, nil, "failed", "")
	lanc(t, d, L.academia, "ajuste", -150, nil, "received", "Saldo migrado")
	// saques no formato da 00012 (tarifa ligada por texto)
	L.saqueConcl = lanc(t, d, L.x, "saque", -1000, nil, "concluido", "Saque PIX")
	L.tarifaC = lanc(t, d, L.x, "tarifa_saque", -200, nil, "concluido", fmt.Sprintf("Tarifa do saque #%d", L.saqueConcl))
	L.saquePend = lanc(t, d, L.x, "saque", -500, nil, "pending", "Saque PIX")
	L.tarifaP = lanc(t, d, L.x, "tarifa_saque", -200, nil, "concluido", fmt.Sprintf("Tarifa do saque #%d", L.saquePend))
	// saque falhado com tarifa devolvida (como o FalharSaque da 00012 fazia)
	sf := lanc(t, d, L.y, "saque", -700, nil, "failed", "Saque PIX")
	lanc(t, d, L.y, "tarifa_saque", -200, nil, "refunded", fmt.Sprintf("Tarifa do saque #%d", sf))
	// tarifa com texto fora do padrão (ajuste manual): deve ficar sem origem, sem quebrar a migration
	L.tarifaTextoDiferente = lanc(t, d, L.y, "tarifa_saque", -200, nil, "concluido", fmt.Sprintf("Tarifa do saque #%d (ajuste manual)", sf))

	novoDesafio := func(status string) uint {
		return uint(execQA(t, d, `INSERT INTO desafios (titulo,categoria,valor_entrada_centavos,data_evento,status,vagas,posicoes_podio,id_criador,id_academia,id_instrutor,criado_em,atualizado_em)
			VALUES (?, 'geral', 1000, ?, ?, 10, 3, ?, ?, ?, NOW(3), NOW(3))`, "Legado "+status, time.Now().Add(72*time.Hour), status, L.academia, idAc, idIn))
	}
	inscrever := func(dd, u uint, entradas int) {
		execQA(t, d, `INSERT INTO desafio_participantes (id_desafio,id_usuario,criado_em) VALUES (?,?,NOW(3))`, dd, u)
		for i := 0; i < entradas; i++ {
			lanc(t, d, u, "entrada_desafio", -1000, &dd, "concluido", "Entrada")
			if i < entradas-1 {
				lanc(t, d, u, "estorno_desafio", 1000, &dd, "concluido", "Reembolso (saída)")
			}
		}
	}
	L.dAberto = novoDesafio("aberto")
	inscrever(L.dAberto, L.academia, 1) // N1: a academia inscrita no próprio desafio
	inscrever(L.dAberto, L.x, 1)
	L.dAndamento = novoDesafio("em_andamento")
	inscrever(L.dAndamento, L.academia, 2) // entrou, saiu, entrou de novo
	inscrever(L.dAndamento, L.y, 1)
	L.dEncerrado = novoDesafio("encerrado")
	inscrever(L.dEncerrado, L.academia, 1)
	inscrever(L.dEncerrado, L.x, 1)
	execQA(t, d, `UPDATE desafio_participantes SET premio_centavos=2000, vencedor=1 WHERE id_desafio=? AND id_usuario=?`, L.dEncerrado, L.x)
	execQA(t, d, `INSERT INTO podio_desafios (id_desafio,posicao,id_participante,premio_centavos,criado_em) VALUES (?,1,?,2000,NOW(3))`, L.dEncerrado, L.x)
	lanc(t, d, L.x, "premio_desafio", 2000, &L.dEncerrado, "concluido", "Prêmio")
	sincronizarSaldos(t, d)
	return L
}

func TestQA_MigracaoLegadoAte19DownEUpDeNovo(t *testing.T) {
	d, sqlDB := bancoQA(t, "legado", 12)
	L := semearLegado(t, d)
	conferirInvariante(t, d, "antes")
	var saldoAcadAntes int64
	d.Raw("SELECT saldo_centavos FROM usuarios WHERE id=?", L.academia).Scan(&saldoAcadAntes)

	p := providerQA(t, sqlDB)
	ctx := context.Background()
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up 13..19 sobre legado: %v", err)
	}
	verificar := func(etapa string) {
		t.Helper()
		conferirInvariante(t, d, etapa)
		var n int64
		d.Raw("SELECT COUNT(*) FROM desafio_participantes WHERE id_usuario=? AND id_desafio IN ?", L.academia, []uint{L.dAberto, L.dAndamento}).Scan(&n)
		if n != 0 {
			t.Errorf("%s: academia ainda inscrita em desafio ativo (%d)", etapa, n)
		}
		d.Raw("SELECT COUNT(*) FROM desafio_participantes WHERE id_usuario=? AND id_desafio=?", L.academia, L.dEncerrado).Scan(&n)
		if n != 1 {
			t.Errorf("%s: inscrição em desafio encerrado não deveria ser tocada (%d)", etapa, n)
		}
		var est []struct {
			IDDesafio uint
			Soma      int64
			N         int64
		}
		d.Raw(`SELECT id_desafio, SUM(valor_centavos) soma, COUNT(*) n FROM transacoes WHERE id_usuario=? AND tipo='estorno_desafio'
			AND descricao LIKE '%migração%' GROUP BY id_desafio ORDER BY id_desafio`, L.academia).Scan(&est)
		if len(est) != 2 || est[0].Soma != 1000 || est[1].Soma != 1000 || est[0].N != 1 || est[1].N != 1 {
			t.Errorf("%s: estornos N1 da academia: %+v", etapa, est)
		}
		var saldoAc int64
		d.Raw("SELECT saldo_centavos FROM usuarios WHERE id=?", L.academia).Scan(&saldoAc)
		if saldoAc != saldoAcadAntes+2000 {
			t.Errorf("%s: saldo da academia %d, esperado %d", etapa, saldoAc, saldoAcadAntes+2000)
		}
		// atletas continuam inscritos
		d.Raw("SELECT COUNT(*) FROM desafio_participantes WHERE id_usuario IN ? AND id_desafio IN ?", []uint{L.x, L.y}, []uint{L.dAberto, L.dAndamento}).Scan(&n)
		if n != 2 {
			t.Errorf("%s: atletas inscritos: %d", etapa, n)
		}
		var origem sql.NullInt64
		d.Raw("SELECT id_transacao_origem FROM transacoes WHERE id=?", L.tarifaC).Scan(&origem)
		if !origem.Valid || uint(origem.Int64) != L.saqueConcl {
			t.Errorf("%s: backfill tarifa concluída: %+v", etapa, origem)
		}
		d.Raw("SELECT id_transacao_origem FROM transacoes WHERE id=?", L.tarifaP).Scan(&origem)
		if !origem.Valid || uint(origem.Int64) != L.saquePend {
			t.Errorf("%s: backfill tarifa pendente: %+v", etapa, origem)
		}
		d.Raw("SELECT id_transacao_origem FROM transacoes WHERE id=?", L.tarifaTextoDiferente).Scan(&origem)
		if origem.Valid {
			t.Errorf("%s: tarifa com texto diferente não deveria ganhar origem: %v", etapa, origem.Int64)
		}
		// academias/instrutores existentes viram aprovada
		var st string
		d.Raw("SELECT status FROM academias WHERE id_usuario=?", L.academia).Scan(&st)
		if st != "aprovada" {
			t.Errorf("%s: academia legada com status %q", etapa, st)
		}
		// FalharSaque pela coluna devolve valor + tarifa do saque pendente legado
	}
	verificar("após Up")

	// Down até 12 e Up de novo: mesmo estado, sem estorno duplicado
	if _, err := p.DownTo(ctx, 12); err != nil {
		t.Fatalf("DownTo 12: %v", err)
	}
	conferirInvariante(t, d, "após Down")
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up de novo: %v", err)
	}
	verificar("após Down+Up")

	// reaplicar sobre o schema já migrado (versões apagadas do goose_db_version)
	execQA(t, d, "DELETE FROM goose_db_version WHERE version_id >= 13")
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("reaplicar 13..19 sobre schema migrado: %v", err)
	}
	verificar("após reaplicar")

	// o reembolso pela coluna funciona para o saque pendente legado
	cr := repositories.NovoCarteiraRepository(d)
	var saldoX int64
	d.Raw("SELECT saldo_centavos FROM usuarios WHERE id=?", L.x).Scan(&saldoX)
	if ok, err := cr.FalharSaque(L.saquePend); err != nil || !ok {
		t.Fatalf("FalharSaque legado: %v %v", ok, err)
	}
	var saldoX2 int64
	d.Raw("SELECT saldo_centavos FROM usuarios WHERE id=?", L.x).Scan(&saldoX2)
	if saldoX2 != saldoX+700 {
		t.Errorf("FalharSaque legado devolveu %d (esperado 700 = 500 + tarifa 200)", saldoX2-saldoX)
	}
	conferirInvariante(t, d, "após FalharSaque")
}

// Sondas: dados legados que fazem a 00016 abortar, e versões fora de ordem.
func TestQA_MigracaoSondasDeFalha(t *testing.T) {
	casos := []struct {
		nome    string
		semear  func(t *testing.T, d *gorm.DB)
		esperar string // trecho do erro; "" = deve passar
	}{
		{"deposito_zero", func(t *testing.T, d *gorm.DB) {
			u := usuarioLegado(t, d, "atleta")
			lanc(t, d, u, "deposito", 0, nil, "failed", "")
		}, "sinal incoerente"},
		{"premio_duplicado", func(t *testing.T, d *gorm.DB) {
			L := semearLegado(t, d)
			lanc(t, d, L.x, "premio_desafio", 2000, &L.dEncerrado, "concluido", "Prêmio (repetido)")
			sincronizarSaldos(t, d)
		}, "duplicado"},
		{"estorno_zero", func(t *testing.T, d *gorm.DB) {
			u := usuarioLegado(t, d, "atleta")
			lanc(t, d, u, "estorno_desafio", 0, nil, "concluido", "")
		}, "sinal incoerente"},
		{"tipo_vazio_sql_mode_nao_estrito", func(t *testing.T, d *gorm.DB) {
			u := usuarioLegado(t, d, "atleta")
			// sessão sem modo estrito (comum em RDS antigo): ENUM inválido vira ''
			d.Transaction(func(tx *gorm.DB) error {
				tx.Exec("SET SESSION sql_mode=''")
				tx.Exec(`INSERT INTO transacoes (id_usuario,tipo,valor_centavos,status,created_at,updated_at) VALUES (?, 'bonus', 100, 'received', NOW(3), NOW(3))`, u)
				tx.Exec("SET SESSION sql_mode=DEFAULT")
				return nil
			})
		}, "sinal incoerente"},
		{"tarifa_texto_com_sufixo", func(t *testing.T, d *gorm.DB) {
			u := usuarioLegado(t, d, "atleta")
			s := lanc(t, d, u, "saque", -1000, nil, "concluido", "")
			lanc(t, d, u, "tarifa_saque", -200, nil, "concluido", fmt.Sprintf("Tarifa do saque #%d - estornada", s))
			lanc(t, d, u, "tarifa_saque", -200, nil, "concluido", "Tarifa do saque #abc")
		}, ""},
		{"tarifa_duplicada_mesmo_saque", func(t *testing.T, d *gorm.DB) {
			u := usuarioLegado(t, d, "atleta")
			s := lanc(t, d, u, "saque", -1000, nil, "concluido", "")
			lanc(t, d, u, "tarifa_saque", -200, nil, "concluido", fmt.Sprintf("Tarifa do saque #%d", s))
			lanc(t, d, u, "tarifa_saque", -200, nil, "concluido", fmt.Sprintf("Tarifa do saque #%d", s))
		}, "tarifa_saque duplicada"}, // R3: SIGNAL 1644 da pré-checagem, não mais o 1062 cru
	}
	for i, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d, sqlDB := bancoQA(t, fmt.Sprintf("s%d", i), 12)
			c.semear(t, d)
			_, err := providerQA(t, sqlDB).Up(context.Background())
			switch {
			case c.esperar == "" && err != nil:
				t.Errorf("deveria passar: %v", err)
			case c.esperar != "" && (err == nil || !strings.Contains(err.Error(), c.esperar)):
				t.Errorf("esperava erro com %q, veio: %v", c.esperar, err)
			default:
				t.Logf("resultado: %v", err)
			}
		})
	}

	t.Run("versao_fora_de_ordem", func(t *testing.T) {
		// banco remoto que recebeu uma migration de número maior (outra branch) antes destas
		d, sqlDB := bancoQA(t, "ooo", 12)
		execQA(t, d, "INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (20, 1, NOW())")
		_, err := providerQA(t, sqlDB).Up(context.Background())
		t.Logf("resultado: %v", err)
		if err == nil {
			t.Errorf("esperava erro de migrations fora de ordem")
		}
	})
	t.Run("versao_13_ja_aplicada_de_outra_branch", func(t *testing.T) {
		// 00013 marcada como aplicada mas sem o conteúdo da moderação (outra branch): a 00019 altera as
		// tabelas da moderação e precisa falhar alto em vez de seguir com schema incompleto.
		d, sqlDB := bancoQA(t, "v13", 12)
		execQA(t, d, "INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (13, 1, NOW())")
		_, err := providerQA(t, sqlDB).Up(context.Background())
		t.Logf("resultado: %v", err)
		if err == nil {
			t.Errorf("esperava erro (00019 referencia as tabelas da moderação)")
		}
	})

	t.Run("duas_instancias_migrando_ao_mesmo_tempo", func(t *testing.T) {
		// provider goose cru, SEM locker: documenta o 1304/1305 que motivou o R2. database.Migrar usa a
		// trava (ver TestMigrar_InstanciasConcorrentesComTrava em ajustes_posqa_test.go).
		_, sqlDB := bancoQA(t, "par", 12)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() { _, err := providerQA(t, sqlDB).Up(context.Background()); errs <- err }()
		}
		e1, e2 := <-errs, <-errs
		t.Logf("instância A: %v", e1)
		t.Logf("instância B: %v", e2)
	})
}
