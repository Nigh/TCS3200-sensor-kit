"""Keep the shared protocol's published wire examples independently checked."""
import binascii
from pathlib import Path
import re
import struct

document = (Path(__file__).resolve().parents[2] / "docs/protocol.md").read_text()
vectors = [bytes.fromhex(line) for line in re.findall(r"^54 43(?: [0-9a-f]{2})+$", document, re.M)]
assert len(vectors) == 5
for frame in vectors:
    assert frame[2] == 3
    length = int.from_bytes(frame[6:8], "little")
    assert len(frame) == length + 10
    assert binascii.crc_hqx(frame[2:-2], 0xFFFF) == int.from_bytes(frame[-2:], "little")
assert vectors[0][2:8] == struct.pack("<BBHH", 3, 1, 1, 0)
assert vectors[1][8:-2] == b"\0"
assert vectors[2][8:-2] == b"\2\0"
assert vectors[3][8:-2] == struct.pack("<I", 15000)
assert vectors[4][8:-2] == struct.pack("<II4H6B", 0, 100000, 0, 0, 0, 0, 15, 0, 0, 0, 0, 15)
print("shared protocol's five golden vectors passed independent CRC/layout checks")
