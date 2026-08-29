#pragma once
// Configuration Options:

// 1 enables debug output, 0 disables it
#define DEBUG 1

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
#define PROGRESS_Y (PADDING + ALBUM_SIZE + PADDING)
#define PROGRESS_H 8  // Height of the Progressbad-box


// ---------- Colors ----------
#define BG_COLOR TFT_BLACK
#define CARD_COLOR 0x18E3
#define ACCENT_COLOR TFT_CYAN
#define TEXT_COLOR TFT_WHITE
#define SUBTEXT_COLOR TFT_LIGHTGREY