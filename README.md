# Scrub

Strips metadata from images on Windows.

Right-click a photo in Explorer, pick "Remove Metadata with Scrub", and you get a cleaned copy next to the original. `IMG_1234.jpg` becomes `Scrub-IMG_1234.jpg`. The original is left alone.

## Install

Download `scrub.exe` from the latest GitHub Actions build or release, then:

```powershell
.\scrub.exe --install
```

That copies it to `%LOCALAPPDATA%\Scrub\scrub.exe` and adds the Explorer menu for your user. No admin needed.

## Use

Select one or more images in Explorer, right-click, choose "Remove Metadata with Scrub".

Or from a terminal:

```powershell
.\scrub.exe image1.jpg image2.png image3.webp
```

## Uninstall

```powershell
.\scrub.exe --uninstall
```

That removes the Explorer menu. Delete `%LOCALAPPDATA%\Scrub\scrub.exe` if you also want the binary gone.

## Formats

JPEG, PNG, WebP, and GIF drop metadata without re-encoding pixels. JPEG loses APP segments and comments (EXIF, XMP, IPTC, GPS, thumbnails). PNG keeps critical chunks plus `tRNS` and APNG frames. WebP drops EXIF, XMP, and ICCP. GIF drops comments and XMP.

BMP, TIFF, and HEIC/HEIF are rewritten with Windows Imaging. HEIC only works if Windows has a codec for it.

Unrecognized files are skipped with an error. An existing `Scrub-*` file with the same name is overwritten.

## Build

Windows, Go 1.22 or newer:

```powershell
go build -trimpath -ldflags "-s -w" -o scrub.exe .
```
