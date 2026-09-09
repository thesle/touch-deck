package core

// Persisted data models for TouchDeck. Copied verbatim from app.go with JSON
// tags frozen so existing config.json files load unchanged (Requirement 3.2).
// Only the package changes; the field names and JSON tags are byte-for-byte
// identical to the Wails implementation in app.go.

// ButtonConfig represents a button's configuration.
type ButtonConfig struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	BgImage   string `json:"bgImage"`
	BgColor   string `json:"bgColor"`
	FontColor string `json:"fontColor"`
	FontSize  int    `json:"fontSize"`
	Order     int    `json:"order"`
}

// PageConfig represents a page of buttons.
type PageConfig struct {
	PageIndex int            `json:"pageIndex"`
	Buttons   []ButtonConfig `json:"buttons"`
}

// Config represents the application configuration.
type Config struct {
	Rows  int          `json:"rows"`
	Cols  int          `json:"cols"`
	Pages []PageConfig `json:"pages"`
}
