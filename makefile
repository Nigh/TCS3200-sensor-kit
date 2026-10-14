.PHONY: all build docker_build clean rebuild format test
all build docker_build clean rebuild format test:
	$(MAKE) -C firmware $@
