package services

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// FotoStorage guarda as fotos de check-in. Salvar devolve o caminho relativo
// ("uploads/checkins/<nome>") que vai para o banco e vira URL pública ("/"+caminho).
type FotoStorage interface {
	Salvar(dados []byte, ext string) (string, error)
	Remover(caminho string)
}

// FotoStorageDisco grava em Dir/checkins/ com nome aleatório (nunca usa o nome enviado pelo cliente).
type FotoStorageDisco struct{ Dir string }

// Salvar grava os bytes (já validados) com nome aleatório de 128 bits.
func (f FotoStorageDisco) Salvar(dados []byte, ext string) (string, error) {
	dir := filepath.Join(f.Dir, "checkins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("criar diretório de uploads: %w", err)
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nome := hex.EncodeToString(b) + ext
	if err := os.WriteFile(filepath.Join(dir, nome), dados, 0o644); err != nil {
		return "", fmt.Errorf("gravar foto: %w", err)
	}
	return "uploads/checkins/" + nome, nil
}

// Remover apaga uma foto previamente salva (usado em rollback); só aceita caminhos de checkins.
func (f FotoStorageDisco) Remover(caminho string) {
	nome := filepath.Base(caminho)
	if err := os.Remove(filepath.Join(f.Dir, "checkins", nome)); err != nil && !os.IsNotExist(err) {
		log.Printf("[UPLOAD] falha ao remover %s: %v", nome, err)
	}
}

// DetectarImagem identifica JPG, PNG e WEBP pelos magic bytes (ignora nome/Content-Type
// enviados pelo cliente) e devolve a extensão canônica.
func DetectarImagem(b []byte) (string, bool) {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return ".jpg", true
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return ".png", true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return ".webp", true
	}
	return "", false
}
