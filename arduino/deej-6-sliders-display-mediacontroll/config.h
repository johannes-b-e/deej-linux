#pragma once
// Configuration Options:

// 1 enables debug output, 0 disables it
#define DEBUG 1

#define NumPotis 6

// TFT Screen Size;
#define SCREEN_W 480
#define SCREEN_H 320

// Universal Padding along the screen edge:
#define PADDING 16

// Size of the Albumcover viewport:
#define ALBUM_SIZE 230

// Where the Text on the Right side should start
#define TEXT_X (PADDING + ALBUM_SIZE + PADDING)
#define TITLE_Y 60

// Where the Progressbar should start
//#define PROGRESS_Y (PADDING + ALBUM_SIZE + PADDING)
#define PROGRESS_H 8  // Height of the Progressbad-box
#define PROGRESS_Y 210          // moved up, now beside the album art
#define PROGRESS_W (SCREEN_W - TEXT_X - PADDING)  // ~202px, right column width only

// slider row, below everything
#define SLIDER_Y (PADDING + ALBUM_SIZE + PADDING)  // 262, same spot progress bar used to be
#define SLIDER_LABEL_H 20
#define SLIDER_BAR_H 8
#define SLIDER_GAP 2


// ---------- Colors ----------
#define BG_COLOR TFT_BLACK
#define CARD_COLOR 0x18E3
#define ACCENT_COLOR TFT_VIOLET
#define TEXT_COLOR TFT_WHITE
#define SUBTEXT_COLOR TFT_LIGHTGREY
