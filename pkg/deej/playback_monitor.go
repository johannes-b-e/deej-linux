// Package deej — playback_monitor.go
//
// Orchestration layer: polls MPRIS for the currently playing track and
// current playback position, detects changes, and forwards ready-to-display
// metadata + cover art + position updates to the microcontroller via
// SerialIO. Player-selection policy lives in mpris_selection.go, image
// processing lives in cover_art.go.
package deej

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
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

	lastSent        playbackFingerprint
	localCoverCache map[string][]byte
}

// playbackFingerprint is a cheap, comparable summary of a track's identity.
// If two fingerprints are equal, we assume nothing meaningful changed and
// skip re-fetching/re-encoding artwork and resending data over serial.
type playbackFingerprint struct {
	title    string
	artist   string
	album    string
	artURL   string
	trackURL string
	duration int64
	source   string
}

func fingerprintFor(meta displayMetadata) playbackFingerprint {
	return playbackFingerprint{
		title:    meta.Title,
		artist:   meta.Artist,
		album:    meta.Album,
		artURL:   meta.ArtURL,
		trackURL: meta.TrackURL,
		duration: meta.Duration,
		source:   meta.Source,
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
		logger:          logger,
		poll:            poll,
		localCoverCache: make(map[string][]byte),
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

		cover, err := m.resolveCoverArt(meta)
		if err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to fetch/process cover art", "error", err)
			}
			cover = nil
		}

		if cover != nil {
			start := time.Now()
			if err := m.serial.SendCover(cover); err != nil {
				if m.logger != nil {
					m.logger.Warnw("Failed to send cover art to display", "error", err)
				}
				// metadata already sent successfully - still mark this
				// track as "sent" so we don't keep retrying the whole
				// thing every poll; only the image failed
			} else if m.logger != nil {
				m.logger.Infof("Sent image data (%.3fKB) in %s", float64(len(cover))/1024.0, time.Since(start))
			}
		}

		m.lastSent = fp
		if m.logger != nil {
			m.logger.Infow("Sent playback data to display", "title", meta.Title, "artist", meta.Artist, "source", meta.Source)
		}

		time.Sleep(m.poll)
	}
}

// RunPositionUpdates periodically sends the current playback position,
// paused state, and mic-mute status to the display, independent of whether
// the track itself has changed. Intended to be started via
// `go m.RunPositionUpdates(interval)`.
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

		paused := false
		if p, err := m.player.IsPaused(); err == nil {
			paused = p
		} else if m.logger != nil {
			m.logger.Debugw("Failed to read playback paused state", "error", err)
		}

		micMuted := m.serial.IsMicMuted()

		if err := m.serial.SendUpdate(int(position), paused, micMuted); err != nil {
			if m.logger != nil {
				m.logger.Warnw("Failed to send position update to display", "error", err)
			}
		}
	}
}