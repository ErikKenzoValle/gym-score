package services_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gymScore-backend/internal/services"
)

func TestDetectarImagem_PorMagicBytes(t *testing.T) {
	casos := []struct {
		nome string
		dado []byte
		ext  string
		ok   bool
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0}, ".jpg", true},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0}, ".png", true},
		{"webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), ".webp", true},
		{"riff wave", []byte("RIFF\x10\x00\x00\x00WAVEfmt "), "", false},
		{"gif", []byte("GIF89a"), "", false},
		{"html", []byte("<html></html>"), "", false},
		{"png truncado", []byte{0x89, 'P', 'N', 'G'}, "", false},
		{"vazio", nil, "", false},
	}
	for _, c := range casos {
		ext, ok := services.DetectarImagem(c.dado)
		if ext != c.ext || ok != c.ok {
			t.Errorf("%s: obteve (%q,%v), esperava (%q,%v)", c.nome, ext, ok, c.ext, c.ok)
		}
	}
}

func TestFotoStorageDisco_NomeAleatorioERemocao(t *testing.T) {
	dir := t.TempDir()
	st := services.FotoStorageDisco{Dir: dir}
	p1, err := st.Salvar([]byte{1, 2, 3}, ".png")
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := st.Salvar([]byte{1, 2, 3}, ".png")
	if p1 == p2 || !strings.HasPrefix(p1, "uploads/checkins/") || !strings.HasSuffix(p1, ".png") {
		t.Errorf("caminhos devem ser únicos e dentro de uploads/checkins: %s %s", p1, p2)
	}
	if _, err := os.Stat(filepath.Join(dir, "checkins", filepath.Base(p1))); err != nil {
		t.Errorf("arquivo não foi gravado: %v", err)
	}
	st.Remover(p1)
	if _, err := os.Stat(filepath.Join(dir, "checkins", filepath.Base(p1))); !os.IsNotExist(err) {
		t.Error("arquivo deveria ter sido removido")
	}
	st.Remover("../../etc/passwd") // path traversal: usa só o nome-base, não sai da pasta
}
