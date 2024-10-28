help:
	@cat Makefile | grep '# `' | grep -v '@cat Makefile'

# `make build`                 Build binary
build:
	bash build.sh
