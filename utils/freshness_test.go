package utils

import (
	"testing"
	"time"

	"github.com/yafyx/baak-api/models"
)

func TestFreshnessHashTracksContentNotFetchTime(t *testing.T) {
	page := models.PublicPage{Title: "Academic news", Text: "first"}
	StampPublicPage(&page, time.Minute)
	hash := page.Metadata.ContentHash
	StampPublicPage(&page, time.Hour)
	if page.Metadata.ContentHash != hash {
		t.Fatal("unchanged content changed hash")
	}
	if page.Metadata.ExpiresAt.Sub(page.Metadata.FetchedAt) != time.Hour {
		t.Fatal("wrong TTL")
	}
	page.Text = "second"
	StampPublicPage(&page, time.Minute)
	if page.Metadata.ContentHash == hash {
		t.Fatal("changed content kept hash")
	}
}
