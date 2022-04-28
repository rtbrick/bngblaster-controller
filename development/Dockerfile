FROM ubuntu:latest

# gcc for cgo
RUN apt-get update && apt-get install -y --no-install-recommends \
		g++ \
		gcc \
		libc6-dev \
		make \
		pkg-config \
		ca-certificates  \
		git  \
		curl  \
	&& rm -rf /var/lib/apt/lists/*

RUN curl -s https://dl.google.com/go/go1.15.3.linux-amd64.tar.gz | tar -v -C /usr/local -xz

RUN set -eux; \
	export GOPATH="/go"; \
	export PATH="$GOPATH/bin:/usr/local/go/bin:$PATH"; \
	export GONOSUMDB="github.com/rtbrick/*,nd.rtbrick.com/*"; \
	go version; \
	go get -u golang.org/x/lint/golint;

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
ENV GONOSUMDB "github.com/rtbrick/*,nd.rtbrick.com/*"