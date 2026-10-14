BUILD_DIR = build
SDK_IMAGE = xianii/pico-sdk:2.3.0
CC = cc

.PHONY: all build docker_build clean rebuild format test
all: docker_build

build:
	cmake -S . -B $(BUILD_DIR) -G Ninja -DCMAKE_BUILD_TYPE=Release
	cmake --build $(BUILD_DIR)

docker_build:
	docker run --rm -u $(shell id -u):$(shell id -g) \
		-v $(CURDIR):/workspace -w /workspace \
		$(SDK_IMAGE) /bin/bash -c 'cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release && cmake --build build'

clean:
	rm -rf $(BUILD_DIR)

rebuild: clean all

format:
	bash format.sh

test:
	mkdir -p $(BUILD_DIR)/tests
	$(CC) -std=c11 -Wall -Wextra -Werror -pedantic -g -fsanitize=address,undefined -Isrc \
		tests/test_core.c src/protocol.c src/app.c -o $(BUILD_DIR)/tests/test_core
	$(BUILD_DIR)/tests/test_core
	$(CC) -std=c11 -Wall -Wextra -Werror -pedantic -g -fsanitize=address,undefined -Itests/stubs -Isrc \
		tests/test_usb.c src/usb_func.c src/protocol.c src/app.c -o $(BUILD_DIR)/tests/test_usb
	$(BUILD_DIR)/tests/test_usb
	python3 tests/test_pio.py
	python3 tests/test_vectors.py
