# gslidev

`gslidev` renders a Slidev presentation and synchronizes it to Google Slides as images.

## Usage

```console
gslidev slides.md
gslidev slides.md --presentation-id PRESENTATION_ID
```

If `--presentation-id` is omitted, a new Google Slides presentation is created. If it is supplied, the target presentation's existing slides are replaced.

The title and target IDs can also be stored in Slidev's headmatter:

```yaml
---
title: My presentation
gslidev:
  presentationID: PRESENTATION_ID
  folderID: DRIVE_FOLDER_ID
---
```

The following options are available:

```text
--presentation-id  Google Slides presentation ID
--title            title for a newly created presentation
--folder-id        Drive folder for the presentation and temporary images
--profile          OAuth credentials profile
--chrome-path      path to Chrome or Chromium
--slidev-path      path to the slidev executable
--verbose          enable debug logging
```

## Google OAuth setup

Create a Google Cloud OAuth client of type **Desktop app**, enable Google Slides API and Google Drive API, and save the downloaded credentials as:

```text
${XDG_DATA_HOME:-~/.local/share}/gslidev/credentials.json
```

If `${XDG_DATA_HOME:-~/.local/share}/deck/credentials.json` already exists, it is used as a fallback so an existing `deck` setup can be reused.

The first execution opens the browser for authorization. The refresh token is stored under:

```text
${XDG_STATE_HOME:-~/.local/state}/gslidev/token.json
```

For non-interactive runs, `GSLIDEV_ACCESS_TOKEN` can be supplied instead.

## Media behavior

- Each slide is captured from Slidev and inserted as a full-slide PNG.
- GIF elements are hidden only while capturing, then the original GIF is inserted at its original position.
- Video elements are hidden while capturing and are not inserted; they can be added manually in Google Slides.
- HTML comments at the end of each Slidev slide are copied to the Google Slides speaker notes.

## Development

```console
task check
```
