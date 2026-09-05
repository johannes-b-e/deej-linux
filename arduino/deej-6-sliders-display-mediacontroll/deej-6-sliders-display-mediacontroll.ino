#include <TFT_eSPI.h>
#include <TJpg_Decoder.h>
#include <LittleFS.h>

//#include "wifi_manager.h"
#include "config.h"
#include "SerialReceiver.h"
#include "UserInterface.h"

int poti[NumPotis] = {33, 32, 35, 34, 39, 36};

//track controller's powermode:
#define POWERMODE_ACTIVE 1    // Normal Operations
#define POWERMODE_STANDBY 0   // Sliders but no Display
#define POWERMODE_OFF -1      // idle
int powermode = POWERMODE_ACTIVE;
const int transistorPin = 25;
bool justWokeUp = false;


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

  pinMode(transistorPin, OUTPUT);
  digitalWrite(transistorPin, HIGH);

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

bool updateButtons() {
  bool pressRegistered = false;
  for (int i = 0; i < NumButtons; i++) {
    if (digitalRead(buttons[i]) == LOW && !pressed[i]) {
      Serial.println(buttonCMD[i]);
      pressRegistered = true;
      pressed[i] = true;
    }
    else if (digitalRead(buttons[i]) != LOW && pressed[i]) {
      pressed[i] = false;
    }
  }
  return pressRegistered;
}
bool UpdateSliders(String &output, bool higherThreshold = false) {
  bool updateRequired = false;
  int threshold = higherThreshold ? 50 : 4;  // higher threshold only to wake from standby.

  for (int i = 0; i < NumPotis; i++) {
    int raw = analogRead(poti[i]);

    filtered[i] = filtered[i] * 0.85 + raw * 0.15;  //Hystersis(?) smoothing value over by combining with prev. value.
    int value = (int)(filtered[i] / 4);

    if (abs(value - last[i]) > threshold) {
      ui.drawSliderRowProgress(5-i, value, last[i]);
      last[i] = value;
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

void goToSleep(int mode = 0){
  powermode = mode;
  Serial.print("esp going to sleep: ");
  Serial.println(mode == POWERMODE_STANDBY ? "standby" : "deep-sleep");
  digitalWrite(transistorPin, LOW);
}
void wakeUp(){
  digitalWrite(transistorPin, HIGH);
  powermode = POWERMODE_ACTIVE;
  justWokeUp = true;
  ui.begin();
  ui.Update();  //Update UI with defaults
  for (int i = 0; i < NumPotis; i++){
    last[i] = 0;
  }
  ui.drawSliderRow(last);

  Serial.println("waking up!");
}

void STANDBY_OPERATIONS(){
  receiver.update();
  if(UpdateSliders(SliderOutput, true) || receiver.DeejJustConnected()){
    Serial.println(SliderOutput);
    wakeUp();
  }
  if (receiver.hasNewSong()) {
    wakeUp();
    receiver.timeSincePackage = millis();
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
  if(updateButtons()) { wakeUp(); }
}
// ---------- Loop ----------
void loop() {

  if (receiver.resetTriggered()) {
    ui.drawAlbum("/default_cover.jpg");
  }

/*
  switch (powermode) {
    case POWERMODE_OFF:
      if(Serial.available() > 0){
        wakeUp();
        break;
      }
      delay(1000); return;  // Skip actual program-logic alltogether

    case POWERMODE_STANDBY:  // read sliders but require a higher threshhold for them to register.
      STANDBY_OPERATIONS();
      return;
    
    case POWERMODE_ACTIVE:
      
      if(millis() - receiver.timeSincePackage >= 20 * 60'000) {
        goToSleep(POWERMODE_STANDBY);
        return;
      }
      
      if(millis() - receiver.timeSinceAnyMessage >= 10 * 1000){
        goToSleep(POWERMODE_OFF);
        return;
      }
      
      
      break;
  }

    */
  
  receiver.update();

  if(UpdateSliders(SliderOutput) || justWokeUp){
    justWokeUp = false;
    Serial.println(SliderOutput);
  }

  if (receiver.hasNewSong()) {
    //receiver.timeSincePackage = millis();
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