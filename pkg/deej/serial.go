package deej

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

const (
	frameTypeMetadata = byte(1)
	frameTypeImage    = byte(2)
	frameTypeUpdate   = byte(3)
	imageChunkSize    = 4096
)

// SerialIO provides a deej-aware abstraction layer to managing serial I/O
type SerialIO struct {
	comPort  string
	baudRate uint

	deej   *Deej
	logger *zap.SugaredLogger

	stopChannel chan bool
	connected   bool
	connOptions serial.OpenOptions
	conn        io.ReadWriteCloser

	okChannel chan struct{} // signaled when an "OK" line arrives from the esp32

	// transfer protection: set while a cover transfer is active
	transferMu  sync.Mutex
	transfering bool

	lastKnownNumSliders        int
	currentSliderPercentValues []float32
	sliderMoveConsumers        []chan SliderMoveEvent

	mediaController *MediaController
}

// SliderMoveEvent represents a single slider move captured by deej
type SliderMoveEvent struct {
	SliderID     int
	PercentValue float32
}

var expectedLinePattern = regexp.MustCompile(`^\d{1,4}(\|\d{1,4})*\r\n$`)
var expectedCommandPattern = regexp.MustCompile(`^CMD:(\w+)\r\n$`)

// NewSerialIO creates a SerialIO instance that uses the provided deej
// instance's connection info to establish communications with the arduino chip
func NewSerialIO(deej *Deej, logger *zap.SugaredLogger) (*SerialIO, error) {
	logger = logger.Named("serial")

	// Initialize media controller (not fatal if playerctl isn't available)
	mediaController, err := NewMediaController(logger)
	if err != nil {
		logger.Infow("Media controller not available", "error", err)
	}

	sio := &SerialIO{
		deej:                deej,
		logger:              logger,
		stopChannel:         make(chan bool, 4),
		connected:           false,
		conn:                nil,
		okChannel:           make(chan struct{}, 8),
		sliderMoveConsumers: []chan SliderMoveEvent{},
		mediaController:     mediaController,
	}

	logger.Debug("Created serial i/o instance")

	// respond to config changes
	sio.setupOnConfigReload()

	return sio, nil
}

// Start attempts to connect to our arduino chip
func (sio *SerialIO) Start() error {

	// don't allow multiple concurrent connections
	if sio.connected {
		sio.logger.Warn("Already connected, can't start another without closing first")
		return errors.New("serial: connection already active")
	}

	// set minimum read size according to platform (0 for windows, 1 for linux)
	// this prevents a rare bug on windows where serial reads get congested,
	// resulting in significant lag
	minimumReadSize := 0
	if util.Linux() {
		minimumReadSize = 1
	}

	sio.connOptions = serial.OpenOptions{
		PortName:        sio.deej.config.ConnectionInfo.COMPort,
		BaudRate:        uint(sio.deej.config.ConnectionInfo.BaudRate),
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: uint(minimumReadSize),
	}

	sio.logger.Debugw("Attempting serial connection",
		"comPort", sio.connOptions.PortName,
		"baudRate", sio.connOptions.BaudRate,
		"minReadSize", minimumReadSize)

	var err error
	sio.conn, err = serial.Open(sio.connOptions)
	if err != nil {

		// might need a user notification here, TBD
		sio.logger.Warnw("Failed to open serial connection", "error", err)
		return fmt.Errorf("open serial connection: %w", err)
	}

	namedLogger := sio.logger.Named(strings.ToLower(sio.connOptions.PortName))

	namedLogger.Infow("Connected", "conn", sio.conn)
	sio.connected = true

	// read lines or await a stop
	go func() {
		connReader := bufio.NewReader(sio.conn)
		lineChannel := sio.readLine(namedLogger, connReader)
		sio.conn.Write([]byte{0xFF})	// notify the microcontroller that deej connected
		for {
			select {
			case <-sio.stopChannel:
				sio.close(namedLogger)
			case line := <-lineChannel:
				if line == "BORKED" {
					sio.close(namedLogger)
					sio.logger.Info("RX bork signal from reader. Signalling stop channel...")
					sio.stopChannel <- false
					sio.logger.Info("Stop channel signalled with a retry.")
					os.Exit(1)
				} else {
					sio.handleLine(namedLogger, line)
				}
			}
		}
	}()

	return nil
}

