#pragma once
#include <Arduino.h>
#include <SPIFFS.h>

class SerialReceiver {
  public:
    void begin();
    void update();

    bool hasNewSong();
    bool DeejJustConnected();
    bool resetTriggered();
    bool isMuted();
    bool isPaused();

    String getTitle();
    String getArtist();
    int getDuration();

  private:

    void handleFrame(uint8_t type, uint8_t* data, int len);
    void processBuffer();

    static const int BUF_SIZE = 512;

    File imageFile;

    uint8_t buffer[BUF_SIZE];
    int bufIndex = 0;

    String title;
    String artist;
    int duration = 0;

    bool newSongReady = false;
    bool DeejConnected = false;
    bool reset = false;

    bool muted = false;
    bool paused = false;

    uint32_t received = 0;
    uint32_t length = 0;
};