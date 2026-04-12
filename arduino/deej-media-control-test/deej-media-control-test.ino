// Test script for media control commands
// Sends CMD:playpause when button is pressed
// Sends slider data on 1 slider

#define BUTTON_PIN 4
#define SLIDER_PIN A0

int lastButtonState = HIGH;

const unsigned long debounceDelay = 50;        // 50ms debounce
unsigned long lastPressTime = 0;

void setup() {
  Serial.begin(9600);
  pinMode(BUTTON_PIN, INPUT_PULLUP);
  pinMode(SLIDER_PIN, INPUT);
  
  delay(1000);  // Wait for serial to establish
  Serial.println("Media control test started");
}

void loop() {
  // Read and send slider data
  int sliderValue = analogRead(SLIDER_PIN);
  Serial.println(String(sliderValue));
  
  // Check button for media command
  updateButton();
  
  delay(20);
}

void updateButton() {
  int currentState = digitalRead(BUTTON_PIN);
  unsigned long currentTime = millis();
  
  // Debounce: only act if enough time has passed
  if (currentTime - lastPressTime < debounceDelay) {
    return;
  }
  
  // Detect falling edge (button pressed - goes from HIGH to LOW)
  if (lastButtonState == HIGH && currentState == LOW) {
    lastPressTime = currentTime;
    
    // Send media command
    Serial.println("CMD:playpause");
    
    delay(100);  // Extra debounce delay after sending command
  }
  
  lastButtonState = currentState;
}