// Stop signals us to shut down our serial connection, if one is active
func (sio *SerialIO) Stop() {
	if sio.connected {
		sio.logger.Debug("Shutting down serial connection")
		sio.stopChannel <- true
	} else {
		sio.logger.Debug("Not currently connected, nothing to stop")
	}
}

// SubscribeToSliderMoveEvents returns an unbuffered channel that receives
// a sliderMoveEvent struct every time a slider moves
func (sio *SerialIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	ch := make(chan SliderMoveEvent)
	sio.sliderMoveConsumers = append(sio.sliderMoveConsumers, ch)

	return ch
}

func (sio *SerialIO) setupOnConfigReload() {
	configReloadedChannel := sio.deej.config.SubscribeToChanges()

	const stopDelay = 50 * time.Millisecond

	go func() {
		for {
			select {
			case <-configReloadedChannel:

				// make any config reload unset our slider number to ensure process volumes are being re-set
				// (the next read line will emit SliderMoveEvent instances for all sliders)\
				// this needs to happen after a small delay, because the session map will also re-acquire sessions
				// whenever the config file is reloaded, and we don't want it to receive these move events while the map
				// is still cleared. this is kind of ugly, but shouldn't cause any issues
				go func() {
					<-time.After(stopDelay)
					sio.lastKnownNumSliders = 0
				}()

				// if connection params have changed, attempt to stop and start the connection
				if sio.deej.config.ConnectionInfo.COMPort != sio.connOptions.PortName ||
					uint(sio.deej.config.ConnectionInfo.BaudRate) != sio.connOptions.BaudRate {

					sio.logger.Info("Detected change in connection parameters, attempting to renew connection")
					sio.Stop()

					// let the connection close
					<-time.After(stopDelay)

					if err := sio.Start(); err != nil {
						sio.logger.Warnw("Failed to renew connection after parameter change", "error", err)
					} else {
						sio.logger.Debug("Renewed connection successfully")
					}
				}
			}
		}
	}()
}

func (sio *SerialIO) close(logger *zap.SugaredLogger) {
	if err := sio.conn.Close(); err != nil {
		logger.Warnw("Failed to close serial connection", "error", err)
	} else {
		logger.Debug("Serial connection closed")
	}

	sio.conn = nil
	sio.connected = false
}

func (sio *SerialIO) readLine(logger *zap.SugaredLogger, reader *bufio.Reader) chan string {
	ch := make(chan string)

	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if sio.deej.Verbose() {
					logger.Warnw("Failed to read line from serial", "error", err, "line", line)
				}

				// Report the error via the line channel
				ch <- "BORKED"
				return
			}

			if sio.deej.Verbose() {
				logger.Debugw("Read new line", "line", line)
			}

			// deliver the line to the channel
			ch <- line
		}
	}()

	return ch
}

