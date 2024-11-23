help:
	@cat Makefile | grep '# `' | grep -v '@cat Makefile'

# `make build`                         构建
build:
	bash build.sh

# `make publish`                       发布
publish:
	bash publish.sh
