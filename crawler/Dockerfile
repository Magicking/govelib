FROM golang
MAINTAINER Sylvain Laurent

ENV GOBIN $GOPATH/bin

ENV PROJECT_PATH /go/src/crawler

COPY src /go/src
COPY vendor $PROJECT_PATH/vendor
COPY main.go $PROJECT_PATH/

WORKDIR $PROJECT_PATH

#RUN go get -v

RUN go install $PROJECT_PATH/main.go

ENTRYPOINT /go/bin/main
