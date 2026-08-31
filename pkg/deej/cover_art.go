// Package deej — cover_art.go
//
// Pure image-processing pipeline: fetching/loading source art, cropping to
// square, resizing, rounding corners, and JPEG-encoding to the display's
// expected 230x230 format. Nothing here touches MPRIS or serial directly.
package deej

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"
)

const (
	coverSize        = 230 // target width/height of the cover art sent to the display
	coverRadius      = 25  // corner rounding radius, in pixels
	coverJPEGQuality = 70

	defaultCoversDir = "./default_covers"
)

// localCoverPath returns the placeholder cover path for a given MPRIS
// selection source, or "" if the source should use real fetched artwork.
func localCoverPath(source string) string {
	switch source {
	case "youtube":
		return defaultCoversDir + "/youtube.jpg"
	case "twitch":
		return defaultCoversDir + "/twitch.jpg"
	case "firefox":
		return defaultCoversDir + "/firefox.jpg"
	default:
		return "" // feishin, other -> use real artwork
	}
}

// resolveCoverArt returns display-ready cover art for the given metadata:
// a local placeholder for known Firefox sources (cached after first load),
// or freshly fetched/processed artwork otherwise.
func (m *PlaybackMonitor) resolveCoverArt(meta displayMetadata) ([]byte, error) {
	path := localCoverPath(meta.Source)
	if path == "" {
		return m.fetchCoverArt(meta.ArtURL)
	}

	if cached, ok := m.localCoverCache[path]; ok {
		return cached, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open local cover %s: %w", path, err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode local cover %s: %w", path, err)
	}

	encoded, err := processCoverImage(src)
	if err != nil {
		return nil, err
	}

	m.localCoverCache[path] = encoded
	return encoded, nil
}

// fetchCoverArt downloads the artwork at url and returns it as a 230x230
// JPEG with rounded corners, ready for display. Returns (nil, nil) if url
// is empty.
func (m *PlaybackMonitor) fetchCoverArt(url string) ([]byte, error) {
	if url == "" {
		return nil, nil
	}

	// Support data: URIs (e.g. Jellyfin's `data:image/jpeg;base64,...`)
	if strings.HasPrefix(url, "data:") {
		comma := strings.Index(url, ",")
		if comma < 0 || comma+1 >= len(url) {
			return nil, fmt.Errorf("invalid data URI for cover art")
		}
		payload := url[comma+1:]
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode data URI: %w", err)
		}
		src, _, err := image.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("decode cover art: %w", err)
		}
		return processCoverImage(src)
	}

	resp, err := m.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch cover art: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch cover art: unexpected status %d", resp.StatusCode)
	}

	src, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode cover art: %w", err)
	}

	return processCoverImage(src)
}

// processCoverImage runs the shared crop/resize/round/encode pipeline used
// by both fetched and local placeholder covers.
func processCoverImage(src image.Image) ([]byte, error) {
	squared := cropToSquare(src)
	resized := resizeBilinear(squared, coverSize, coverSize)
	rounded := roundCorners(resized, coverRadius)

	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, rounded, &jpeg.Options{Quality: coverJPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode cover art: %w", err)
	}
	return buf.Bytes(), nil
}

// cropToSquare center-crops img to a square, using the smaller of its two
// dimensions as the side length.
func cropToSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	side := w
	if h < side {
		side = h
	}

	offsetX := bounds.Min.X + (w-side)/2
	offsetY := bounds.Min.Y + (h-side)/2

	cropRect := image.Rect(offsetX, offsetY, offsetX+side, offsetY+side)

	cropped := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(cropped, cropped.Bounds(), img, cropRect.Min, draw.Src)

	return cropped
}

// resizeBilinear resizes src to exactly w x h using bilinear interpolation,
// matching the "Linear" interpolation mode in tools like GIMP.
func resizeBilinear(src image.Image, w, h int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		fy := float64(y) * float64(srcH-1) / float64(h-1)
		y0 := int(fy)
		y1 := y0 + 1
		if y1 >= srcH {
			y1 = srcH - 1
		}
		wy := fy - float64(y0)

		for x := 0; x < w; x++ {
			fx := float64(x) * float64(srcW-1) / float64(w-1)
			x0 := int(fx)
			x1 := x0 + 1
			if x1 >= srcW {
				x1 = srcW - 1
			}
			wx := fx - float64(x0)

			c00 := color.RGBAModel.Convert(src.At(bounds.Min.X+x0, bounds.Min.Y+y0)).(color.RGBA)
			c10 := color.RGBAModel.Convert(src.At(bounds.Min.X+x1, bounds.Min.Y+y0)).(color.RGBA)
			c01 := color.RGBAModel.Convert(src.At(bounds.Min.X+x0, bounds.Min.Y+y1)).(color.RGBA)
			c11 := color.RGBAModel.Convert(src.At(bounds.Min.X+x1, bounds.Min.Y+y1)).(color.RGBA)

			r := bilerp(float64(c00.R), float64(c10.R), float64(c01.R), float64(c11.R), wx, wy)
			g := bilerp(float64(c00.G), float64(c10.G), float64(c01.G), float64(c11.G), wx, wy)
			b := bilerp(float64(c00.B), float64(c10.B), float64(c01.B), float64(c11.B), wx, wy)

			dst.Set(x, y, color.RGBA{
				R: uint8(r + 0.5),
				G: uint8(g + 0.5),
				B: uint8(b + 0.5),
				A: 255,
			})
		}
	}

	return dst
}

// bilerp performs bilinear interpolation between four corner values.
func bilerp(v00, v10, v01, v11, wx, wy float64) float64 {
	top := v00*(1-wx) + v10*wx
	bottom := v01*(1-wx) + v11*wx
	return top*(1-wy) + bottom*wy
}

// roundCorners composites src onto a solid black background using a
// rounded-rect alpha mask, so the cut corners show through as black -
// matching the microcontroller display's black background.
func roundCorners(src image.Image, radius int) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	mask := roundedMask(w, h, radius)

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.DrawMask(out, out.Bounds(), src, bounds.Min, mask, image.Point{}, draw.Over)

	return out
}

// roundedMask returns an alpha mask: opaque inside the rounded-rect area,
// transparent in the four corner cutouts.
func roundedMask(w, h, radius int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			inside := true
			checkCorner := false
			cx, cy := 0, 0

			switch {
			case x < radius && y < radius:
				cx, cy, checkCorner = radius, radius, true
			case x >= w-radius && y < radius:
				cx, cy, checkCorner = w-radius-1, radius, true
			case x < radius && y >= h-radius:
				cx, cy, checkCorner = radius, h-radius-1, true
			case x >= w-radius && y >= h-radius:
				cx, cy, checkCorner = w-radius-1, h-radius-1, true
			}

			if checkCorner {
				dx := float64(x - cx)
				dy := float64(y - cy)
				if dx*dx+dy*dy > float64(radius*radius) {
					inside = false
				}
			}

			if inside {
				mask.SetAlpha(x, y, color.Alpha{A: 255})
			} else {
				mask.SetAlpha(x, y, color.Alpha{A: 0})
			}
		}
	}

	return mask
}