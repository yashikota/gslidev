# gslidev

`gslidev` renders a Slidev presentation and synchronizes it to Google Slides as images.

## Usage

```console
gslidev slides.md
gslidev --presentation-id PRESENTATION_ID slides.md
```

If `--presentation-id` is omitted, a new Google Slides presentation is created. If it is supplied, the target presentation is updated. Existing page IDs are preserved by position; inserting or reordering source slides therefore changes the content associated with those positions. Manual page elements are replaced.

New pages are staged before the existing content is changed. The final replacement is a single atomic Google Slides batch, so a rejected image or note update does not partially erase the original deck. If synchronization fails, temporary pages are removed on a best-effort basis.

Place flags before the input filename.

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
--output           export PNGs, notes, GIFs, and manifest.json locally without contacting Google
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
- Pages and notes come from Slidev's resolved runtime data, including per-slide frontmatter and `src` imports. The last trailing HTML comment is the note, matching Slidev.
- Each page is rendered in Slidev print mode, producing one completed image rather than separate click steps. Fonts, images, Mermaid rendering, and `data-waitfor` elements are awaited before capture.
- Project-local Slidev installations are located in the input directory or its ancestors. A Node entry such as `--slidev-path /path/to/@slidev/cli/bin/slidev.mjs` also works on Windows.

## Preview before syncing

```console
gslidev --output preview slides.md
```

The output contains numbered PNGs and UTF-8 narration text files. GIFs are exported separately with normalized placement coordinates in `manifest.json`, as they are separate objects in Google Slides. Use a fresh directory for each preview; existing files with the same names are overwritten, but unrelated or stale files are not deleted.

For Google Vids, synchronize the deck and select the imported speaker notes as the narration script. Write the spoken form of equations in the notes instead of relying on automatic interpretation of the slide image. Vids conversion is performed in Google Vids, not by this CLI.

## Development

```console
task check
```

The browser integration test is opt-in. Set `GSLIDEV_TEST_SLIDEV` to the Slidev executable or Node entry and `GSLIDEV_TEST_CHROME` to Chromium, then run `go test ./...`. It checks rendered equations, per-slide frontmatter, imported slides, note correspondence, and nonblank page captures without contacting Google.
