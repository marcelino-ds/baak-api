package models

import "time"

type SourceMetadata struct {
	FetchedAt   time.Time `json:"fetched_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	ContentHash string    `json:"content_hash"`
}

type NewsArticle struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	PublishedAt string       `json:"published_at"`
	Author      string       `json:"author"`
	Content     string       `json:"content"`
	Attachments []PublicLink `json:"attachments"`
}

type PDFPage struct {
	Number int          `json:"number"`
	Text   string       `json:"text"`
	Tables [][][]string `json:"tables"`
}

type PDFDocument struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Title     string         `json:"title"`
	PageCount int            `json:"page_count"`
	Pages     []PDFPage      `json:"pages"`
	Warnings  []string       `json:"warnings"`
	Metadata  SourceMetadata `json:"metadata"`
}
