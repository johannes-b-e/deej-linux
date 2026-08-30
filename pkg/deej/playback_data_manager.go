// Package deej — playback_monitor.go
//
// This module manages metadata and cover art which can be sent to the
// microcontroller for display on a screen. Using MPRIS, we retrieve this
// from whatever is playing right now, check if it changed, package it in
// the correct format, extract the image and convert it to a 230x230 JPEG
// with rounded corners (20px radius). The result is handed off to
// serial.go's SendMetadata/SendCover, which handle the wire protocol.
package deej

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	coverSize        = 230 // target width/height of the cover art sent to the display
	coverRadius      = 25  // corner rounding radius, in pixels
	coverJPEGQuality = 70
)

// PlaybackMonitor polls MPRIS for the currently playing track, detects
// changes, and forwards a ready-to-display metadata + cover art package to
// the microcontroller via SerialIO.
type PlaybackMonitor struct {
	player     *mprisClient
	serial     *SerialIO
	httpClient *http.Client
	logger     *zap.SugaredLogger
	poll       time.Duration

	lastSent playbackFingerprint
}

// playbackFingerprint is a cheap, comparable summary of a track's identity.
// If two fingerprints are equal, we assume nothing meaningful changed and
// skip re-fetching/re-encoding artwork and resending data over serial.
type playbackFingerprint struct {
	title    string
	artist   string
	album    string
	artURL   string
	duration int64
}

func fingerprintFor(meta displayMetadata) playbackFingerprint {
	return playbackFingerprint{
		title:    meta.Title,
		artist:   meta.Artist,
		album:    meta.Album,
		artURL:   meta.ArtURL,
		duration: meta.Duration,
	}
}

// NewPlaybackMonitor creates a PlaybackMonitor that will poll MPRIS every
// `poll` interval and forward changes to sio.
func NewPlaybackMonitor(sio *SerialIO, poll time.Duration, logger *zap.SugaredLogger) (*PlaybackMonitor, error) {
	player, err := newMPRISClient()
	if err != nil {
		return nil, fmt.Errorf("init mpris client: %w", err)
	}

	return &PlaybackMonitor{
		player: player,
		serial: sio,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
		poll:   poll,
	}, nil
}

// Close releases the underlying MPRIS connection.
func (m *PlaybackMonitor) Close() error {
	if m == nil || m.player == nil {
		return nil
	}
	return m.player.Close()
}

// Run polls MPRIS forever (intended to be called via `go m.Run()`), sending
// updated playback data to the microcontroller whenever the track changes.
func (m *PlaybackMonitor) Run() {
	for {
		meta, err := m.player.currentMetadata()
		if err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to read MPRIS metadata", "error", err)
			}
			time.Sleep(m.poll)
			continue
		}

		fp := fingerprintFor(meta)
		if fp == m.lastSent {
			// nothing changed since the last successful send - skip the
			// expensive cover art fetch/resize/encode entirely
			time.Sleep(m.poll)
			continue
		}

		if err := m.serial.SendMetadata(meta.Title, meta.Artist, int(meta.Duration)); err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to send metadata to display", "error", err)
			}
			// don't update lastSent - we'll retry this same track next poll
			time.Sleep(m.poll)
			continue
		}

		cover, err := m.fetchCoverArt(meta.ArtURL)
		if err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to fetch/process cover art", "error", err)
			}
			cover = nil
		}

		if cover != nil {
			if m.logger != nil {
				start := time.Now()
				if err := m.serial.SendCover(cover); err != nil {
					m.logger.Warnw("Failed to send cover art to display", "error", err)
					// metadata already sent successfully - still mark this
					// track as "sent" so we don't keep retrying the whole
					// thing every poll; only the image failed
				} else {
					m.logger.Infof("Sent image data (%.3fKB) in %s", float64(len(cover))/1024.0, time.Since(start))
				}
			}
		}

		m.lastSent = fp
		if m.logger != nil {
			m.logger.Infow("Sent playback data to display", "title", meta.Title, "artist", meta.Artist)
		}

		time.Sleep(m.poll)
	}
}

// fetchCoverArt downloads the artwork at url and returns it as a 230x230
// JPEG with 20px rounded corners, ready for display. Returns (nil, nil) if
// url is empty.
func (m *PlaybackMonitor) fetchCoverArt(url string) ([]byte, error) {
	if url == "" {
		return nil, nil
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

// RunPositionUpdates periodically sends the current playback position and
// mic-mute status to the display, independent of whether the track itself
// has changed. Intended to be started via `go m.RunPositionUpdates()`.
func (m *PlaybackMonitor) RunPositionUpdates(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		position, err := m.player.currentPosition()
		if err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to read MPRIS position", "error", err)
			}
			continue
		}

		micMuted := m.serial.IsMicMuted()
		paused := false
		if m.player != nil {
			if p, err := m.player.IsPaused(); err == nil {
				paused = p
			} else if m.logger != nil {
				m.logger.Debugw("Failed to read playback paused state", "error", err)
			}
		}

		if err := m.serial.SendUpdate(int(position), paused, micMuted); err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to send position update to display", "error", err)
			}
		}
	}
}