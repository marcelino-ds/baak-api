package models

// PublicLink is a link exposed by a public BAAK page.
type PublicLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
	Kind string `json:"kind,omitempty"`
}

// PublicTable is a normalized HTML table from a public BAAK page.
type PublicTable struct {
	Headers []string            `json:"headers"`
	Rows    [][]string          `json:"rows"`
	Records []map[string]string `json:"records"`
}

type PublicOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Pagination struct {
	Page     int  `json:"page"`
	Next     int  `json:"next_page,omitempty"`
	Previous int  `json:"previous_page,omitempty"`
	Total    *int `json:"total,omitempty"`
}

type NewsItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date"`
	URL   string `json:"url"`
}

type PublicSection struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Endpoint    string   `json:"endpoint"`
	Methods     []string `json:"methods"`
	Parameter   string   `json:"parameter,omitempty"`
}

// PublicPage contains public, read-only BAAK content.
type PublicPage struct {
	Section    string                    `json:"section"`
	Source     string                    `json:"source"`
	Path       string                    `json:"path"`
	Title      string                    `json:"title"`
	Text       string                    `json:"text,omitempty"`
	Links      []PublicLink              `json:"links,omitempty"`
	Tables     []PublicTable             `json:"tables,omitempty"`
	Options    map[string][]PublicOption `json:"options,omitempty"`
	News       []NewsItem                `json:"news,omitempty"`
	Pagination Pagination                `json:"pagination"`
}
