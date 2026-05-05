package service

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"chatp2p/internal/model"
)

const (
	stickerAssetBasePath = "sticker_assets"
	stickerAssetMime     = "image/svg+xml"
)

//go:embed sticker_assets/catalog.json sticker_assets/classic-faces/*.svg
var stickerAssets embed.FS

type stickerRecord struct {
	sticker   model.Sticker
	assetPath string
}

type stickerCatalogFile struct {
	Packs []stickerPackFile `json:"packs"`
}

type stickerPackFile struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	BuiltIn     bool          `json:"builtIn"`
	Stickers    []stickerFile `json:"stickers"`
}

type stickerFile struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	AssetFile   string   `json:"assetFile"`
	MimeType    string   `json:"mimeType"`
}

type StickerAsset struct {
	ContentType string
	Body        []byte
}

type StickerService struct {
	packs    []model.StickerPack
	stickers []stickerRecord
	byPack   map[string][]stickerRecord
	byID     map[string]stickerRecord
}

func NewStickerService() *StickerService {
	service, err := loadStickerService()
	if err != nil {
		panic(err)
	}
	return service
}

func loadStickerService() (*StickerService, error) {
	catalogBytes, err := stickerAssets.ReadFile(stickerAssetBasePath + "/catalog.json")
	if err != nil {
		return nil, fmt.Errorf("read sticker catalog: %w", err)
	}

	var catalog stickerCatalogFile
	if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
		return nil, fmt.Errorf("parse sticker catalog: %w", err)
	}

	service := &StickerService{
		packs:    make([]model.StickerPack, 0, len(catalog.Packs)),
		stickers: []stickerRecord{},
		byPack:   map[string][]stickerRecord{},
		byID:     map[string]stickerRecord{},
	}

	for _, packFile := range catalog.Packs {
		packID := strings.TrimSpace(packFile.ID)
		if packID == "" {
			return nil, fmt.Errorf("sticker pack id is required")
		}

		service.packs = append(service.packs, model.StickerPack{
			ID:          packID,
			Name:        strings.TrimSpace(packFile.Name),
			Description: strings.TrimSpace(packFile.Description),
			BuiltIn:     packFile.BuiltIn,
		})

		for _, stickerFile := range packFile.Stickers {
			record, err := newStickerRecord(packID, stickerFile)
			if err != nil {
				return nil, err
			}
			service.stickers = append(service.stickers, record)
			service.byPack[packID] = append(service.byPack[packID], record)
			service.byID[record.sticker.ID] = record
		}
	}

	return service, nil
}

func newStickerRecord(packID string, stickerFile stickerFile) (stickerRecord, error) {
	stickerID := strings.TrimSpace(stickerFile.ID)
	assetFile := strings.Trim(strings.TrimSpace(stickerFile.AssetFile), "/")
	if stickerID == "" || assetFile == "" {
		return stickerRecord{}, fmt.Errorf("sticker id and asset file are required")
	}
	if _, err := stickerAssets.ReadFile(stickerAssetBasePath + "/" + assetFile); err != nil {
		return stickerRecord{}, fmt.Errorf("read sticker asset %s: %w", assetFile, err)
	}

	mimeType := strings.TrimSpace(stickerFile.MimeType)
	if mimeType == "" {
		mimeType = stickerAssetMime
	}

	return stickerRecord{
		sticker: model.Sticker{
			ID:          stickerID,
			PackID:      packID,
			Name:        strings.TrimSpace(stickerFile.Name),
			Description: strings.TrimSpace(stickerFile.Description),
			Keywords:    stickerFile.Keywords,
			AssetURL:    "/api/v1/stickers/" + stickerID + "/asset.svg",
			MimeType:    mimeType,
		},
		assetPath: stickerAssetBasePath + "/" + assetFile,
	}, nil
}

func (s *StickerService) ListPacks() []model.StickerPack {
	packs := make([]model.StickerPack, 0, len(s.packs))
	for _, pack := range s.packs {
		pack.Stickers = stickersFromRecords(s.byPack[pack.ID])
		packs = append(packs, pack)
	}
	return packs
}

func (s *StickerService) FindPack(id string) (model.StickerPack, bool) {
	id = strings.TrimSpace(id)
	for _, pack := range s.packs {
		if pack.ID == id {
			pack.Stickers = stickersFromRecords(s.byPack[pack.ID])
			return pack, true
		}
	}
	return model.StickerPack{}, false
}

func (s *StickerService) ListStickers(packID string) ([]model.Sticker, bool) {
	records, ok := s.byPack[strings.TrimSpace(packID)]
	if !ok {
		return nil, false
	}
	return stickersFromRecords(records), true
}

func (s *StickerService) FindSticker(id string) (model.Sticker, bool) {
	record, ok := s.byID[strings.TrimSpace(id)]
	return record.sticker, ok
}

func (s *StickerService) Asset(id string) (StickerAsset, bool) {
	record, ok := s.byID[strings.TrimSpace(id)]
	if !ok {
		return StickerAsset{}, false
	}
	body, err := stickerAssets.ReadFile(record.assetPath)
	if err != nil {
		return StickerAsset{}, false
	}
	return StickerAsset{
		ContentType: record.sticker.MimeType,
		Body:        body,
	}, true
}

func stickersFromRecords(records []stickerRecord) []model.Sticker {
	stickers := make([]model.Sticker, 0, len(records))
	for _, record := range records {
		stickers = append(stickers, record.sticker)
	}
	return stickers
}
