# Scrub

Scrub is a lightweight Windows shell utility for removing metadata from image files.

It is not a desktop app and has no background service, tray process, telemetry, cloud features, automatic updates, or configuration UI. Install it once, right-click image files in File Explorer, choose **Remove Metadata with Scrub**, and Scrub writes cleaned copies beside the originals.

## Install

Download `scrub.exe` from the latest GitHub Actions build or release, then run:

```powershell
.\scrub.exe --install
```

This copies Scrub to:

```text
%LOCALAPPDATA%\Scrub\scrub.exe
```

and registers the Explorer context menu entry:

```text
Remove Metadata with Scrub
```

No administrator rights are required because the registry entry is installed for the current user only.

## Use

In File Explorer:

1. Select one or more image files.
2. Right-click the selection.
3. Choose **Remove Metadata with Scrub**.

Scrub creates cleaned files in the same folder:

```text
IMG_1234.jpg -> Scrub-IMG_1234.jpg
```

Command-line use also works:

```powershell
.\scrub.exe image1.jpg image2.png image3.webp
```

## Uninstall

```powershell
.\scrub.exe --uninstall
```

This removes the Explorer context menu entry. You can then delete `%LOCALAPPDATA%\Scrub\scrub.exe` if desired.

## Format Support

| Format | Behavior |
| --- | --- |
| JPEG | Losslessly removes APP metadata and comments, including EXIF, XMP, IPTC, GPS, thumbnails, and camera metadata containers. |
| PNG | Removes nonessential metadata chunks while preserving critical and pixel-relevant chunks. |
| WebP | Losslessly removes EXIF, XMP, and ICCP chunks. |
| GIF | Removes comments and XMP application extensions while preserving image and animation data. |
| BMP | Uses Windows imaging fallback to write a clean copy. |
| TIFF | Uses Windows imaging fallback to write a clean copy. |
| HEIC/HEIF | Attempts Windows imaging fallback when Windows has compatible codecs installed. |

Unsupported or unrecognized formats are skipped with an error message. Existing `Scrub-*` output files with the same name are replaced.

## Build Locally

Requirements:

- Windows
- Go 1.22 or newer

Build:

```powershell
go test ./...
go build -trimpath -ldflags "-s -w" -o scrub.exe .
```