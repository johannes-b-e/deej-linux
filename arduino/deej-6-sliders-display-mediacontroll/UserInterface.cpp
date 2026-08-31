#include <Arduino.h>
#include <TFT_eSPI.h>
#include <TJpg_Decoder.h>

#include "UserInterface.h"
#include "config.h"

UserInterface* UserInterface::instance = nullptr;

const char* sliderLabels[6] = {"MIC", "DISCORD", "MUSIC", "FIREFOX", "GAME", "MASTER"};
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
  tft.fillScreen(BG_COLOR);
}

void UserInterface::Update(String newTitle, String newArtist, int newDuration, bool with_img) {
  lastFillW = 0;
  progressInit = false;

  title = newTitle;
  artist = newArtist;
  duration = newDuration;

  tft.fillRect(250, 0, SCREEN_W - 250, SCREEN_H-70, BG_COLOR);

  tft.setTextColor(SUBTEXT_COLOR, BG_COLOR);
  tft.drawString("Now Playing", TEXT_X, PADDING + 5, 2);

  drawTitle();
  drawArtist();
}

void UserInterface::UpdateProgessBar(int timestamp) {
  int barX = TEXT_X;                    // was PADDING - now starts at text column
  int barW = PROGRESS_W;                // was full width - now just the right column

  float progress = (duration > 0) ? (float)timestamp / duration : 0.0f;
  int fillW = barW * progress;

  if (!progressInit) {
    tft.fillRoundRect(barX, PROGRESS_Y, barW, PROGRESS_H, 4, CARD_COLOR);
    progressInit = true;
  }

  if (fillW < lastFillW) {
    tft.fillRoundRect(barX, PROGRESS_Y, barW, PROGRESS_H, 4, CARD_COLOR);
    if (fillW > 0) tft.fillRect(barX, PROGRESS_Y, fillW, PROGRESS_H, ACCENT_COLOR);
  } else if (fillW > lastFillW) {
    tft.fillRect(barX + lastFillW, PROGRESS_Y, fillW - lastFillW, PROGRESS_H, ACCENT_COLOR);
  }
  lastFillW = fillW;

  if (timestamp != lastTimeShown) {
    tft.setTextColor(SUBTEXT_COLOR, BG_COLOR);
    tft.fillRect(barX, PROGRESS_Y + 12, 60, 20, BG_COLOR);
    tft.drawString(formatTime(timestamp), barX, PROGRESS_Y + 12, 2);

    String total = formatTime(duration);
    int totalW = tft.textWidth(total, 2);
    int rightX = barX + barW - totalW;
    tft.fillRect(rightX, PROGRESS_Y + 12, totalW, 20, BG_COLOR);
    tft.drawString(total, rightX, PROGRESS_Y + 12, 2);

    lastTimeShown = timestamp;
  }
}
  
void UserInterface::drawSliderRow(int values[6]) {
  int totalW = SCREEN_W - 2 * PADDING;
  int cellW = totalW / 6;

  for (int i = 0; i < 6; i++) {
    int x = PADDING + i * cellW;

    // label
    tft.setTextColor(sliderColors[i], BG_COLOR);
    tft.fillRect(x, SLIDER_Y, cellW - 4, SLIDER_LABEL_H, BG_COLOR);
    tft.drawString(sliderLabels[i], x, SLIDER_Y, 2);

    // bar background
    int barY = SLIDER_Y + SLIDER_LABEL_H + SLIDER_GAP;
    tft.fillRoundRect(x, barY, cellW - 4, SLIDER_BAR_H, 2, CARD_COLOR);

    // bar fill
    int fillW = (cellW - 4) * values[5-i] / 1023;   // reverse ordering because I failed when soldering the sliders (passiert den besten)
    if (fillW > 0) {
      tft.fillRect(x, barY, fillW, SLIDER_BAR_H, sliderColors[i]);
    }
  }
}

// ---------- Album ----------
void UserInterface::drawAlbum(const char* imagefile) {
  int x = PADDING;
  int y = PADDING;
  int r = ALBUM_SIZE * 0.125;

  tft.fillRoundRect(x, y, ALBUM_SIZE, ALBUM_SIZE, r, CARD_COLOR);

  if (LittleFS.exists(imagefile)) {

    TJpgDec.setJpgScale(1);

    int innerX = x + 1;
    int innerY = y + 1;

    TJpgDec.drawFsJpg(innerX, innerY, imagefile, LittleFS);

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
  String lines[2];                          // was 3
  int count = wrapText(artist, lines, 2, textW, 2);  // was 3

  int y = TITLE_Y + 80;                     // was +90, small nudge up

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