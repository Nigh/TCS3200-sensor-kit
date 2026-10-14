cmake_minimum_required(VERSION 3.16)
set(PICO_BOARD waveshare_rp2040_zero CACHE STRING "Target board")

# Pull in SDK (must be before project)
include($ENV{PICO_SDK_PATH}/external/pico_sdk_import.cmake)

project(tcs3200_sensor C CXX ASM)
set(CMAKE_C_STANDARD 11)

pico_sdk_init()

add_subdirectory(src)
