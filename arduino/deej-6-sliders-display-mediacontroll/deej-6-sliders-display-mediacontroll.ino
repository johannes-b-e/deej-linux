#include <TFT_eSPI.h>
#include <TJpg_Decoder.h>
#include <LittleFS.h>

//#include "wifi_manager.h"
#include "config.h"
#include "SerialReceiver.h"
#include "UserInterface.h"

int poti[6] = {33, 32, 35, 34, 39, 36};
int NumPotis = 6;

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

void initFS(){
  if (!LittleFS.begin(true)) {
    Serial.println("DBG:LittleFS mount FAILED even after format attempt");
  } else {
      Serial.println("DBG:LittleFS mounted OK");
  }

  Serial.println("DBG:--- LittleFS contents ---");
  File root = LittleFS.open("/");
  File file = root.openNextFile();
  while (file) {
      Serial.print("DBG:file=");
      Serial.print(file.name());
      Serial.print(" size=");
      Serial.println(file.size());
      file = root.openNextFile();
  }
  Serial.println("DBG:--- end ---");
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

  initFS();

  receiver.begin();

  ui.Update();  //Update UI with defaults
  ui.drawAlbum("/default_cover.jpg");

  for(int i = 0; i < NumPotis; i++){
    analogSetPinAttenuation(poti[i], ADC_11db);
  }

  pinMode(BUTTON_PLAYPAUSE, INPUT_PULLUP);
  pinMode(BUTTON_NEXT, INPUT_PULLUP);
  pinMode(BUTTON_MUTEMIC, INPUT_PULLUP);
  pinMode(BUTTON_PREVIOUS, INPUT_PULLUP);
}

float filtered[6] = {0};
int last[6] = {0};

int readSlider(int index, int raw) {

    filtered[index] =
        filtered[index] * 0.85 +
        raw * 0.15;

    int value = (int)(filtered[index] / 4);

    if (abs(value - last[index]) > 4) {
        last[index] = value;
    }

    return last[index];
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


String oldDebug;
// ---------- Loop ----------
void loop() {
  if(receiver.resetTriggered())
  {
    ui.drawAlbum("/default_cover.jpg");
  }
  receiver.update();
  String debug = "";
  for (int i = 0; i < NumPotis; i++) {
    debug += readSlider(i, analogRead(poti[i]));

    if (i < NumPotis - 1) {
      debug += "|";
    }
  }
  
  // Only send something if meaningful change as to not over-spam the serial-connection, or alternativly if a new Deej client initially connected
  if(debug != oldDebug || receiver.DeejJustConnected()){
    Serial.println(debug);
    oldDebug = debug;
  }
  
  if (receiver.hasNewSong()) {
    Serial.println("New song received!");

    ui.Update(
      receiver.getTitle(),
      receiver.getArtist(),
      receiver.getDuration(),
      true
    );
    bool pom = receiver.pausedOrmuted();
    Serial.print("DBG:main loop hasNewSong, pausedOrmuted="); Serial.println(pom);
    if (!pom) {
      Serial.println("DBG:main loop drawing /cover.jpg");
      UserInterface::instance->drawAlbum("/cover.jpg");
    }
  }
  

  //ui.UpdateProgessBar(0);
  updateButtons();
}