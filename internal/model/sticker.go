package model

type StickerPack struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	BuiltIn     bool      `json:"builtIn"`
	Stickers    []Sticker `json:"stickers,omitempty"`
}

type Sticker struct {
	ID          string   `json:"id"`
	PackID      string   `json:"packId"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	AssetURL    string   `json:"assetUrl"`
	MimeType    string   `json:"mimeType"`
}
