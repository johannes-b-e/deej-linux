#include <TFT_eSPI.h>
#include <TJpg_Decoder.h>
#include <LittleFS.h>

//#include "wifi_manager.h"
#include "config.h"
#include "SerialReceiver.h"
#include "UserInterface.h"

int poti[NumPotis] = {33, 32, 35, 34, 39, 36};


float filtered[NumPotis] = {0};
int last[NumPotis] = {0};
String SliderOutput;

int buttons[4] = {19, 21, 16, 17};
int NumButtons = 4;
bool pressed[4] = {false, false, false, false};

String buttonCMD[4] = {
  "CMD:mutemic",
  "CMD:prev",
  "CMD:playpause",
  "CMD:next"
};

const int BUTTON_PLAYPAUSE = 16;
const int BUTTON_NEXT = 17;
const int BUTTON_MUTEMIC = 19;
const int BUTTON_PREVIOUS = 21;


int prevValue = 0;

SerialReceiver receiver;
UserInterface ui;

void onErrorCallback(hardwareSerial_error_t error) {
    Serial.printf("UART Error: %d\n", error);
}

// ---------- Setup ----------
void setup() {

  ui.begin();

  int buffersize = Serial.setRxBufferSize(16384);
  Serial.setTxBufferSize(16384);
  Serial.begin(921600);
  Serial.print("Buffersize set to ");
  Serial.println(buffersize);

  Serial.onReceiveError(onErrorCallback);

  LittleFS.begin(true);

  receiver.begin();

  ui.Update();  //Update UI with defaults
  int values[NumPotis] = {0};
  ui.drawSliderRow(values);
  ui.drawAlbum("/default_cover.jpg");

  for(int i = 0; i < NumPotis; i++){
    analogSetPinAttenuation(poti[i], ADC_11db);
  }

  pinMode(BUTTON_PLAYPAUSE, INPUT_PULLUP);
  pinMode(BUTTON_NEXT, INPUT_PULLUP);
  pinMode(BUTTON_MUTEMIC, INPUT_PULLUP);
  pinMode(BUTTON_PREVIOUS, INPUT_PULLUP);
}

void updateButtons() {
  for (int i = 0; i < NumButtons; i++) {
    if (digitalRead(buttons[i]) == LOW && !pressed[i]) {
      Serial.println(buttonCMD[i]);
      pressed[i] = true;
    }
    else if (digitalRead(buttons[i]) != LOW && pressed[i]) {
      pressed[i] = false;
    }
  }
}
bool UpdateSliders(String &output) {
  bool updateRequired = false;

  for (int i = 0; i < NumPotis; i++) {
    int raw = analogRead(poti[i]);

    filtered[i] = filtered[i] * 0.85 + raw * 0.15;  //Hystersis(?) smoothing value over by combining with prev. value.

    int value = (int)(filtered[i] / 4);

    if (abs(value - last[i]) > 4) {
      ui.drawSliderRowProgress(5-i, value, last[i]);
      last[i] = value;
      // Reverse physical order for UI
      //ui.drawSliderRowSlider(5 - i, value);

      updateRequired = true;
    }
  }

  if (updateRequired) {
    output = "";

    for (int i = 0; i < NumPotis; i++) {
      output += last[i];

      if (i < NumPotis - 1) {
        output += "|";
      }
    }
  }
  return updateRequired;
}
// ---------- Loop ----------
void loop() {
  if (receiver.resetTriggered()) {
    ui.drawAlbum("/default_cover.jpg");
  }

  receiver.update();

  if(UpdateSliders(SliderOutput) || receiver.DeejJustConnected()){
    Serial.println(SliderOutput);
  }

  /*
  String debug = "";

  for (int i = 0; i < NumPotis; i++) {
    sliderValues[i] = readSlider(i, analogRead(poti[i]));

    debug += sliderValues[i];

    if (i < NumPotis - 1) {
      debug += "|";
    }
  }
  

  // Only send/update UI if values changed or a new Deej client connected
  if (debug != oldDebug || receiver.DeejJustConnected()) {
    Serial.println(debug);

    // Update the visual sliders
    ui.drawSliderRow(sliderValues);

    oldDebug = debug;
  }
  */

  if (receiver.hasNewSong()) {
    Serial.println("New song received!");

    ui.Update(
      receiver.getTitle(),
      receiver.getArtist(),
      receiver.getDuration(),
      true
    );

    if (!receiver.pausedOrmuted()) {
      UserInterface::instance->drawAlbum("/cover.jpg");
    }
  }

  updateButtons();
}