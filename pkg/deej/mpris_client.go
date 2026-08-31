// Package deej — mpris_client.go
//
// Low-level MPRIS/D-Bus plumbing: connecting to the session bus and reading
// properties (metadata, position, playback status) from whichever player is
// currently selected. Player-selection policy itself lives in
// mpris_selection.go.
package deej

import (
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// displayMetadata is the playback info extracted from an MPRIS player,
// ready for formatting/sending to the display.
type displayMetadata struct {
	Title    string
	Artist   string
	Album    string
	Duration int64
	ArtURL   string
	Source   string // "feishin", "youtube", "twitch", "firefox", "other"
	TrackURL string // optional URL of the track (e.g. youtube watch URL)
}

// maxReasonableDurationMicros guards against Firefox's "unknown duration"
// sentinel (int64 max), which would otherwise be misread as a real,
// absurdly large track length.
const maxReasonableDurationMicros = int64(24 * 60 * 60 * 1000000) // 24h

type mprisClient struct {
	bus *dbus.Conn

	mu     sync.RWMutex
	name   string
	source string
}

func newMPRISClient() (*mprisClient, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}
	return &mprisClient{bus: conn}, nil
}

func (c *mprisClient) Close() error {
	if c == nil || c.bus == nil {
		return nil
	}
	return c.bus.Close()
}

// refreshSelection re-runs the player-selection hierarchy (mpris_selection.go)
// and updates the client's current target. Selection can change at any time
// (e.g. Feishin gets started while Firefox was playing), so this is called
// before every property read that depends on "which player is currently the
// right one" rather than once at startup.
func (c *mprisClient) refreshSelection() error {
	// pass the previously-selected name so selection can prefer it when
	// nothing is actively playing
	name, source, err := findMPRISPlayer(c.bus, c.name)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.name = name
	c.source = source
	c.mu.Unlock()
	return nil
}

func (c *mprisClient) currentTarget() (name, source string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.name, c.source
}

// currentMetadata re-selects the correct player, then reads its metadata.
func (c *mprisClient) currentMetadata() (displayMetadata, error) {
	if err := c.refreshSelection(); err != nil {
		return displayMetadata{}, err
	}
	name, source := c.currentTarget()

	obj := c.bus.Object(name, "/org/mpris/MediaPlayer2")
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
		return displayMetadata{}, fmt.Errorf("read player properties: %w", err)
	}

	meta := displayMetadata{Source: source}

	metadataVariant, ok := props["Metadata"]
	if !ok {
		meta.Title, meta.Artist = "Unknown title", "Unknown artist"
		return meta, nil
	}
	metadata, ok := metadataVariant.Value().(map[string]dbus.Variant)
	if !ok {
		meta.Title, meta.Artist = "Unknown title", "Unknown artist"
		return meta, nil
	}

	if v, ok := metadata["xesam:title"]; ok {
		switch tv := v.Value().(type) {
		case string:
			meta.Title = tv
		case []byte:
			meta.Title = string(tv)
		}
	}

	if v, ok := metadata["xesam:artist"]; ok {
		switch av := v.Value().(type) {
		case string:
			meta.Artist = av
		case []string:
			meta.Artist = strings.Join(av, ", ")
		case []interface{}:
			parts := make([]string, 0, len(av))
			for _, item := range av {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
			}
			meta.Artist = strings.Join(parts, ", ")
		}
	}

	if v, ok := metadata["xesam:album"]; ok {
		switch av := v.Value().(type) {
		case string:
			meta.Album = av
		case []byte:
			meta.Album = string(av)
		}
	}

	if v, ok := metadata["mpris:length"]; ok {
		if n, ok := v.Value().(int64); ok && n > 0 && n < maxReasonableDurationMicros {
			meta.Duration = n / 1000000 // microseconds -> seconds
		}
		// else: leave Duration at 0 - sentinel/unknown value, not real data
	}

	if v, ok := metadata["mpris:artUrl"]; ok {
		switch uv := v.Value().(type) {
		case string:
			meta.ArtURL = uv
		case []byte:
			meta.ArtURL = string(uv)
		}
	}

	if v, ok := metadata["xesam:url"]; ok {
		switch uv := v.Value().(type) {
		case string:
			meta.TrackURL = uv
		case []byte:
			meta.TrackURL = string(uv)
		}
	}

	if meta.Title == "" {
		meta.Title = "Unknown title"
	}
	if meta.Artist == "" {
		meta.Artist = "Unknown artist"
	}

	return meta, nil
}

// currentPosition returns the currently-selected player's playback position,
// in seconds. Position lives at the top level of the Player interface's
// properties (unlike Title/Artist/Album, which are nested inside Metadata).
func (c *mprisClient) currentPosition() (int64, error) {
	name, _ := c.currentTarget()
	if name == "" {
		return 0, nil
	}

	obj := c.bus.Object(name, "/org/mpris/MediaPlayer2")
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
		return 0, fmt.Errorf("read player properties: %w", err)
	}

	if v, ok := props["Position"]; ok {
		if n, ok := v.Value().(int64); ok {
			return n / 1000000, nil // microseconds -> seconds
		}
	}
	return 0, nil
}

// IsPaused returns true if the currently-selected player reports a "Paused"
// playback status.
func (c *mprisClient) IsPaused() (bool, error) {
	name, _ := c.currentTarget()
	if name == "" {
		return false, nil
	}

	obj := c.bus.Object(name, "/org/mpris/MediaPlayer2")
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
		return false, fmt.Errorf("read player properties: %w", err)
	}

	if v, ok := props["PlaybackStatus"]; ok {
		if s, ok := v.Value().(string); ok {
			return s == "Paused", nil
		}
	}
	return false, nil
}