func (sio *SerialIO) handleLine(logger *zap.SugaredLogger, line string) {

	// the esp32 sends "OK\n" after consuming each image chunk (firmware must
	// terminate this with \n for line-based reading to work at all - see
	// SerialReceiver.cpp's Serial.write("OK") call).
	trimmed := strings.TrimRight(line, "\r\n")
	if trimmed == "OK" {
		select {
		case sio.okChannel <- struct{}{}:
		default:
		}
		return
	}

	// Check for media control commands first (e.g., "CMD:pause\r\n")
	if expectedCommandPattern.MatchString(line) {
		sio.handleMediaCommand(logger, line)
		return
	}

	// this function receives an unsanitized line which is guaranteed to end with LF,
	// but most lines will end with CRLF. it may also have garbage instead of
	// deej-formatted values, so we must check for that! just ignore bad ones
	if !expectedLinePattern.MatchString(line) {
		return
	}

	// trim the suffix
	line = strings.TrimSuffix(line, "\r\n")

	// split on pipe (|), this gives a slice of numerical strings between "0" and "1023"
	splitLine := strings.Split(line, "|")
	numSliders := len(splitLine)

	// update our slider count, if needed - this will send slider move events for all
	if numSliders != sio.lastKnownNumSliders {
		logger.Infow("Detected sliders", "amount", numSliders)
		sio.lastKnownNumSliders = numSliders
		sio.currentSliderPercentValues = make([]float32, numSliders)

		// reset everything to be an impossible value to force the slider move event later
		for idx := range sio.currentSliderPercentValues {
			sio.currentSliderPercentValues[idx] = -1.0
		}
	}

	// for each slider:
	moveEvents := []SliderMoveEvent{}
	for sliderIdx, stringValue := range splitLine {

		// convert string values to integers ("1023" -> 1023)
		number, _ := strconv.Atoi(stringValue)

		// turns out the first line could come out dirty sometimes (i.e. "4558|925|41|643|220")
		// so let's check the first number for correctness just in case
		if sliderIdx == 0 && number > 1023 {
			sio.logger.Debugw("Got malformed line from serial, ignoring", "line", line)
			return
		}

		// map the value from raw to a "dirty" float between 0 and 1 (e.g. 0.15451...)
		dirtyFloat := float32(number) / 1023.0

		// normalize it to an actual volume scalar between 0.0 and 1.0 with 2 points of precision
		normalizedScalar := util.NormalizeScalar(dirtyFloat)

		// if sliders are inverted, take the complement of 1.0
		if sio.deej.config.InvertSliders {
			normalizedScalar = 1 - normalizedScalar
		}

		// check if it changes the desired state (could just be a jumpy raw slider value)
		if util.SignificantlyDifferent(sio.currentSliderPercentValues[sliderIdx], normalizedScalar, sio.deej.config.NoiseReductionLevel) {

			// if it does, update the saved value and create a move event
			sio.currentSliderPercentValues[sliderIdx] = normalizedScalar

			moveEvents = append(moveEvents, SliderMoveEvent{
				SliderID:     sliderIdx,
				PercentValue: normalizedScalar,
			})

			if sio.deej.Verbose() {
				logger.Debugw("Slider moved", "event", moveEvents[len(moveEvents)-1])
			}
		}
	}

	// deliver move events if there are any, towards all potential consumers
	if len(moveEvents) > 0 {
		for _, consumer := range sio.sliderMoveConsumers {
			for _, moveEvent := range moveEvents {
				consumer <- moveEvent
			}
		}
	}
}

// writeFrameHeader writes the display-protocol frame header (0xAA 0x55 +
// type byte + little-endian uint32 length), matching what SerialReceiver's
// WAIT_AA state expects.
func (sio *SerialIO) writeFrameHeader(frameType byte, length int) error {
	if !sio.connected || sio.conn == nil {
		return fmt.Errorf("serial not connected")
	}

	lenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBytes, uint32(length))

	for _, chunk := range [][]byte{{0xAA, 0x55}, {frameType}, lenBytes} {
		if _, err := sio.conn.Write(chunk); err != nil {
			return fmt.Errorf("write frame header: %w", err)
		}
	}
	return nil
}

