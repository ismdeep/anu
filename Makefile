help:
	@cat Makefile | grep '# `' | grep -v '@cat Makefile'

# `make build-deb-in-docker`           通过Docker环境构建amd64和arm64架构的deb包
.PHONY: build-deb-in-docker
build-deb-in-docker:
	bash build-deb-in-docker.sh amd64
	bash build-deb-in-docker.sh arm64