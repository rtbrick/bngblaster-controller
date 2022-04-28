CWD := $(shell cd -P -- '$(shell dirname -- "$0")' && pwd -P)
PARENT := $(shell cd '$(CWD)/..' && pwd -P)


create_container:
	docker build . -t ubuntu-dev

develop_in_docker:
	docker run --rm -it \
     -v /var/run/docker.sock:/var/run/docker.sock \
     -v $(PARENT):/development \
     -w /development \
     ubuntu-dev:latest /bin/bash