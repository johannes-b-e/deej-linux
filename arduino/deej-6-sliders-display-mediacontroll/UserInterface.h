#pragma once

#include <Arduino.h>
#include <TFT_eSPI.h>


class UserInterface {
  public:
    void begin();
    void Update(String newTitle = "Literally nothing!", String newArtist = "waiting for playback data ...", int newDuration = 0, bool with_img = false);
    void UpdateProgessBar(int timestamp = 0);

    static UserInterface* instance;

  private:
    void drawAlbum();
    void drawTitle();
    void drawArtist();
    String formatTime(int sec);
    int wrapText(String text, String* lines, int maxLines, int maxWidth, int font);
    static bool tft_output(int16_t x, int16_t y, uint16_t w, uint16_t h, uint16_t *bitmap);


    // ---------- TFT ----------
    TFT_eSPI tft = TFT_eSPI();

    // ---------- Demo-Data ----------
    String title;
    String artist;

    int duration = 150;
    int currentTime = 0;

    // ---------- State ----------
    int lastFillW = 0;
    bool progressInit = false;
    int lastTimeShown = -1;
};

