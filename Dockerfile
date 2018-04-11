FROM golang
MAINTAINER Sylvain Laurent

ENV GOBIN $GOPATH/bin
ENV PROJECT_PATH /go/src/github.com/Magicking/govelib

ADD vendor /usr/local/go/src/
ADD cmd $PROJECT_PATH/cmd
ADD common $PROJECT_PATH/common

WORKDIR $PROJECT_PATH

RUN go build -o /go/bin/crawler $PROJECT_PATH/cmd/crawler/main.go
RUN go build -o /go/bin/heatmap $PROJECT_PATH/cmd/heatmap/main.go

ENTRYPOINT /go/bin/crawler

EXPOSE 8080
