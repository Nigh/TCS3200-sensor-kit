.PHONY: all build docker_build clean rebuild format test
all build docker_build clean rebuild format test:
	$(MAKE) -C firmware $@

.PHONY: host host-demo host-build host-test
host:
	$(MAKE) -C host run
host-demo:
	$(MAKE) -C host demo
host-build:
	$(MAKE) -C host build
host-test:
	$(MAKE) -C host test
