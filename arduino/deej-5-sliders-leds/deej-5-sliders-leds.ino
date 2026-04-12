#include <Adafruit_NeoPixel.h>

#define NUM_PIXELS   5
#define NEOPIXEL_PIN 3

#define BUTTON_PIN 4
#define SWITCH_PIN 5

const uint8_t brightnessLevels[] = {255, 100, 10}; 

const int NUM_BRIGHTNESS_LEVELS = 3;

int brightnessIndex = 0;
int lastSwitchState = HIGH;

unsigned long lastPressTime = 0;
const unsigned long inactivityTimeout = 10000; // 10 seconds

Adafruit_NeoPixel pixels(NUM_PIXELS, NEOPIXEL_PIN, NEO_GRB + NEO_KHZ800);

// =====================
// SLIDERS
// =====================
const int NUM_SLIDERS = 6;

// 5 analog sliders
const int analogInputs[5] = {A4, A3, A2, A1, A0};

int analogSliderValues[NUM_SLIDERS] = {0};

// Virtual slider (button-controlled)
int virtualSliderValue = 0;

// Button state tracking
int lastButtonState = HIGH;

// =====================
// LED COLORS (UPDATED)
// =====================

// More saturated Discord purple-blue
const uint8_t DISCORD_R = 50,  DISCORD_G = 60,  DISCORD_B = 255;

// More saturated Spotify green
const uint8_t SPOTIFY_R = 0,   SPOTIFY_G = 255, SPOTIFY_B = 0;

// More saturated red (controller)
const uint8_t CONTROLLER_R = 255, CONTROLLER_G = 10, CONTROLLER_B = 10;

// More saturated orange (Chrome / Firefox theme)
const uint8_t CHROME_R = 255, CHROME_G = 80, CHROME_B = 0;

// Neutral speaker grey (left mostly unchanged, slightly cleaner)
const uint8_t SPEAKER_R = 150, SPEAKER_G = 150, SPEAKER_B = 150;


const uint32_t logoColors[5] = {
  Adafruit_NeoPixel::Color(DISCORD_R,   DISCORD_G,   DISCORD_B),
  Adafruit_NeoPixel::Color(SPOTIFY_R,   SPOTIFY_G,   SPOTIFY_B),
  Adafruit_NeoPixel::Color(CONTROLLER_R, CONTROLLER_G, CONTROLLER_B),
  Adafruit_NeoPixel::Color(CHROME_R,    CHROME_G,    CHROME_B),
  Adafruit_NeoPixel::Color(SPEAKER_R,   SPEAKER_G,   SPEAKER_B)
};

// =====================
// SETUP
// =====================
void setup() {
  Serial.begin(9600);

  // Analog sliders
  for (int i = 0; i < 5; i++) {
    pinMode(analogInputs[i], INPUT);
  }

  // Button
  pinMode(BUTTON_PIN, INPUT_PULLUP);

  // Switch (now just a normal input, not controlling sleep)
  pinMode(SWITCH_PIN, INPUT_PULLUP);
  pixels.setBrightness(brightnessLevels[brightnessIndex]);

  // LEDs init
  pixels.begin();
  pixels.clear();
  pixels.show();
}

// =====================
// LOOP
// =====================
void loop() {

  updateSliderValues();
  updateButton();
  sendSliderValues();
  updateDimmer();
  updateLEDs();

  delay(10);
}

// =====================
// Slider handling
// =====================
void updateSliderValues() {
  for (int i = 0; i < 5; i++) {
    analogSliderValues[i] = analogRead(analogInputs[i]);
  }

  // Virtual slider (6th)
  analogSliderValues[5] = virtualSliderValue;
}

// =====================
// Button handling (toggle virtual slider)
// =====================
void updateButton() {
  int currentState = digitalRead(BUTTON_PIN);

  if (lastButtonState == HIGH && currentState == LOW) {
    if (virtualSliderValue == 0) {
      virtualSliderValue = 1023;
    } else {
      virtualSliderValue = 0;
    }
  }

  lastButtonState = currentState;
}

// =====================
// Serial output
// =====================
void sendSliderValues() {
  String builtString = "";

  for (int i = 0; i < NUM_SLIDERS; i++) {
    builtString += String(analogSliderValues[i]);

    if (i < NUM_SLIDERS - 1) {
      builtString += "|";
    }
  }

  Serial.println(builtString);
}

bool ledsEnabled() {
  return digitalRead(SWITCH_PIN) == LOW; 
  // LOW = switch ON (connected to GND)
}

void updateDimmer() {
  int currentState = digitalRead(SWITCH_PIN);

  // Detect button press (falling edge)
  if (lastSwitchState == HIGH && currentState == LOW) {

    brightnessIndex = (brightnessIndex + 1) % NUM_BRIGHTNESS_LEVELS;

    pixels.setBrightness(brightnessLevels[brightnessIndex]);

    lastPressTime = millis(); // reset inactivity timer

    Serial.print("Brightness set to: ");
    Serial.println(brightnessLevels[brightnessIndex]);
  }

  // Inactivity handling (prepare next press to go to 0)
  if (millis() - lastPressTime > inactivityTimeout) {
    // Set index so NEXT increment wraps to 0
    brightnessIndex = NUM_BRIGHTNESS_LEVELS - 1;
  }

  lastSwitchState = currentState;
}

// =====================
// LEDs
// =====================
void updateLEDs() {
  if (!ledsEnabled()) {
    pixels.clear();
    pixels.show();
    return;
  }

  for (int i = 0; i < NUM_PIXELS; i++) {
    pixels.setPixelColor(i, logoColors[i]);
  }
  pixels.show();
}