// writeFrame writes a complete single-shot frame (header + payload) - used
// for metadata, where the firmware reads the whole payload in one shot
// rather than chunk-by-chunk.
func (sio *SerialIO) writeFrame(frameType byte, payload []byte) error {
	if err := sio.writeFrameHeader(frameType, len(payload)); err != nil {
		return err
	}
	if _, err := sio.conn.Write(payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

// waitForOK blocks until an "OK" acknowledgment has been observed coming
// from the esp32, or the given timeout elapses.
func (sio *SerialIO) waitForOK(timeout time.Duration) error {
	select {
	case <-sio.okChannel:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for OK from esp32")
	}
}

// SendMetadata sends the currently-playing track's metadata to the display,
// formatted as "title|artist|duration_seconds" to match SerialReceiver's
// PackageType 1 parsing.
func (sio *SerialIO) SendMetadata(title, artist string, durationSeconds int) error {
	payload := []byte(fmt.Sprintf("%s|%s|%d", title, artist, durationSeconds))
	return sio.writeFrame(frameTypeMetadata, payload)
}

// SendCover sends JPEG-encoded cover art to the display in imageChunkSize
// chunks, waiting for an "OK" acknowledgment from the esp32 after each
// chunk before sending the next one - matching SerialReceiver's
// PackageType 2 handling.
func (sio *SerialIO) SendCover(data []byte) error {
	// mark transfer as active to prevent status updates during image upload
	sio.transferMu.Lock()
	sio.transfering = true
	sio.transferMu.Unlock()
	if err := sio.writeFrameHeader(frameTypeImage, len(data)); err != nil {
		sio.transferMu.Lock()
		sio.transfering = false
		sio.transferMu.Unlock()
		return err
	}

	for offset := 0; offset < len(data); offset += imageChunkSize {
		end := offset + imageChunkSize
		if end > len(data) {
			end = len(data)
		}

		if _, err := sio.conn.Write(data[offset:end]); err != nil {
			return fmt.Errorf("write image chunk: %w", err)
		}
		if err := sio.waitForOK(2 * time.Second); err != nil {
			// if an OK wasn't received, still try to continue but log
			sio.logger.Warnw("No OK received for image chunk", "range", fmt.Sprintf("%d-%d", offset, end), "error", err)
		}
	}
	// mark transfer finished
	sio.transferMu.Lock()
	sio.transfering = false
	sio.transferMu.Unlock()
	return nil
}

// SendUpdate sends a lightweight playback-position + mic-mute update to the
// display, formatted as "timestamp_seconds|mic_muted(1/0)" to match
// SerialReceiver's PackageType 3 parsing.
func (sio *SerialIO) SendUpdate(timestampSeconds int, paused bool, micMuted bool) error {
	// Do not send status updates while a cover transfer is active.
	sio.transferMu.Lock()
	transferring := sio.transfering
	sio.transferMu.Unlock()
	if transferring {
		if sio.deej != nil && sio.deej.Verbose() {
			sio.logger.Debugw("Dropping SendUpdate while cover transfer active")
		}
		return nil
	}
	muteFlag := 0
	if micMuted {
		muteFlag = 1
	}
	pauseFlag := 0
	if paused {
		pauseFlag = 1
	}
	payload := []byte(fmt.Sprintf("%d|%d|%d", timestampSeconds, pauseFlag, muteFlag))
	return sio.writeFrame(frameTypeUpdate, payload)
}

// IsMicMuted reports whether the input (microphone) session is currently
// muted. Returns false if no mic session is found.
func (sio *SerialIO) IsMicMuted() bool {
	if sio.deej == nil || sio.deej.sessions == nil {
		return false
	}
	sessions, ok := sio.deej.sessions.get(inputSessionName)
	if !ok || len(sessions) == 0 {
		return false
	}
	return sessions[0].GetMute()
}

// handleMediaCommand processes media control commands from the Arduino
func (sio *SerialIO) setMicMute(mute bool) error {
	if sio.deej == nil || sio.deej.sessions == nil {
		return fmt.Errorf("session map unavailable")
	}

	sessions, ok := sio.deej.sessions.get(inputSessionName)
	if !ok || len(sessions) == 0 {
		return fmt.Errorf("no mic session found")
	}

	for _, session := range sessions {
		if session.GetMute() == mute {
			continue
		}
		if err := session.SetMute(mute); err != nil {
			return err
		}
	}

	return nil
}

func (sio *SerialIO) handleMediaCommand(logger *zap.SugaredLogger, line string) {
	// Extract command name from "CMD:command" format
	matches := expectedCommandPattern.FindStringSubmatch(line)
	if len(matches) < 2 {
		return
	}

	command := matches[1]
	logger.Debugw("Received media command", "command", command)

	// Microphone mute controls do not depend on playerctl.
	if strings.EqualFold(command, "mutemic") || strings.EqualFold(command, "togglemic") {
		if sio.deej == nil || sio.deej.sessions == nil {
			logger.Warnw("No session map available for microphone control", "command", command)
			return
		}

		sessions, ok := sio.deej.sessions.get(inputSessionName)
		if !ok || len(sessions) == 0 {
			logger.Warnw("No microphone session found to toggle", "command", command)
			return
		}

		newMuteState := !sessions[0].GetMute()
		if err := sio.setMicMute(newMuteState); err != nil {
			logger.Warnw("Failed to toggle microphone mute", "error", err)
		}
		return
	}

	if strings.EqualFold(command, "unmutemic") {
		if err := sio.setMicMute(false); err != nil {
			logger.Warnw("Failed to unmute microphone", "error", err)
		}
		return
	}

	if sio.mediaController == nil {
		logger.Debug("Media controller not available, ignoring command")
		return
	}

	// Execute the appropriate command
	switch strings.ToLower(command) {
	case "play":
		sio.mediaController.Play()
	case "pause":
		sio.mediaController.Pause()
	case "playpause", "play-pause", "toggle":
		sio.mediaController.PlayPause()
	case "next", "skip":
		sio.mediaController.Next()
	case "prev", "previous":
		sio.mediaController.Previous()
	case "stop":
		sio.mediaController.Stop()
	default:
		logger.Warnw("Unknown media command", "command", command)
	}
}