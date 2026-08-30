#pragma once
#include <Arduino.h>
#include <LittleFS.h>

class SerialReceiver {
  public:
    void begin();
    void update();

    bool hasNewSong();
    bool DeejJustConnected();
    bool resetTriggered();
    bool pausedOrmuted();

    String getTitle();
    String getArtist();
    int getDuration();

  private:

    void handleFrame(uint8_t type, uint8_t* data, int len);
    void processBuffer();

    static const int BUF_SIZE = 128;

    File imageFile;

    uint8_t buffer[BUF_SIZE];
    int bufIndex = 0;

    String title;
    String artist;
    int duration = 0;

    bool newSongReady = false;
    bool DeejConnected = false;
    bool reset = false;

    uint32_t received = 0;
    uint32_t length = 0;
    uint32_t lastChunkLengths = 0;
};