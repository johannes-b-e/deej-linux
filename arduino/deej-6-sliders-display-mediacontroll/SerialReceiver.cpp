#include "SerialReceiver.h"
#include "UserInterface.h"

uint8_t state = 0;

uint8_t type = 0;
uint32_t len = 0;
uint32_t idx = 0;

int readDataTimeoutMs = 1000;
int readDataStartedAt = 0;

bool paused = false;
bool muted = false;

File imgFile;

// states
#define WAIT_AA 0
#define READ_DATA 1


void SerialReceiver::begin() {
  
}

uint8_t PackageType = 0;

// // ---------- UPDATE ----------
void SerialReceiver::update() {

  
  // ---------- RESET CHECK  ----------
  if (state == WAIT_AA && Serial.available() > 0 && Serial.peek() == 0xFF) {
    Serial.read(); // consume the marker byte
    DeejConnected = true;
    Serial.println("RESET_ACK"); // optional, but useful for debugging
    return; // don't fall through to header parsing this call
  }
  

  // ---------- HEADER ----------
  if (state == WAIT_AA) {

    // wait until enough bytes came in for a full package header:
    if (Serial.available() < 7) return;

    // Check for the marker-bytes announcing a new package:
    if (Serial.read() != 0xAA) return;
    if (Serial.read() != 0x55) return;

    // the next byte corresponds to the package type: 1 = Metadatastring / 2 = coverimage / 3 = misc. UI-Update
    PackageType = Serial.read();

    // read another 4 bytes which constitute the total lengths of the package content in bytes:
    uint8_t lenBytes[4];
    Serial.readBytes((char*)lenBytes, 4);

    length =
      (uint32_t)lenBytes[0] |
      ((uint32_t)lenBytes[1] << 8) |
      ((uint32_t)lenBytes[2] << 16) |
      ((uint32_t)lenBytes[3] << 24);

    //Serial.println("Header OK");
    //Serial.println(length); // gets misinterpreted as a single-slider value

    // Start a fresh image file for every cover package
    if (PackageType == 2) {
      if (imageFile) {
          imageFile.close();
      }

      LittleFS.remove("/cover.jpg");
      imageFile = LittleFS.open("/cover.jpg", FILE_WRITE);

      if (!imageFile) {
          Serial.println("IMAGE_OPEN_FAILED");
          state = WAIT_AA;
          return;
      }
    }

    received = 0;   // reset the counter on how many chunks are received
    state = READ_DATA;  // switches state so that package-content can be read
    readDataStartedAt = millis(); // take a snapshot of the current systemtime
  }

  

  // ---------- DATA ----------
  if (state == READ_DATA) {
    if (millis() - readDataStartedAt > readDataTimeoutMs) {
      if (imageFile) {
        imageFile.close();
      }
      state = WAIT_AA;
      Serial.println("TIMEOUT_ABORT");
      reset = true;
      return;
    }

    if (Serial.available() == 0) return;
    if(PackageType == 2){

      // Receive the image in chunks. The final chunk may be smaller than BUF_SIZE.
      size_t remaining = length - received;
      size_t toRead = min((size_t)BUF_SIZE, remaining); // if remaining < bufsize, only read remaining, otherwise readBytes will timeout
      int n = Serial.readBytes(buffer, toRead);

      // literally just write these bytes to our cover image file
      imageFile.write(buffer, n);
      received += n;  // keep track on how many bytes where received
      
      Serial.write("OK\n"); // give the ok for the next chunk to be send
      readDataStartedAt = millis();   // reset the timeout after each successfully received chunk.

      if (received >= length) {
        imageFile.close();
        state = WAIT_AA;
        newSongReady = true;  // notify the rest of the program that a new song is ready to be displayed on the screen
        Serial.println("DONE");
        //LittleFS.rename("/cover.jpg", "/paused.jpg");    // Using this and the test.py python script you can upload files to the esp's permanent storage
      }
    }
    else if(PackageType == 1){ 
      // Metadata-String: "title|artist|length(sec)"
      int n = Serial.readBytes(buffer, length); // read exactly as many bytes as the header announced
      String Meta = String((char*)buffer).substring(0, length);   // cast byte-array as a string

      // split the string along "|" and extract the metadata fields:
      int first = Meta.indexOf('|');
      int second = Meta.indexOf('|', first + 1);

      title = Meta.substring(0, first);
      artist = Meta.substring(first + 1, second);
      String durationStr = Meta.substring(second + 1);

      duration = durationStr.toInt();
      state = WAIT_AA;    // reset the receiver
    }
    else if(PackageType == 3) // Update-Package, containing info like: timestamp, mic-mute-status
    {
      // Update-String: "timestamp(sec)|pause-status(1/0)|mic(1 / 0)"
      int n = Serial.readBytes(buffer, length); // read exactly as many bytes as the header announced
      String Update = String((char*)buffer).substring(0, length);   // cast byte-array as a string

      // split the string along "|" and extract the update-data fields:
      int first = Update.indexOf('|');
      int second = Update.indexOf('|', first + 1);

      int timestamp = Update.substring(0, first).toInt();
      int pausestatus = Update.substring(first + 1, second).toInt();
      int mutestatus = Update.substring(second + 1).toInt();

      // update UI
      if (UserInterface::instance) {
        UserInterface::instance->UpdateProgessBar(timestamp);
        // Hierarchy: Muted -> paused -> normal-cover
        if (mutestatus != muted || pausestatus != paused) {
          muted = mutestatus;
          paused = pausestatus;

          UserInterface::instance->drawAlbum(
              muted ? "/muted.jpg" :
              paused ? "/paused.jpg" :
                      "/cover.jpg"
          );
        }
      }

      state = WAIT_AA;    // reset the receiver
    }
  }
}



// ---------- PUBLIC ----------
bool SerialReceiver::hasNewSong() { return newSongReady && !(newSongReady = false); }
bool SerialReceiver::DeejJustConnected() { return DeejConnected && !(DeejConnected = false); }
bool SerialReceiver::resetTriggered() { return reset && !(reset = false); }
bool SerialReceiver::pausedOrmuted() { return muted || paused; }

String SerialReceiver::getTitle() { return title; }
String SerialReceiver::getArtist() { return artist; }
int SerialReceiver::getDuration() { return duration; }