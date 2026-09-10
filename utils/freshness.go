package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/yafyx/baak-api/models"
)

func StampPublicPage(page *models.PublicPage, ttl time.Duration) {
	page.Metadata = nil
	content, _ := json.Marshal(page)
	hash := sha256.Sum256(content)
	now := time.Now().UTC()
	page.Metadata = &models.SourceMetadata{FetchedAt: now, ExpiresAt: now.Add(ttl), ContentHash: hex.EncodeToString(hash[:])}
}
