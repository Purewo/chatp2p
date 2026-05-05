# Stickers

Sticker APIs expose a server-managed catalog that chat clients can render in a sticker picker.

## Built-In Pack

The current backend ships one built-in pack:

- `classic-faces`: a little yellow face pack backed by OpenMoji SVG assets.

The pack is inspired by classic IM sticker conventions, but it does not copy QQ artwork or branded character designs. Assets are downloaded from the OpenMoji project and stored under `internal/service/sticker_assets/`.

## List Packs

Use `GET /api/v1/sticker-packs` to load all sticker packs:

```json
{
  "items": [
    {
      "id": "classic-faces",
      "name": "Little Yellow Faces",
      "builtIn": true,
      "stickers": []
    }
  ]
}
```

## List Stickers

Use `GET /api/v1/sticker-packs/{packId}/stickers` to load stickers for a single pack.

Each sticker includes:

- `id`: stable sticker id used by messages.
- `assetUrl`: app-relative SVG asset URL.
- `mimeType`: currently `image/svg+xml`.
- `keywords`: search terms for picker filtering.

## Asset Source

The built-in SVG assets come from OpenMoji:

- Project: https://openmoji.org/
- Source repository: https://github.com/hfg-gmuend/openmoji
- License: CC BY-SA 4.0, copied at `third_party/openmoji/LICENSE.txt`

## Send Sticker Message

Use the normal message endpoint with `type: "sticker"` and `body` set to the sticker id:

```json
{
  "type": "sticker",
  "body": "classic-smile"
}
```

The response and WebSocket event use the normal `Message` shape. Frontend clients should render `body` as a sticker id when `type` is `sticker`.
