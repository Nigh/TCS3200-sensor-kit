add_executable(tcs3200_sensor
    main.c sensor.c app.c protocol.c usb_func.c usb_poll.c usb_descriptors.c)
target_include_directories(tcs3200_sensor PRIVATE ${CMAKE_CURRENT_LIST_DIR})
pico_generate_pio_header(tcs3200_sensor ${CMAKE_CURRENT_LIST_DIR}/period.pio)
target_link_libraries(tcs3200_sensor PRIVATE
    pico_stdlib pico_multicore pico_unique_id hardware_pio hardware_clocks tinyusb_device)
target_compile_options(tcs3200_sensor PRIVATE -Wall -Wextra -Werror)
pico_enable_stdio_usb(tcs3200_sensor 0)
pico_enable_stdio_uart(tcs3200_sensor 0)
pico_add_extra_outputs(tcs3200_sensor)
