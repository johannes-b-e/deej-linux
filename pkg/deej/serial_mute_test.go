package deej

import (
	"sync"
	"testing"
)

type fakeMuteSession struct {
	muted bool
}

func (s *fakeMuteSession) GetVolume() float32 { return 0 }
func (s *fakeMuteSession) SetVolume(v float32) error { return nil }
func (s *fakeMuteSession) Key() string { return "mic" }
func (s *fakeMuteSession) Release() {}
func (s *fakeMuteSession) SetMute(m bool) error {
	s.muted = m
	return nil
}
func (s *fakeMuteSession) GetMute() bool { return s.muted }

func TestSerialIOSetMicMute(t *testing.T) {
	m := &sessionMap{
		m:    map[string][]Session{"mic": {&fakeMuteSession{}}},
		lock: &sync.Mutex{},
	}

	d := &Deej{sessions: m}
	sio := &SerialIO{deej: d}

	sio.setMicMute(true)

	sessions, ok := d.sessions.get(inputSessionName)
	if !ok || len(sessions) != 1 {
		t.Fatalf("expected mic session to exist, got %#v", sessions)
	}

	muteSession, ok := sessions[0].(interface{ GetMute() bool; SetMute(bool) error })
	if !ok {
		t.Fatal("mic session does not support mute operations")
	}

	if !muteSession.GetMute() {
		t.Fatal("expected mic to be muted")
	}

	sio.setMicMute(false)
	if muteSession.GetMute() {
		t.Fatal("expected mic to be unmuted")
	}
}
