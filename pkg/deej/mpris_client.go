package deej

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	mprisServicePrefix = "org.mpris.MediaPlayer2."
)

// displayMetadata is the playback info extracted from an MPRIS player,
// ready for formatting/sending to the display.
type displayMetadata struct {
	Title    string
	Artist   string
	Album    string
	Duration int64
	ArtURL   string
}

type mprisClient struct {
	bus  *dbus.Conn
	name string
}

func newMPRISClient() (*mprisClient, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}

	name, err := findMPRISPlayer(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &mprisClient{bus: conn, name: name}, nil
}

func (c *mprisClient) Close() error {
	if c == nil || c.bus == nil {
		return nil
	}
	return c.bus.Close()
}

// listAllBusNames asks the D-Bus daemon itself for every name currently
// registered on the bus. conn.Names() only returns names owned by our own
// connection, so it can't be used to discover other services like MPRIS
// players.
func listAllBusNames(conn *dbus.Conn) ([]string, error) {
	var names []string
	obj := conn.BusObject()
	if err := obj.Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil, err
	}
	return names, nil
}

func findMPRISPlayer(conn *dbus.Conn) (string, error) {
	names, err := listAllBusNames(conn)
	if err != nil {
		return "", fmt.Errorf("list bus names: %w", err)
	}

	candidates := make([]string, 0, len(names))
	statuses := make(map[string]string, len(names))

	for _, name := range names {
		if !strings.HasPrefix(name, mprisServicePrefix) {
			continue
		}
		candidates = append(candidates, name)

		obj := conn.Object(name, "/org/mpris/MediaPlayer2")
		var props map[string]dbus.Variant
		if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
			continue
		}
		if v, ok := props["PlaybackStatus"]; ok {
			if s, ok := v.Value().(string); ok {
				statuses[name] = s
			}
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no mpris media player found on dbus")
	}

	best := chooseBestMPRISPlayer(candidates, statuses)
	if best == "" {
		return candidates[0], nil
	}
	return best, nil
}

func chooseBestMPRISPlayer(names []string, statuses map[string]string) string {
	if len(names) == 0 {
		return ""
	}

	best := names[0]
	bestScore := scoreForMPRISPlayer(best, statuses[best])

	for _, name := range names[1:] {
		score := scoreForMPRISPlayer(name, statuses[name])
		if score > bestScore || (score == bestScore && isPreferredMPRISPlayer(name) && !isPreferredMPRISPlayer(best)) {
			best = name
			bestScore = score
		}
	}

	return best
}

func scoreForMPRISPlayer(name string, status string) int {
	score := 0
	switch status {
	case "Playing":
		score = 3
	case "Paused":
		score = 2
	case "Stopped":
		score = 1
	}

	if isPreferredMPRISPlayer(name) {
		score += 1
	}

	return score
}

func isPreferredMPRISPlayer(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "feishin")
}

func (c *mprisClient) currentMetadata() (displayMetadata, error) {
	obj := c.bus.Object(c.name, "/org/mpris/MediaPlayer2")

	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
		return displayMetadata{}, fmt.Errorf("read player properties: %w", err)
	}

	meta := displayMetadata{}

	metadataVariant, ok := props["Metadata"]
	if !ok {
		meta.Title = "Unknown title"
		meta.Artist = "Unknown artist"
		return meta, nil
	}

	metadata, ok := metadataVariant.Value().(map[string]dbus.Variant)
	if !ok {
		meta.Title = "Unknown title"
		meta.Artist = "Unknown artist"
		return meta, nil
	}

	if v, ok := metadata["xesam:title"]; ok {
		if s, ok := v.Value().(string); ok {
			meta.Title = s
		}
	}

	if v, ok := metadata["xesam:artist"]; ok {
		switch artistValue := v.Value().(type) {
		case string:
			meta.Artist = artistValue
		case []string:
			meta.Artist = strings.Join(artistValue, ", ")
		case []interface{}:
			parts := make([]string, 0, len(artistValue))
			for _, item := range artistValue {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
			}
			meta.Artist = strings.Join(parts, ", ")
		}
	}

	if v, ok := metadata["xesam:album"]; ok {
		if s, ok := v.Value().(string); ok {
			meta.Album = s
		}
	}

	if v, ok := metadata["mpris:length"]; ok {
		if n, ok := v.Value().(int64); ok {
			meta.Duration = n / 1000000 // microseconds -> seconds
		}
	}

	if v, ok := metadata["mpris:artUrl"]; ok {
		if s, ok := v.Value().(string); ok {
			meta.ArtURL = s
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

// currentPosition returns the player's current playback position, in
// seconds. Position lives at the top level of the Player interface's
// properties (unlike Title/Artist/Album, which are nested inside Metadata).
func (c *mprisClient) currentPosition() (int64, error) {
	obj := c.bus.Object(c.name, "/org/mpris/MediaPlayer2")

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