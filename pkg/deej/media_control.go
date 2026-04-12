package deej

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

// MediaController handles media playback commands via playerctl
type MediaController struct {
	logger *zap.SugaredLogger
}

// NewMediaController creates a new media controller instance
func NewMediaController(logger *zap.SugaredLogger) (*MediaController, error) {
	logger = logger.Named("media_control")

	// Verify playerctl is available
	if err := exec.Command("which", "playerctl").Run(); err != nil {
		logger.Warnw("playerctl not found - media controls disabled", "error", err)
		return nil, fmt.Errorf("playerctl not available: %w", err)
	}

	mc := &MediaController{
		logger: logger,
	}

	logger.Debug("Created media controller instance")
	return mc, nil
}

// PlayPause toggles play/pause on the current media player
func (mc *MediaController) PlayPause() error {
	return mc.executeCommand("play-pause")
}

// Next skips to the next track
func (mc *MediaController) Next() error {
	return mc.executeCommand("next")
}

// Previous goes to the previous track
func (mc *MediaController) Previous() error {
	return mc.executeCommand("previous")
}

// Play starts playback
func (mc *MediaController) Play() error {
	return mc.executeCommand("play")
}

// Pause pauses playback
func (mc *MediaController) Pause() error {
	return mc.executeCommand("pause")
}

// Stop stops playback
func (mc *MediaController) Stop() error {
	return mc.executeCommand("stop")
}

// executeCommand runs a playerctl command
func (mc *MediaController) executeCommand(command string) error {
	cmd := exec.Command("playerctl", command)

	// Run the command but don't fail if no player is active - just log it
	if err := cmd.Run(); err != nil {
		mc.logger.Debugw("playerctl command failed (no player active?)", "command", command, "error", err)
		// Don't return error - it's normal if no player is running
		return nil
	}

	mc.logger.Debugw("Executed media command", "command", command)
	return nil
}
