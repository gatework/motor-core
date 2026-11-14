package source

import (
	"crypto/sha256"
	"encoding/hex"
)

// Sum returns the SHA-256 content identity of the ChangeSet data.
func (c *ChangeSet) Sum() string {
	digest := sha256.Sum256(c.Data)
	return hex.EncodeToString(digest[:])
}
