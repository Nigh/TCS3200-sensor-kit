# pico USB CDC+HID template

`PID=0x4005`
`VID=0xACDC`

## Prepare

### Get docker

```shell
docker pull xianii/pico-sdk:latest
```

### build

```shell
# build (docker, files owned by your user)
make
# remove root-owned build/ from earlier docker runs
make docker_clean
# or: make clean  (falls back to docker_clean if build/ is not writable)
make format
make rebuild
```

## Usage

- Pico enumerates CDC and HID.
- Logs go out CDC via a 1 KB ring drained in the main loop (timer IRQ never writes USB).
- CDC `UPLOAD` + newline reboots to UF2 bootloader.
- Any HID OUT report is logged on CDC and echoed back with each byte incremented by 1.
