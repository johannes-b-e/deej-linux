import serial
import struct
import time
import threading

ser = serial.Serial("/dev/ttyUSB0", 921600)

CHUNK_SIZE = 4096

def wait_for_ok(timeout=2.0):
    ser.timeout = 0.001
    start = time.time()

    buf = ""

    while time.time() - start < timeout:
        data = ser.read(64).decode(errors="ignore")
        if data:
            buf += data

            if "OK\n" in buf:
                print("ESP:OK")
                return True
            else:
                print(f"ESP:{buf}\n")

    raise TimeoutError("No OK received")

# ---------- SERIAL MONITOR ----------
def send_meta(meta_dict):

    header = b'\xAA\x55'
    type_ = 1
    data_str = f"{meta_dict['Title']}|{meta_dict['Artist']}|{meta_dict['Duration']}"
    payload = data_str.encode()
    length = len(payload)

    # send header first
    ser.write(header)
    ser.write(bytes([type_]))
    ser.write(struct.pack("<I", length))

    ser.write(payload)


def send_image(path):
    with open(path, "rb") as f:
        data = f.read()

    print("Size:", len(data))

    header = b'\xAA\x55'
    type_ = 2
    length = len(data)

    # send header first
    ser.write(header)
    ser.write(bytes([type_]))
    ser.write(struct.pack("<I", length))

    # send payload in chunks
    offset = 0

    while offset < length:
        chunk = data[offset:offset + CHUNK_SIZE]

        ser.write(chunk)
        ser.flush()  # optional now, but OK for safety

        wait_for_ok()  # ⬅️ THIS is the key change

        offset += len(chunk)
        print(f"{offset}/{length}")

data = {
    "Title": "Knights - Cat on the Ridge",
    "Artist": "Pomrad",
    "Duration": 107
}

send_meta(data)
send_image("cover.jpg")


while True:
    time.sleep(1)
