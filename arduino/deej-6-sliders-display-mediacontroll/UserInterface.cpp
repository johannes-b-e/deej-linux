#include <Arduino.h>
#include <TFT_eSPI.h>
#include <TJpg_Decoder.h>

#include "UserInterface.h"
#include "config.h"

UserInterface* UserInterface::instance = nullptr;

// ---------- JPEG callback ----------
bool UserInterface::tft_output(int16_t x, int16_t y, uint16_t w, uint16_t h, uint16_t *bitmap) {
    instance->tft.pushImage(x, y, w, h, bitmap);
    return true;
}

void UserInterface::begin() {
  instance = this;
  TJpgDec.setCallback(tft_output);
  tft.init();
  tft.setRotation(1);

  TJpgDec.setCallback(tft_output);
  TJpgDec.setSwapBytes(true);
}

void UserInterface::Update(String newTitle, String newArtist, int newDuration, bool with_img) {

  currentTime = 0;
  lastFillW = 0;
  progressInit = false;

  title = newTitle;
  artist = newArtist;
  duration = newDuration;

  tft.fillScreen(BG_COLOR);

  drawAlbum();

  tft.setTextColor(SUBTEXT_COLOR, BG_COLOR);
  tft.drawString("Now Playing", TEXT_X, PADDING + 5, 2);

  drawTitle();
  drawArtist();
}

void UserInterface::UpdateProgessBar(int timestamp) {

  if (timestamp == 0) {
    // simulate playback locally only when we have no real timestamp
    static unsigned long lastPlayback = 0;
    if (millis() - lastPlayback > 1000) {
      currentTime++;
      lastPlayback = millis();
    } else {
      return;
    }
  } else {
    currentTime = timestamp; // trust the real value directly
  }

  int barX = PADDING;
  int barW = SCREEN_W - 2 * PADDING;

  float progress = (duration > 0) ? (float)currentTime / duration : 0.0f;

  int fillW = barW * progress;

  if (!progressInit) {
    tft.fillRoundRect(barX, PROGRESS_Y, barW, PROGRESS_H, 4, CARD_COLOR);
    progressInit = true;
  }

  if (fillW > lastFillW) {
    tft.fillRect(barX + lastFillW, PROGRESS_Y, fillW - lastFillW, PROGRESS_H, ACCENT_COLOR);
  }

  lastFillW = fillW;

  if (currentTime != lastTimeShown) {
    tft.setTextColor(SUBTEXT_COLOR, BG_COLOR);

    tft.fillRect(barX, PROGRESS_Y + 12, 60, 20, BG_COLOR);
    tft.drawString(formatTime(currentTime), barX, PROGRESS_Y + 12, 2);

    String total = formatTime(duration);
    int totalW = tft.textWidth(total, 2);

    int rightX = barX + barW - totalW;

    tft.fillRect(rightX, PROGRESS_Y + 12, totalW, 20, BG_COLOR);
    tft.drawString(total, rightX, PROGRESS_Y + 12, 2);

    lastTimeShown = currentTime;
  }
}

// ---------- Album ----------
void UserInterface::drawAlbum() {
  int x = PADDING;
  int y = PADDING;
  int r = ALBUM_SIZE * 0.125;

  tft.fillRoundRect(x, y, ALBUM_SIZE, ALBUM_SIZE, r, CARD_COLOR);

  if (SPIFFS.exists("/cover.jpg")) {

    TJpgDec.setJpgScale(1);

    int innerX = x + 1;
    int innerY = y + 1;

    TJpgDec.drawFsJpg(innerX, innerY, "/cover.jpg");

    tft.drawRoundRect(x, y, ALBUM_SIZE, ALBUM_SIZE, r, TFT_DARKGREY);
  }
}


void UserInterface::drawTitle() {
  int textW = SCREEN_W - TEXT_X - PADDING;

  String lines[3];
  int count = wrapText(title, lines, 3, textW, 4);

  int y = TITLE_Y;

  tft.setTextColor(TEXT_COLOR, BG_COLOR);
  tft.fillRect(TEXT_X, TITLE_Y, textW, 80, BG_COLOR);

  for (int i = 0; i < count; i++) {
    tft.drawString(lines[i], TEXT_X, y, 4);
    y += 26;
  }
}

void UserInterface::drawArtist() {
  int textW = SCREEN_W - TEXT_X - PADDING;

  String lines[3];
  int count = wrapText(artist, lines, 3, textW, 2);

  int y = TITLE_Y + 90;

  tft.setTextColor(SUBTEXT_COLOR, BG_COLOR);

  for (int i = 0; i < count; i++) {
    tft.drawString(lines[i], TEXT_X, y, 2);
    y += 18;
  }
}

// ---------- Progress ----------
String UserInterface::formatTime(int sec) {
  int m = sec / 60;
  int s = sec % 60;

  char buf[8];
  sprintf(buf, "%d:%02d", m, s);

  return String(buf);
}


// ---------- Text wrap ----------
int UserInterface::wrapText(String text, String* lines, int maxLines, int maxWidth, int font) {
  int lineCount = 0;
  String current;

  int start = 0;

  while (start < text.length() && lineCount < maxLines) {
    int spaceIndex = text.indexOf(' ', start);
    if (spaceIndex == -1) spaceIndex = text.length();

    String word = text.substring(start, spaceIndex);
    String test = current + (current.length() ? " " : "") + word;

    if (tft.textWidth(test, font) > maxWidth) {
      lines[lineCount++] = current;
      current = word;
    } else {
      current = test;
    }

    start = spaceIndex + 1;
  }

  if (lineCount < maxLines) {
    lines[lineCount++] = current;
  }

  return lineCount;
}