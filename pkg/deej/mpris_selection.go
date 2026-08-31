// Package deej — mpris_selection.go
//
// Player-selection policy: given every MPRIS player currently on the
// session bus, decide which one deej should treat as "the" active player,
// and classify Firefox tabs into cover-art source tiers.
//
// Priority hierarchy:
//  1. Feishin, if playing
//  2. Firefox, if playing (sub-preference: youtube > twitch > other)
//  3. Feishin, if nothing at all is playing (paused fallback)
//  4. any other player
//  5. error - nothing found
package deej

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

const mprisServicePrefix = "org.mpris.MediaPlayer2."

// playerCandidate is a lightweight snapshot of one MPRIS player, gathered
// once per selection pass.
type playerCandidate struct {
	name   string
	status string
	url    string
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

// gatherCandidates collects every MPRIS player currently on the bus, along
// with its playback status and (if present) the xesam:url of what it's
// playing - used to classify Firefox tabs by site.
func gatherCandidates(conn *dbus.Conn) []playerCandidate {
	names, err := listAllBusNames(conn)
	if err != nil {
		return nil
	}

	var candidates []playerCandidate
	for _, name := range names {
		if !strings.HasPrefix(name, mprisServicePrefix) {
			continue
		}

		obj := conn.Object(name, "/org/mpris/MediaPlayer2")
		var props map[string]dbus.Variant
		if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
			continue
		}

		c := playerCandidate{name: name}
		if v, ok := props["PlaybackStatus"]; ok {
			if s, ok := v.Value().(string); ok {
				c.status = s
			}
		}
		if v, ok := props["Metadata"]; ok {
			if m, ok := v.Value().(map[string]dbus.Variant); ok {
				if uv, ok := m["xesam:url"]; ok {
					if s, ok := uv.Value().(string); ok {
						c.url = s
					}
				}
			}
		}
		candidates = append(candidates, c)
	}
	return candidates
}

func isFeishin(c playerCandidate) bool { return strings.Contains(strings.ToLower(c.name), "feishin") }
func isFirefox(c playerCandidate) bool { return strings.Contains(strings.ToLower(c.name), "firefox") }
func isJellyfin(c playerCandidate) bool { return strings.Contains(strings.ToLower(c.name), "jellyfin") }
func isPlaying(c playerCandidate) bool { return c.status == "Playing" }

// firefoxSource classifies a firefox tab's URL into a cover-art source tier.
func firefoxSource(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, "youtube.com"), strings.Contains(lower, "youtu.be"):
		return "youtube"
	case strings.Contains(lower, "twitch.tv"):
		return "twitch"
	default:
		return "firefox"
	}
}

// findMPRISPlayer implements the priority hierarchy described above,
// returning the chosen player's bus name and a "source" tag used to pick
// the right placeholder cover art downstream.
// findMPRISPlayer implements the priority hierarchy described above,
// returning the chosen player's bus name and a "source" tag used to pick
// the right placeholder cover art downstream.
//
// The selection now prefers any playing player (with app-specific
// sub-preferences), and if nothing is playing will stick with the
// previously-selected player if it still exists on the bus.
func findMPRISPlayer(conn *dbus.Conn, previous string) (name string, source string, err error) {
	candidates := gatherCandidates(conn)
	if len(candidates) == 0 {
		if previous != "" {
			return previous, "other", nil
		}
		return "", "", fmt.Errorf("no mpris media player found on dbus")
	}

	// If any player is playing, prefer those and pick by app-priority.
	var playing []playerCandidate
	for _, c := range candidates {
		if isPlaying(c) {
			playing = append(playing, c)
		}
	}
	if len(playing) > 0 {
		// app priority: Jellyfin > Feishin > Firefox > other
		score := func(c playerCandidate) int {
			if isJellyfin(c) {
				return 100
			}
			if isFeishin(c) {
				return 90
			}
			if isFirefox(c) {
				// firefox gets sub-priority based on url
				switch firefoxSource(c.url) {
				case "youtube":
					return 83
				case "twitch":
					return 82
				default:
					return 80
				}
			}
			return 10
		}
		best := playing[0]
		for _, c := range playing[1:] {
			if score(c) > score(best) {
				best = c
			}
		}
		if isFirefox(best) {
			return best.name, firefoxSource(best.url), nil
		}
		if isJellyfin(best) {
			return best.name, "jellyfin", nil
		}
		if isFeishin(best) {
			return best.name, "feishin", nil
		}
		return best.name, "other", nil
	}

	// Nothing is playing. If we have a previous selection that still
	// exists on the bus, stick with it.
	if previous != "" {
		for _, c := range candidates {
			if c.name == previous {
				// try to preserve the previous source tag if possible
				if isFirefox(c) {
					return c.name, firefoxSource(c.url), nil
				}
				if isJellyfin(c) {
					return c.name, "jellyfin", nil
				}
				if isFeishin(c) {
					return c.name, "feishin", nil
				}
				return c.name, "other", nil
			}
		}
	}

	// Fallback to Feishin if present (paused/stopped)
	for _, c := range candidates {
		if isFeishin(c) {
			return c.name, "feishin", nil
		}
	}

	// Otherwise pick the best paused/stopped player using a generic score
	score := func(s string) int {
		switch s {
		case "Playing":
			return 2
		case "Paused":
			return 1
		}
		return 0
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if score(c.status) > score(best.status) {
			best = c
		}
	}
	if isFirefox(best) {
		return best.name, firefoxSource(best.url), nil
	}
	if isJellyfin(best) {
		return best.name, "jellyfin", nil
	}
	if isFeishin(best) {
		return best.name, "feishin", nil
	}
	return best.name, "other", nil
}