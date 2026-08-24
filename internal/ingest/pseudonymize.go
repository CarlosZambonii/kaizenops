package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// PseudonymizeAuthor transforma um username cru do GitHub num hash estável,
// para que a identidade do contribuidor nunca chegue ao armazenamento.
// Usa HMAC-SHA256 em vez de SHA256 simples: sem o salt, não dá pra montar
// uma tabela arco-íris de usernames do GitHub para reidentificar autores.
func PseudonymizeAuthor(salt, username string) string {
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(username))
	return hex.EncodeToString(mac.Sum(nil))
}
